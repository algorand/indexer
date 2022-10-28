package blockprocessor

import "github.com/prometheus/client_golang/prometheus"

const (
	EvalTimeName = "eval_time_sec"
)

var (
	EvalTimeSeconds = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Subsystem: "indexer_daemon",
			Name:      EvalTimeName,
			Help:      "Time spent calling Eval function in seconds.",
		})
)
