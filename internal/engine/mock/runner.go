package mock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/beark7/ember/internal/engine"
)

type state int

const (
	stateReady state = iota
	stateCrashed
	stateStopped
)

// runner is one in-process mock runner.
type runner struct {
	id   string
	spec engine.RunnerSpec
	cfg  Config

	ctx    context.Context // cancelled by Stop; every Chat derives from it
	cancel context.CancelFunc

	mu       sync.Mutex
	st       state
	inUse    int
	metrics  engine.Metrics
	scriptAt int
	calls    atomic.Int64

	server   *http.Server
	listener net.Listener
	stopOnce sync.Once
	stopErr  error
}

var _ engine.Runner = (*runner)(nil)

func newRunner(id string, spec engine.RunnerSpec, cfg Config) (*runner, error) {
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("mock: listen: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &runner{id: id, spec: spec, cfg: cfg, ctx: ctx, cancel: cancel, listener: ln}
	r.metrics.Memory = engine.MemoryEstimate{RAM: cfg.MemoryBytes}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", r.handleHealth)
	mux.HandleFunc("GET /props", r.handleProps)
	mux.HandleFunc("POST /tokenize", r.handleTokenize)
	r.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = r.server.Serve(ln) }()
	return r, nil
}

// ID implements engine.Runner.
func (r *runner) ID() string { return r.id }

// Spec implements engine.Runner.
func (r *runner) Spec() engine.RunnerSpec { return r.spec }

// Endpoint implements engine.Runner.
func (r *runner) Endpoint() string { return "http://" + r.listener.Addr().String() }

// Health implements engine.Runner.
func (r *runner) Health(_ context.Context) (engine.Health, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	h := engine.Health{
		ModelRef:   r.spec.ModelRef,
		SlotsTotal: r.cfg.Parallel,
		SlotsIdle:  r.cfg.Parallel - r.inUse,
		Memory:     engine.MemoryEstimate{RAM: r.cfg.MemoryBytes},
	}
	switch r.st {
	case stateReady:
		h.Ready = true
	case stateCrashed:
		h.Message = ErrCrashed.Error()
		return h, ErrCrashed
	case stateStopped:
		h.Message = engine.ErrStopped.Error()
		return h, engine.ErrStopped
	}
	return h, nil
}

// Metrics implements engine.Runner.
func (r *runner) Metrics() engine.Metrics {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.metrics
	m.SlotsInUse = r.inUse
	m.RequestsTotal = r.calls.Load()
	return m
}

// Stop implements engine.Runner. It cancels in-flight chats and closes the
// HTTP endpoint; it is idempotent.
func (r *runner) Stop(ctx context.Context) error {
	r.stopOnce.Do(func() {
		r.mu.Lock()
		r.st = stateStopped
		r.mu.Unlock()
		r.cancel()
		if err := r.server.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			r.stopErr = fmt.Errorf("mock: shutdown: %w", err)
		}
	})
	return r.stopErr
}

// Tokenize implements engine.Runner with a deterministic whitespace tokenizer.
func (r *runner) Tokenize(_ context.Context, text string) ([]int32, error) {
	if err := r.checkReady(); err != nil {
		return nil, err
	}
	return tokenize(text), nil
}

// Embeddings implements engine.Runner with deterministic 8-dimensional vectors.
func (r *runner) Embeddings(_ context.Context, texts []string) ([][]float32, error) {
	if err := r.checkReady(); err != nil {
		return nil, err
	}
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = embed(t)
	}
	return out, nil
}

func (r *runner) checkReady() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch r.st {
	case stateStopped:
		return engine.ErrStopped
	case stateCrashed:
		return fmt.Errorf("%w: %w", engine.ErrNotReady, ErrCrashed)
	default:
		return nil
	}
}

// Chat implements engine.Runner.
func (r *runner) Chat(ctx context.Context, req engine.ChatRequest) (<-chan engine.Event, error) {
	r.mu.Lock()
	switch r.st {
	case stateStopped:
		r.mu.Unlock()
		return nil, engine.ErrStopped
	case stateCrashed:
		r.mu.Unlock()
		return nil, fmt.Errorf("%w: %w", engine.ErrNotReady, ErrCrashed)
	}
	if r.inUse >= r.cfg.Parallel {
		r.mu.Unlock()
		return nil, engine.ErrNoSlot
	}
	r.inUse++
	reply := r.pickReplyLocked(req)
	r.mu.Unlock()
	r.calls.Add(1)

	ch := make(chan engine.Event, 16)
	go r.generate(ctx, req, reply, ch)
	return ch, nil
}

// pickReplyLocked decides the answer for req. Caller holds r.mu.
func (r *runner) pickReplyLocked(req engine.ChatRequest) plannedReply {
	last := lastMessage(req.Messages)

	if req.ToolChoice.Mode != engine.ToolChoiceNone && last.Role == engine.RoleUser {
		for _, t := range req.Tools {
			if req.ToolChoice.Mode == engine.ToolChoiceNamed && req.ToolChoice.Name != t.Name {
				continue
			}
			if args, ok := r.cfg.ToolCalls[t.Name]; ok {
				return plannedReply{toolName: t.Name, toolArgs: string(args)}
			}
		}
	}

	var text string
	switch r.cfg.Reply {
	case ReplyLorem:
		text = lorem
	case ReplyScript:
		if len(r.cfg.Script) > 0 {
			text = r.cfg.Script[r.scriptAt%len(r.cfg.Script)]
			r.scriptAt++
		}
	default: // echo
		switch last.Role {
		case engine.RoleTool:
			text = fmt.Sprintf("Tool %s returned: %s", last.Name, last.Text())
		default:
			text = last.Text()
		}
	}
	if text == "" {
		text = "(empty)"
	}
	return plannedReply{text: text}
}

type plannedReply struct {
	text     string
	toolName string
	toolArgs string
}

func (r *runner) generate(reqCtx context.Context, req engine.ChatRequest, reply plannedReply, ch chan<- engine.Event) {
	defer close(ch)
	defer func() {
		r.mu.Lock()
		r.inUse--
		r.mu.Unlock()
	}()

	ctx, cancel := context.WithCancel(reqCtx)
	defer cancel()
	stop := context.AfterFunc(r.ctx, cancel) // runner Stop cancels this chat too
	defer stop()

	send := func(ev engine.Event) bool {
		select {
		case ch <- ev:
			return true
		case <-ctx.Done():
			return false
		}
	}
	fail := func(err error, reason engine.FinishReason) {
		// Best effort: the consumer may already be gone.
		select {
		case ch <- engine.Event{Type: engine.EventError, Err: err, FinishReason: reason}:
		default:
		}
	}

	start := time.Now()
	promptTokens := countPromptTokens(req.Messages)
	if !sleepCtx(ctx, time.Duration(float64(promptTokens)/r.cfg.PrefillTokensPerSec*float64(time.Second))) {
		fail(ctx.Err(), engine.FinishCancelled)
		return
	}
	ttft := time.Since(start)
	perToken := time.Duration(float64(time.Second) / r.cfg.TokensPerSec)

	// Chunks: either text tokens or tool-call argument pieces.
	var chunks []engine.Event
	finish := engine.FinishStop
	if reply.toolName != "" {
		id := fmt.Sprintf("call_%d", r.calls.Load())
		parts := splitN(reply.toolArgs, 3)
		chunks = append(chunks, engine.Event{Type: engine.EventToolCall, ToolCall: &engine.ToolCallDelta{Index: 0, ID: id, Name: reply.toolName}})
		for _, p := range parts {
			chunks = append(chunks, engine.Event{Type: engine.EventToolCall, ToolCall: &engine.ToolCallDelta{Index: 0, ArgumentsDelta: p}})
		}
		finish = engine.FinishToolCalls
	} else {
		toks := wordTokens(reply.text)
		maxTokens := req.Sampling.MaxTokens
		if maxTokens <= 0 {
			maxTokens = r.cfg.DefaultMaxTokens
		}
		if len(toks) > maxTokens {
			toks = toks[:maxTokens]
			finish = engine.FinishLength
		}
		if cut, ok := cutAtStop(toks, req.Sampling.Stop); ok {
			toks = cut
			finish = engine.FinishStop
		}
		for _, t := range toks {
			chunks = append(chunks, engine.Event{Type: engine.EventToken, Text: t})
		}
	}

	emitted := 0
	genStart := time.Now()
	for _, ev := range chunks {
		if r.cfg.CrashAfterTokens > 0 && emitted >= r.cfg.CrashAfterTokens {
			r.crash()
			fail(ErrCrashed, engine.FinishError)
			return
		}
		if !send(ev) {
			fail(ctx.Err(), engine.FinishCancelled)
			return
		}
		emitted++
		if !sleepCtx(ctx, perToken) {
			fail(ctx.Err(), engine.FinishCancelled)
			return
		}
	}

	genDur := time.Since(genStart)
	usage := engine.Usage{
		PromptTokens:     promptTokens,
		CompletionTokens: emitted,
		TTFT:             ttft,
		Duration:         time.Since(start),
	}
	if ttft > 0 {
		usage.PrefillTPS = float64(promptTokens) / ttft.Seconds()
	}
	if genDur > 0 {
		usage.DecodeTPS = float64(emitted) / genDur.Seconds()
	}
	r.mu.Lock()
	r.metrics.DecodeTPS = usage.DecodeTPS
	r.metrics.PrefillTPS = usage.PrefillTPS
	r.metrics.TTFT = ttft
	r.mu.Unlock()

	if !send(engine.Event{Type: engine.EventUsage, Usage: &usage}) {
		fail(ctx.Err(), engine.FinishCancelled)
		return
	}
	send(engine.Event{Type: engine.EventDone, FinishReason: finish})
}

func (r *runner) crash() {
	r.mu.Lock()
	if r.st == stateReady {
		r.st = stateCrashed
	}
	r.mu.Unlock()
}

// --- HTTP endpoint (subset of llama-server's) ---

func (r *runner) handleHealth(w http.ResponseWriter, _ *http.Request) {
	h, err := r.Health(context.Background())
	w.Header().Set("Content-Type", "application/json")
	if err != nil || !h.Ready {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": h.Message})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "slots_idle": h.SlotsIdle})
}

func (r *runner) handleProps(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"model": r.spec.ModelRef, "n_ctx": r.cfg.ContextTokens, "total_slots": r.cfg.Parallel, "engine": Name,
	})
}

func (r *runner) handleTokenize(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<20)).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"tokens": tokenize(body.Content)})
}

// --- helpers ---

const lorem = "Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor " +
	"incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud " +
	"exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure " +
	"dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. " +
	"Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt " +
	"mollit anim id est laborum."

func lastMessage(msgs []engine.Message) engine.Message {
	if len(msgs) == 0 {
		return engine.Message{}
	}
	return msgs[len(msgs)-1]
}

// wordTokens splits text into whitespace-delimited tokens that concatenate
// back to the original spacing (each token but the first carries its leading
// space).
func wordTokens(text string) []string {
	fields := strings.Fields(text)
	out := make([]string, 0, len(fields))
	for i, f := range fields {
		if i > 0 {
			f = " " + f
		}
		out = append(out, f)
	}
	return out
}

func countPromptTokens(msgs []engine.Message) int {
	n := 0
	for _, m := range msgs {
		n += 4 + len(strings.Fields(m.Text()))
		for _, p := range m.Parts {
			if p.Type == engine.PartImage {
				n += 256
			}
		}
	}
	return n
}

// cutAtStop truncates toks at the first token containing a stop sequence.
func cutAtStop(toks []string, stops []string) ([]string, bool) {
	if len(stops) == 0 {
		return toks, false
	}
	for i, t := range toks {
		for _, s := range stops {
			if s != "" && strings.Contains(t, s) {
				return toks[:i], true
			}
		}
	}
	return toks, false
}

func splitN(s string, n int) []string {
	if n <= 1 || len(s) <= n {
		return []string{s}
	}
	size := (len(s) + n - 1) / n
	var out []string
	for len(s) > 0 {
		k := size
		if k > len(s) {
			k = len(s)
		}
		out = append(out, s[:k])
		s = s[k:]
	}
	return out
}

func tokenize(text string) []int32 {
	fields := strings.Fields(text)
	out := make([]int32, 0, len(fields))
	for _, f := range fields {
		h := fnv.New32a()
		_, _ = h.Write([]byte(f))
		out = append(out, int32(h.Sum32()%32000)) //nolint:gosec // bounded by the modulo, cannot overflow int32
	}
	return out
}

func embed(text string) []float32 {
	const dims = 8
	v := make([]float32, dims)
	for i := range dims {
		h := fnv.New32a()
		_, _ = h.Write([]byte(text))
		_, _ = h.Write([]byte{byte(i)})
		v[i] = float32(h.Sum32()%2000)/1000 - 1 // [-1, 1)
	}
	return v
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}
