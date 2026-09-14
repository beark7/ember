package engine

import (
	"context"
	"time"
)

// Runner is one engine process serving one model. Implementations must be
// safe for concurrent use; Chat may be called concurrently up to the number
// of slots reported by Health.
type Runner interface {
	// ID is unique among runners of a Manager (ULID or similar).
	ID() string
	// Spec returns the specification the runner was started with.
	Spec() RunnerSpec
	// Endpoint is the local base URL, e.g. "http://127.0.0.1:43123".
	Endpoint() string
	// Health reports readiness, free slots and memory; it must be cheap.
	Health(ctx context.Context) (Health, error)
	// Chat streams a completion. The returned channel follows the Event
	// contract documented on Event and is closed when the stream ends.
	Chat(ctx context.Context, req ChatRequest) (<-chan Event, error)
	// Tokenize returns the token ids of text under the model's tokenizer.
	Tokenize(ctx context.Context, text string) ([]int32, error)
	// Embeddings returns one vector per input; ErrUnsupported for chat-only runners.
	Embeddings(ctx context.Context, texts []string) ([][]float32, error)
	// Metrics returns the latest throughput and occupancy figures.
	Metrics() Metrics
	// Stop terminates the runner gracefully, then forcibly after a timeout.
	// Stop is idempotent.
	Stop(ctx context.Context) error
}

// Health is a point-in-time state of a runner.
type Health struct {
	Ready      bool           `json:"ready"`
	ModelRef   string         `json:"model_ref"`
	SlotsTotal int            `json:"slots_total"`
	SlotsIdle  int            `json:"slots_idle"`
	Memory     MemoryEstimate `json:"memory"`  // measured when the engine reports it
	Message    string         `json:"message"` // human-readable detail when not ready
}

// Metrics are the latest measurements of a runner.
type Metrics struct {
	DecodeTPS     float64        `json:"decode_tps"`
	PrefillTPS    float64        `json:"prefill_tps"`
	TTFT          time.Duration  `json:"ttft"`
	SlotsInUse    int            `json:"slots_in_use"`
	RequestsTotal int64          `json:"requests_total"`
	Memory        MemoryEstimate `json:"memory"`
}
