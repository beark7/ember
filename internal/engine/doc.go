// Package engine defines the Engine and Runner contracts every inference
// backend implements and the Manager that owns runner processes (lazy load,
// LRU eviction, restart with backoff). Adapters live in subpackages.
// See docs-private/design/engine.md and ADR-003/015.
package engine
