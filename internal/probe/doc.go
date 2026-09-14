// Package probe builds a measured HardwareProfile of the machine: system specs
// parsed from OS tools, backend devices enumerated through the engine binary
// (llama-server --list-devices) and microbenchmarks. Collectors are interfaces
// with fixture-backed implementations for tests. See docs-private/design/probe.md.
package probe
