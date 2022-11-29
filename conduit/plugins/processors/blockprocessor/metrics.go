package blockprocessor

import "github.com/prometheus/client_golang/prometheus"

// Prometheus metric names
const (
	EvalTimeName = "eval_time_sec"
)

// Initialize the prometheus objects
var (
	EvalTimeSeconds = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Subsystem: "indexer_daemon",
			Name:      EvalTimeName,
			Help:      "Time spent calling Eval function in seconds.",
		})
)
