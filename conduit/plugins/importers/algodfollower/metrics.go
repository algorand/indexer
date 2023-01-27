package algodfollower

import "github.com/prometheus/client_golang/prometheus"

// Prometheus metric names
const (
	GetAlgodRawBlockTimeName = "get_algod_raw_block_time_sec"
)

// Initialize the prometheus objects
var (
	GetAlgodRawBlockTimeSeconds = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Subsystem: "algod_follower",
			Name:      GetAlgodRawBlockTimeName,
			Help:      "Total response time from Algod's raw block endpoint in seconds.",
		})
)
