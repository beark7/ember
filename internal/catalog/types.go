package catalog

// ModelDescriptor is what the planner and the engines need to know about a
// model. Only the fields needed by the engine contracts exist yet; the full
// structure of docs-private/design/catalog.md arrives with phases 2 and 4.
type ModelDescriptor struct {
	ID           string   `json:"id"`     // e.g. "qwen3-14b"
	Family       string   `json:"family"` // qwen3, llama3, gemma3, ...
	Arch         string   `json:"arch"`   // gguf general.architecture
	Params       int64    `json:"params"`
	ActiveParams int64    `json:"active_params,omitempty"`
	ContextMax   int      `json:"context_max"`
	Capabilities []string `json:"capabilities,omitempty"`
	Curated      bool     `json:"curated"`
}
