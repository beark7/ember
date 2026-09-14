// Package accounting records one usage row per request (tokens, latencies,
// throughput, status, node) and exposes Prometheus metrics; it hands samples
// to the planner for calibration. See docs-private/design/server.md.
package accounting
