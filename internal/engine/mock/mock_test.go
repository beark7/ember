package mock_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/beark7/ember/internal/catalog"
	"github.com/beark7/ember/internal/engine"
	"github.com/beark7/ember/internal/engine/enginetest"
	"github.com/beark7/ember/internal/engine/mock"
	"github.com/beark7/ember/internal/probe"
)

func modelPath(name string) string {
	return filepath.Join("..", "..", "..", "testdata", "models", name)
}

// startRunner prepares and starts a runner with cfg as engine defaults and an
// optional model file overlay.
func startRunner(t *testing.T, cfg mock.Config, model string) engine.Runner {
	t.Helper()
	eng := mock.New(cfg)
	req := engine.PrepareRequest{ModelRef: "mock:test"}
	if model != "" {
		req.ModelPath = modelPath(model)
	}
	spec, err := eng.Prepare(context.Background(), req)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	r, err := eng.Start(context.Background(), spec)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = r.Stop(context.Background()) })
	return r
}

// collect drains a stream and returns its events.
func collect(t *testing.T, ch <-chan engine.Event) []engine.Event {
	t.Helper()
	var out []engine.Event
	deadline := time.After(10 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-deadline:
			t.Fatal("stream did not close within 10s")
		}
	}
}

func userReq(text string, maxTokens int) engine.ChatRequest {
	return engine.ChatRequest{
		Messages: []engine.Message{engine.TextMessage(engine.RoleUser, text)},
		Sampling: engine.Sampling{MaxTokens: maxTokens},
		Stream:   true,
	}
}

func joinText(evs []engine.Event) string {
	var b strings.Builder
	for _, ev := range evs {
		if ev.Type == engine.EventToken {
			b.WriteString(ev.Text)
		}
	}
	return b.String()
}

func last(evs []engine.Event) engine.Event {
	return evs[len(evs)-1]
}

func TestConformance(t *testing.T) {
	enginetest.RunConformance(t, mock.New(mock.Config{}), engine.PrepareRequest{
		ModelRef: "mock-echo", ModelPath: modelPath("mock-echo.json"),
	})
}

func TestEchoRoundTripsTheUserMessage(t *testing.T) {
	r := startRunner(t, mock.Config{TokensPerSec: 5000}, "")
	evs := collect(t, mustChat(t, r, userReq("ciao mondo, come va?", 0)))
	if got := joinText(evs); got != "ciao mondo, come va?" {
		t.Fatalf("echo = %q", got)
	}
	if fin := last(evs); fin.Type != engine.EventDone || fin.FinishReason != engine.FinishStop {
		t.Fatalf("terminal = %+v", fin)
	}
}

func TestSpeedIsHonoured(t *testing.T) {
	r := startRunner(t, mock.Config{TokensPerSec: 100, Reply: mock.ReplyLorem}, "")
	start := time.Now()
	evs := collect(t, mustChat(t, r, userReq("go", 50)))
	elapsed := time.Since(start)
	tokens := 0
	for _, ev := range evs {
		if ev.Type == engine.EventToken {
			tokens++
		}
	}
	if tokens != 50 {
		t.Fatalf("tokens = %d, want 50", tokens)
	}
	// 50 tokens at 100 t/s ≈ 0.5 s; allow scheduler noise on CI.
	if elapsed < 400*time.Millisecond || elapsed > 900*time.Millisecond {
		t.Fatalf("elapsed = %v, want ≈ 500ms", elapsed)
	}
	u := usageOf(t, evs)
	if u.CompletionTokens != 50 || u.DecodeTPS < 70 || u.DecodeTPS > 130 {
		t.Fatalf("usage = %+v", u)
	}
	if fin := last(evs); fin.FinishReason != engine.FinishLength {
		t.Fatalf("finish = %q, want length", fin.FinishReason)
	}
}

func TestToolCallRoundTrip(t *testing.T) {
	r := startRunner(t, mock.Config{}, "mock-tools.json")
	tools := []engine.Tool{{Name: "get_weather", Parameters: json.RawMessage(`{"type":"object"}`)}}

	req := userReq("What's the weather in Rome?", 0)
	req.Tools = tools
	evs := collect(t, mustChat(t, r, req))

	var id, name, args string
	for _, ev := range evs {
		if ev.Type == engine.EventToolCall {
			if ev.ToolCall.ID != "" {
				id, name = ev.ToolCall.ID, ev.ToolCall.Name
			}
			args += ev.ToolCall.ArgumentsDelta
		}
	}
	if id == "" || name != "get_weather" {
		t.Fatalf("tool call id/name = %q/%q", id, name)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(args), &parsed); err != nil || parsed["city"] != "Rome" {
		t.Fatalf("arguments %q: %v", args, err)
	}
	if fin := last(evs); fin.FinishReason != engine.FinishToolCalls {
		t.Fatalf("finish = %q, want tool_calls", fin.FinishReason)
	}

	// Second turn: the tool result comes back and the model answers with text.
	req2 := engine.ChatRequest{
		Messages: []engine.Message{
			engine.TextMessage(engine.RoleUser, "What's the weather in Rome?"),
			{Role: engine.RoleAssistant, ToolCalls: []engine.ToolCall{{ID: id, Name: name, Arguments: json.RawMessage(args)}}},
			{Role: engine.RoleTool, ToolCallID: id, Name: name, Parts: []engine.Part{{Type: engine.PartText, Text: "22°C sunny"}}},
		},
		Tools:  tools,
		Stream: true,
	}
	evs2 := collect(t, mustChat(t, r, req2))
	if got := joinText(evs2); !strings.Contains(got, "22°C sunny") {
		t.Fatalf("second turn text = %q", got)
	}
	if fin := last(evs2); fin.FinishReason != engine.FinishStop {
		t.Fatalf("second turn finish = %q", fin.FinishReason)
	}
}

func TestToolChoiceNoneSuppressesToolCalls(t *testing.T) {
	r := startRunner(t, mock.Config{}, "mock-tools.json")
	req := userReq("weather?", 0)
	req.Tools = []engine.Tool{{Name: "get_weather"}}
	req.ToolChoice = engine.ToolChoice{Mode: engine.ToolChoiceNone}
	evs := collect(t, mustChat(t, r, req))
	for _, ev := range evs {
		if ev.Type == engine.EventToolCall {
			t.Fatal("tool call emitted with tool_choice none")
		}
	}
}

func TestCrashAfterTokens(t *testing.T) {
	r := startRunner(t, mock.Config{}, "mock-crash.json")
	evs := collect(t, mustChat(t, r, userReq("go", 0)))
	fin := last(evs)
	if fin.Type != engine.EventError || !errors.Is(fin.Err, mock.ErrCrashed) {
		t.Fatalf("terminal = %+v", fin)
	}
	h, err := r.Health(context.Background())
	if h.Ready || !errors.Is(err, mock.ErrCrashed) {
		t.Fatalf("Health after crash = %+v, %v", h, err)
	}
	if _, err := r.Chat(context.Background(), userReq("again", 0)); !errors.Is(err, engine.ErrNotReady) {
		t.Fatalf("Chat after crash: %v", err)
	}
}

func TestCancelClosesWithin100ms(t *testing.T) {
	r := startRunner(t, mock.Config{TokensPerSec: 20, Reply: mock.ReplyLorem}, "")
	ctx, cancel := context.WithCancel(context.Background())
	ch := mustChatCtx(ctx, t, r, userReq("go", 1000))
	<-ch
	cancel()
	start := time.Now()
	for range ch { // drain until closed
		continue
	}
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Fatalf("closed after %v, want ≤ 100ms", d)
	}
	h, _ := r.Health(context.Background())
	if h.SlotsIdle != h.SlotsTotal {
		t.Fatalf("slot not released after cancel: %+v", h)
	}
}

func TestPrepareOverlaysModelFile(t *testing.T) {
	eng := mock.New(mock.Config{TokensPerSec: 1})
	spec, err := eng.Prepare(context.Background(), engine.PrepareRequest{
		ModelRef: "mock-lorem", ModelPath: modelPath("mock-lorem.json"), ContextTokens: 4096,
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	var cfg mock.Config
	if err := json.Unmarshal([]byte(spec.Env[mock.EnvConfig]), &cfg); err != nil {
		t.Fatalf("decode env config: %v", err)
	}
	if cfg.Reply != mock.ReplyLorem || cfg.TokensPerSec != 100 || cfg.Parallel != 2 {
		t.Fatalf("overlay not applied: %+v", cfg)
	}
	if spec.Ctx != 4096 || spec.Parallel != 2 || spec.Memory.RAM <= 0 {
		t.Fatalf("spec = %+v", spec)
	}
	if spec.Argv[0] != "mock-runner" {
		t.Fatalf("argv = %v", spec.Argv)
	}
}

func TestPrepareRejectsOtherBackends(t *testing.T) {
	_, err := mock.New(mock.Config{}).Prepare(context.Background(), engine.PrepareRequest{Backend: engine.BackendCUDA})
	if !errors.Is(err, engine.ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}

func TestFailLoad(t *testing.T) {
	eng := mock.New(mock.Config{FailLoad: true})
	spec, _ := eng.Prepare(context.Background(), engine.PrepareRequest{})
	if _, err := eng.Start(context.Background(), spec); !errors.Is(err, engine.ErrLoadFailed) {
		t.Fatalf("Start err = %v, want ErrLoadFailed", err)
	}
}

func TestStopSequence(t *testing.T) {
	r := startRunner(t, mock.Config{TokensPerSec: 5000, Reply: mock.ReplyScript, Script: []string{"alpha beta STOP gamma"}}, "")
	req := userReq("x", 0)
	req.Sampling.Stop = []string{"STOP"}
	evs := collect(t, mustChat(t, r, req))
	if got := joinText(evs); got != "alpha beta" {
		t.Fatalf("text = %q", got)
	}
}

func TestSlotsAreBounded(t *testing.T) {
	r := startRunner(t, mock.Config{TokensPerSec: 5, Reply: mock.ReplyLorem, Parallel: 1}, "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := mustChatCtx(ctx, t, r, userReq("go", 100))
	<-ch
	if _, err := r.Chat(ctx, userReq("second", 1)); !errors.Is(err, engine.ErrNoSlot) {
		t.Fatalf("second Chat err = %v, want ErrNoSlot", err)
	}
	if m := r.Metrics(); m.SlotsInUse != 1 || m.RequestsTotal != 1 {
		t.Fatalf("metrics = %+v", m)
	}
}

func TestTokenizeAndEmbeddingsAreDeterministic(t *testing.T) {
	r := startRunner(t, mock.Config{}, "")
	a, _ := r.Tokenize(context.Background(), "the quick brown fox")
	b, _ := r.Tokenize(context.Background(), "the quick brown fox")
	if len(a) != 4 || len(b) != 4 || a[0] != b[0] || a[3] != b[3] {
		t.Fatalf("tokenize not deterministic: %v %v", a, b)
	}
	e, err := r.Embeddings(context.Background(), []string{"x", "y"})
	if err != nil || len(e) != 2 || len(e[0]) != 8 || e[0][0] == e[1][0] && e[0][1] == e[1][1] {
		t.Fatalf("embeddings = %v, %v", e, err)
	}
}

func TestHTTPEndpoint(t *testing.T) {
	r := startRunner(t, mock.Config{}, "mock-echo.json")
	ctx := context.Background()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, r.Endpoint()+"/health", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"ok"`) {
		t.Fatalf("/health = %d %s", resp.StatusCode, body)
	}

	req, _ = http.NewRequestWithContext(ctx, http.MethodPost, r.Endpoint()+"/tokenize", strings.NewReader(`{"content":"a b c"}`))
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /tokenize: %v", err)
	}
	var tok struct {
		Tokens []int32 `json:"tokens"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&tok)
	_ = resp.Body.Close()
	if len(tok.Tokens) != 3 {
		t.Fatalf("/tokenize = %+v", tok)
	}

	if err := r.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	req, _ = http.NewRequestWithContext(ctx, http.MethodGet, r.Endpoint()+"/health", nil)
	if resp, err := http.DefaultClient.Do(req); err == nil {
		_ = resp.Body.Close()
		t.Fatal("endpoint still serving after Stop")
	}
}

func TestStopCancelsInFlightChat(t *testing.T) {
	r := startRunner(t, mock.Config{TokensPerSec: 5, Reply: mock.ReplyLorem}, "")
	ch := mustChat(t, r, userReq("go", 1000))
	<-ch
	if err := r.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	evs := collect(t, ch)
	if len(evs) == 0 {
		return // closed without a final event is acceptable
	}
	if fin := last(evs); fin.Type != engine.EventError {
		t.Fatalf("terminal after Stop = %+v", fin)
	}
}

func TestSupports(t *testing.T) {
	eng := mock.New(mock.Config{})
	if ok, _ := eng.Supports(catalogModel("llama"), hw()); !ok {
		t.Fatal("llama should be supported")
	}
	if ok, reasons := eng.Supports(catalogModel("unsupported"), hw()); ok || len(reasons) == 0 {
		t.Fatal("unsupported arch should be refused with a reason")
	}
	b, err := eng.Backends(context.Background(), hw())
	if err != nil || len(b) != 1 || b[0].Backend != engine.BackendMock || len(b[0].Devices) != 1 {
		t.Fatalf("Backends = %+v, %v", b, err)
	}
}

func catalogModel(arch string) catalog.ModelDescriptor {
	return catalog.ModelDescriptor{ID: "m", Arch: arch}
}

func hw() probe.HardwareProfile { return probe.HardwareProfile{} }

func usageOf(t *testing.T, evs []engine.Event) engine.Usage {
	t.Helper()
	for _, ev := range evs {
		if ev.Type == engine.EventUsage {
			return *ev.Usage
		}
	}
	t.Fatal("no usage event")
	return engine.Usage{}
}

func mustChat(t *testing.T, r engine.Runner, req engine.ChatRequest) <-chan engine.Event {
	t.Helper()
	return mustChatCtx(context.Background(), t, r, req)
}

func mustChatCtx(ctx context.Context, t *testing.T, r engine.Runner, req engine.ChatRequest) <-chan engine.Event {
	t.Helper()
	ch, err := r.Chat(ctx, req)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	return ch
}
