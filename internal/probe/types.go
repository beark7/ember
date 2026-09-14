package probe

import "time"

// HardwareProfile is the measured description of a machine. Only the fields
// needed by the engine contracts exist yet; the full structure of
// docs-private/design/probe.md arrives with phase 3.
type HardwareProfile struct {
	SchemaVersion int        `json:"schema_version"`
	CollectedAt   time.Time  `json:"collected_at"`
	Hash          string     `json:"hash"`
	OS            OSInfo     `json:"os"`
	CPU           CPUInfo    `json:"cpu"`
	Memory        MemoryInfo `json:"memory"`
	GPUs          []GPUInfo  `json:"gpus"`
	Warnings      []string   `json:"warnings,omitempty"`
}

// OSInfo identifies the operating system.
type OSInfo struct {
	Name    string `json:"name"`    // linux, darwin, windows
	Version string `json:"version"` // e.g. "14.5", "6.9", "10.0.22631"
	Arch    string `json:"arch"`    // amd64, arm64
}

// CPUInfo describes the processor.
type CPUInfo struct {
	Vendor        string   `json:"vendor"`
	Model         string   `json:"model"`
	PhysicalCores int      `json:"physical_cores"`
	LogicalCores  int      `json:"logical_cores"`
	Flags         []string `json:"flags,omitempty"` // avx2, avx512f, amx, neon, sve ...
}

// MemoryInfo describes system memory in bytes.
type MemoryInfo struct {
	Total     int64 `json:"total"`
	Available int64 `json:"available"`
	Unified   bool  `json:"unified"`
}

// GPUInfo describes one GPU as seen by the engine backends.
type GPUInfo struct {
	Vendor     string `json:"vendor"` // nvidia, amd, intel, apple
	Name       string `json:"name"`
	VRAMTotal  int64  `json:"vram_total"`
	VRAMFree   int64  `json:"vram_free"`
	Unified    bool   `json:"unified"`
	Driver     string `json:"driver,omitempty"`
	ComputeCap string `json:"compute_cap,omitempty"`
}
