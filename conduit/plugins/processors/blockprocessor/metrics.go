package blockprocessor

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/algorand/indexer/conduit"
)

// evalTimeSeconds is used to record how much time is spent running eval.
var evalTimeSeconds = initEvalTimeSeconds(conduit.DefaultMetricsPrefix)

func initEvalTimeSeconds(subsystem string) prometheus.Summary {
	return prometheus.NewSummary(
		prometheus.SummaryOpts{
			Subsystem: subsystem,
			Name:      "eval_time_sec",
			Help:      "Time spent calling Eval function in seconds.",
		})
}
