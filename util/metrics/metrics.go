package metrics

import "github.com/prometheus/client_golang/prometheus"

func RegisterPrometheusMetrics() {
	prometheus.Register(ImportTimeHistogramSeconds)
	prometheus.Register(ImportTimeCounter)
	prometheus.Register(ImportedTransactionsCounter)
	prometheus.Register(ImportedTransactionsHistogram)
}

const ImportTimeHistogramName = "import_time_sec"
const ImportTimeCounterName = "cumulative_import_time_milli_sec"
const ImportedTransactionsHistogramName = "imported_tx_per_sec"
const ImportedTransactionsCounterName = "cumulative_imported_tx"

// ImportTimeHistogramSeconds average block import duration in seconds.
var ImportTimeHistogramSeconds = prometheus.NewSummary(
	prometheus.SummaryOpts{
		Subsystem: "indexer_daemon",
		Name:      ImportTimeHistogramName,
		Help:      "Block import and processing time in seconds.",
	})

// ImportTimeCounter total time spent importing blocks since indexer was launched.
var ImportTimeCounter = prometheus.NewCounter(
	prometheus.CounterOpts{
		Subsystem: "indexer_daemon",
		Name:      ImportTimeCounterName,
		Help:      "Total time spent importing blocks in milli seconds.",
	})

// ImportedTransactionsHistogram average number of transactions per block.
var ImportedTransactionsHistogram = prometheus.NewSummary(
	prometheus.SummaryOpts{
		Subsystem: "indexer_daemon",
		Name:      ImportedTransactionsHistogramName,
		Help:      "Block import and processing time in seconds.",
	})

// ImportedTransactionsCounter total number of transactions imported since indexer was launched.
var ImportedTransactionsCounter = prometheus.NewCounter(
	prometheus.CounterOpts{
		Subsystem: "indexer_daemon",
		Name:      ImportedTransactionsCounterName,
		Help:      "Total transactions imported.",
	})

