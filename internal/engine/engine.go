package engine

import (
	"context"
	"errors"

	"github.com/beark7/ember/internal/catalog"
	"github.com/beark7/ember/internal/probe"
)

// Sentinel errors shared by every adapter.
var (
	// ErrNotReady is returned by a Runner that is starting, crashed or stopped.
	ErrNotReady = errors.New("engine: runner not ready")
	// ErrUnsupported is returned when an engine cannot serve a model or a
	// request feature (e.g. embeddings on a chat-only runner).
	ErrUnsupported = errors.New("engine: unsupported")
	// ErrStopped is returned by operations on a Runner after Stop.
	ErrStopped = errors.New("engine: runner stopped")
	// ErrLoadFailed is returned by Start when the process cannot load the model.
	ErrLoadFailed = errors.New("engine: model load failed")
	// ErrNoSlot is returned by Chat when every slot of the runner is busy; the
	// scheduler is expected to prevent this by honouring Health.SlotsIdle.
	ErrNoSlot = errors.New("engine: no free slot")
)

// Backend is a compute path inside an engine.
type Backend string

// Known backends. Adapters may report others; the planner treats unknown
// backends as CPU-class.
const (
	BackendCPU    Backend = "cpu"
	BackendVulkan Backend = "vulkan"
	BackendCUDA   Backend = "cuda"
	BackendMetal  Backend = "metal"
	BackendROCm   Backend = "rocm"
	BackendMock   Backend = "mock"
)

// Device is one compute device visible to a backend, with memory in bytes.
type Device struct {
	Index       int    `json:"index"`
	Name        string `json:"name"`
	MemoryTotal int64  `json:"memory_total"`
	MemoryFree  int64  `json:"memory_free"`
}

// BackendInfo describes an installed, usable backend build of an engine.
type BackendInfo struct {
	Backend Backend  `json:"backend"`
	Version string   `json:"version"` // engine build tag, e.g. llama.cpp "b7123"
	Devices []Device `json:"devices"`
}

// Engine is an inference implementation Ember can drive. Implementations
// must be safe for concurrent use.
//
// The interface is a contract (AGENTS.md §4): changes need an ADR or a design
// note in the same PR. See docs-private/design/engine.md.
type Engine interface {
	// Name is the stable identifier: "llamacpp", "ds4", "mock".
	Name() string
	// Backends reports the backend builds installed and usable on this machine.
	Backends(ctx context.Context, hw probe.HardwareProfile) ([]BackendInfo, error)
	// Supports reports whether the engine can serve the model on this hardware
	// and, when it cannot, the human-readable reasons.
	Supports(model catalog.ModelDescriptor, hw probe.HardwareProfile) (ok bool, reasons []string)
	// Prepare translates a placement decided by the planner into a concrete
	// runner specification (argv, env). It has no side effects.
	Prepare(ctx context.Context, req PrepareRequest) (RunnerSpec, error)
	// Start launches the runner and returns once it is healthy or fails.
	Start(ctx context.Context, spec RunnerSpec) (Runner, error)
}

// PrepareRequest is the engine-neutral placement produced by the planner.
// Fields an engine does not understand are ignored; fields it cannot honour
// make Prepare fail with ErrUnsupported.
type PrepareRequest struct {
	ModelRef  string  `json:"model_ref"`  // "<id>:<quant>" as shown to users
	ModelPath string  `json:"model_path"` // local file (GGUF; JSON descriptor for the mock)
	Backend   Backend `json:"backend"`

	ContextTokens int `json:"context_tokens"`
	Parallel      int `json:"parallel"` // slots / max concurrent sequences
	Threads       int `json:"threads"`  // 0 = engine default

	GPULayers      int      `json:"gpu_layers"`      // -1 = all, 0 = none
	CPUMoELayers   int      `json:"cpu_moe_layers"`  // llama.cpp --n-cpu-moe
	TensorOverride []string `json:"tensor_override"` // llama.cpp --override-tensor patterns
	KVCacheType    string   `json:"kv_cache_type"`   // f16, q8_0, q4_0; "" = engine default
	FlashAttention bool     `json:"flash_attention"`
	MMProjPath     string   `json:"mmproj_path"`   // vision projector, optional
	ChatTemplate   string   `json:"chat_template"` // override, optional
	Embedding      bool     `json:"embedding"`     // embedding model
	RPCServers     []string `json:"rpc_servers"`   // phase 9 pooling

	PlanID string         `json:"plan_id"`
	Memory MemoryEstimate `json:"memory"`
}

// MemoryEstimate is the planner's expected footprint per tier, in bytes.
type MemoryEstimate struct {
	VRAM int64 `json:"vram"`
	RAM  int64 `json:"ram"`
	SSD  int64 `json:"ssd"`
}

// RunnerSpec is a fully rendered, engine-specific launch description. The
// Manager adds host, port and the per-process API key when starting the
// process (ADR-015); Argv and Env never contain them.
type RunnerSpec struct {
	Engine    string            `json:"engine"`
	Backend   Backend           `json:"backend"`
	ModelRef  string            `json:"model_ref"`
	ModelPath string            `json:"model_path"`
	Argv      []string          `json:"argv"`
	Env       map[string]string `json:"env,omitempty"`
	Ctx       int               `json:"ctx"`
	Parallel  int               `json:"parallel"`
	PlanID    string            `json:"plan_id"`
	Memory    MemoryEstimate    `json:"memory"`
}
