package algodimporter

import "github.com/prometheus/client_golang/prometheus"

const (
	GetAlgodRawBlockTimeName = "get_algod_raw_block_time_sec"
)

var (
	GetAlgodRawBlockTimeSeconds = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Subsystem: "indexer_daemon",
			Name:      GetAlgodRawBlockTimeName,
			Help:      "Total response time from Algod's raw block endpoint in seconds.",
		})
)
