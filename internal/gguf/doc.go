// Package gguf parses GGUF headers (v2 and v3) from an io.ReaderAt without
// loading tensor data: metadata, tensor table, per-architecture accessors and
// exact weight sizes. See docs-private/design/catalog.md.
package gguf
