package metrics

import "github.com/prometheus/client_golang/prometheus"

// RegisterPrometheusMetrics register all prometheus metrics with the global
// metrics handler.
func RegisterPrometheusMetrics() {
	prometheus.Register(BlockImportTimeHistogramSeconds)
	prometheus.Register(CumulativeImportTimeCounter)
	prometheus.Register(CumulativeTransactionsCounter)
	prometheus.Register(ImportedTransactionsPerBlockHistogram)
	prometheus.Register(CurrentRoundGauge)
}

const (

	// ImportTimePerBlockHistogramName metric name.
	ImportTimePerBlockHistogramName = "average_import_time_sec"
	// ImportTimeCounterName metric name.
	ImportTimeCounterName = "cumulative_import_time_sec"
	// TransactionsPerBlockHistogramName metric name.
	TransactionsPerBlockHistogramName = "average_imported_tx_per_block"
	// ImportedTransactionsCounterName metric name.
	ImportedTransactionsCounterName = "cumulative_imported_tx"
	// CurrentRoundGaugeName metric name.
	CurrentRoundGaugeName = "current_round"
)

var (
	// BlockImportTimeHistogramSeconds average block import duration in seconds.
	BlockImportTimeHistogramSeconds = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Subsystem: "indexer_daemon",
			Name:      ImportTimePerBlockHistogramName,
			Help:      "Block import and processing time in seconds.",
		})

	// CumulativeImportTimeCounter total time spent importing blocks since indexer was launched.
	CumulativeImportTimeCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Subsystem: "indexer_daemon",
			Name:      ImportTimeCounterName,
			Help:      "Total time spent importing blocks in seconds.",
		})

	// ImportedTransactionsPerBlockHistogram average number of transactions per block.
	ImportedTransactionsPerBlockHistogram = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Subsystem: "indexer_daemon",
			Name:      TransactionsPerBlockHistogramName,
			Help:      "Transactions per block.",
		})

	// CumulativeTransactionsCounter total number of transactions imported since indexer was launched.
	CumulativeTransactionsCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Subsystem: "indexer_daemon",
			Name:      ImportedTransactionsCounterName,
			Help:      "Cumulative transactions imported.",
		})

	// CurrentRoundGauge current processed round.
	CurrentRoundGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Subsystem: "indexer_daemon",
			Name:      CurrentRoundGaugeName,
			Help:      "The most recent round indexer has imported.",
		})
)
