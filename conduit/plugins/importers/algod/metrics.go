package algodimporter

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/algorand/indexer/conduit"
)

// getAlgodRawBlockTimeSeconds is used to record how long it took to fetch the block.
var getAlgodRawBlockTimeSeconds = initGetAlgodRawBlockTimeSeconds(conduit.DefaultMetricsPrefix)

func initGetAlgodRawBlockTimeSeconds(subsystem string) prometheus.Summary {
	return prometheus.NewSummary(
		prometheus.SummaryOpts{
			Subsystem: subsystem,
			Name:      "get_algod_raw_block_time_sec",
			Help:      "Total response time from Algod's raw block endpoint in seconds.",
		})
}
