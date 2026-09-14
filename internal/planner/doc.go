// Package planner turns a HardwareProfile, a ModelDescriptor and the user's
// requirements into ranked Plans: weight placement per memory tier, memory and
// speed estimates, quantization choice, explanations, and per-node Capacity for
// k concurrent users. It calibrates from real runs. See docs-private/design/planner.md.
package planner
