package mock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/beark7/ember/internal/catalog"
	"github.com/beark7/ember/internal/engine"
	"github.com/beark7/ember/internal/probe"
)

// Name is the engine identifier.
const Name = "mock"

// EnvConfig is the RunnerSpec.Env key carrying the resolved Config as JSON,
// mirroring how a real adapter passes settings to its process.
const EnvConfig = "EMBER_MOCK_CONFIG"

// ErrCrashed is emitted (and returned by Health) after a simulated crash.
var ErrCrashed = errors.New("mock: runner crashed")

// ReplyMode selects how the mock builds its answers.
type ReplyMode string

// Reply modes.
const (
	// ReplyEcho repeats the last user message (or the last tool result).
	ReplyEcho ReplyMode = "echo"
	// ReplyLorem produces lorem ipsum text.
	ReplyLorem ReplyMode = "lorem"
	// ReplyScript cycles through Config.Script.
	ReplyScript ReplyMode = "script"
)

// Config drives the mock. The same structure, as JSON, is the "model file"
// referenced by PrepareRequest.ModelPath (testdata/models/mock-*.json); fields
// present in the file override the engine-level defaults.
type Config struct {
	TokensPerSec        float64                    `json:"tokens_per_sec,omitempty"`         // default 100
	PrefillTokensPerSec float64                    `json:"prefill_tokens_per_sec,omitempty"` // default 5000
	Reply               ReplyMode                  `json:"reply,omitempty"`                  // default echo
	Script              []string                   `json:"script,omitempty"`
	ToolCalls           map[string]json.RawMessage `json:"tool_calls,omitempty"` // tool name → arguments to emit
	CrashAfterTokens    int                        `json:"crash_after_tokens,omitempty"`
	FailLoad            bool                       `json:"fail_load,omitempty"`
	StartDelayMS        int                        `json:"start_delay_ms,omitempty"`
	MemoryBytes         int64                      `json:"memory_bytes,omitempty"` // reported RAM footprint, default 1 GiB
	Parallel            int                        `json:"parallel,omitempty"`     // slots, default 4
	ContextTokens       int                        `json:"context_tokens,omitempty"`
	DefaultMaxTokens    int                        `json:"default_max_tokens,omitempty"` // default 64
}

func (c Config) withDefaults() Config {
	if c.TokensPerSec <= 0 {
		c.TokensPerSec = 100
	}
	if c.PrefillTokensPerSec <= 0 {
		c.PrefillTokensPerSec = 5000
	}
	if c.Reply == "" {
		c.Reply = ReplyEcho
	}
	if c.MemoryBytes <= 0 {
		c.MemoryBytes = 1 << 30
	}
	if c.Parallel <= 0 {
		c.Parallel = 4
	}
	if c.ContextTokens <= 0 {
		c.ContextTokens = 8192
	}
	if c.DefaultMaxTokens <= 0 {
		c.DefaultMaxTokens = 64
	}
	return c
}

// Engine is the mock engine. It is safe for concurrent use.
type Engine struct {
	cfg    Config
	nextID atomic.Int64
}

// New returns a mock engine with cfg as the defaults for every runner.
func New(cfg Config) *Engine {
	return &Engine{cfg: cfg.withDefaults()}
}

var _ engine.Engine = (*Engine)(nil)

// Name implements engine.Engine.
func (e *Engine) Name() string { return Name }

// Backends implements engine.Engine: one virtual backend with one device.
func (e *Engine) Backends(_ context.Context, _ probe.HardwareProfile) ([]engine.BackendInfo, error) {
	return []engine.BackendInfo{{
		Backend: engine.BackendMock,
		Version: "mock-1",
		Devices: []engine.Device{{Index: 0, Name: "mock device", MemoryTotal: 64 << 30, MemoryFree: 60 << 30}},
	}}, nil
}

// Supports implements engine.Engine. The mock serves any model except those
// whose Arch is "unsupported", which lets tests exercise refusal paths.
func (e *Engine) Supports(model catalog.ModelDescriptor, _ probe.HardwareProfile) (bool, []string) {
	if model.Arch == "unsupported" {
		return false, []string{"mock: architecture marked unsupported"}
	}
	return true, nil
}

// Prepare implements engine.Engine. When req.ModelPath names an existing
// JSON file its fields override the engine defaults; the resolved Config
// travels in RunnerSpec.Env[EnvConfig].
func (e *Engine) Prepare(_ context.Context, req engine.PrepareRequest) (engine.RunnerSpec, error) {
	if req.Backend != "" && req.Backend != engine.BackendMock {
		return engine.RunnerSpec{}, fmt.Errorf("mock: backend %q: %w", req.Backend, engine.ErrUnsupported)
	}
	cfg := e.cfg
	if req.ModelPath != "" {
		data, err := os.ReadFile(req.ModelPath)
		if err != nil {
			return engine.RunnerSpec{}, fmt.Errorf("mock: read model file: %w", err)
		}
		if err := json.Unmarshal(data, &cfg); err != nil {
			return engine.RunnerSpec{}, fmt.Errorf("mock: parse model file %s: %w", req.ModelPath, err)
		}
		cfg = cfg.withDefaults()
	}
	if req.Parallel > 0 {
		cfg.Parallel = req.Parallel
	}
	if req.ContextTokens > 0 {
		cfg.ContextTokens = req.ContextTokens
	}
	encoded, err := json.Marshal(cfg)
	if err != nil {
		return engine.RunnerSpec{}, fmt.Errorf("mock: encode config: %w", err)
	}
	return engine.RunnerSpec{
		Engine:    Name,
		Backend:   engine.BackendMock,
		ModelRef:  req.ModelRef,
		ModelPath: req.ModelPath,
		Argv:      []string{"mock-runner", "--model", req.ModelPath},
		Env:       map[string]string{EnvConfig: string(encoded)},
		Ctx:       cfg.ContextTokens,
		Parallel:  cfg.Parallel,
		PlanID:    req.PlanID,
		Memory:    engine.MemoryEstimate{RAM: cfg.MemoryBytes},
	}, nil
}

// Start implements engine.Engine: it starts an in-process runner with a real
// loopback HTTP endpoint so that managers and servers treat it like a process.
func (e *Engine) Start(ctx context.Context, spec engine.RunnerSpec) (engine.Runner, error) {
	cfg := e.cfg
	if raw, ok := spec.Env[EnvConfig]; ok {
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			return nil, fmt.Errorf("mock: decode %s: %w", EnvConfig, err)
		}
		cfg = cfg.withDefaults()
	}
	if cfg.StartDelayMS > 0 {
		select {
		case <-time.After(time.Duration(cfg.StartDelayMS) * time.Millisecond):
		case <-ctx.Done():
			return nil, fmt.Errorf("mock: start: %w", ctx.Err())
		}
	}
	if cfg.FailLoad {
		return nil, fmt.Errorf("mock: fail_load set: %w", engine.ErrLoadFailed)
	}
	id := fmt.Sprintf("mock-%d", e.nextID.Add(1))
	return newRunner(id, spec, cfg)
}
