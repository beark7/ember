// Package sched queues requests per runner with a bounded number of parallel
// slots, FIFO with interactive priority and per-user fair share, and returns
// 429/503 semantics to the server. See docs-private/design/server.md.
package sched
