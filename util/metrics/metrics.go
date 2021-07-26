package metrics

import "github.com/prometheus/client_golang/prometheus"

// RegisterPrometheusMetrics register all prometheus metrics with the global
// metrics handler.
func RegisterPrometheusMetrics() {
	prometheus.Register(BlockImportTimeSeconds)
	prometheus.Register(CumulativeImportTime)
	prometheus.Register(CumulativeTxns)
	prometheus.Register(ImportedTxnsPerBlock)
	prometheus.Register(CurrentRoundGauge)
	prometheus.Register(CumulativeBlockUploadTime)
	prometheus.Register(BlockUploadTime)
}

// Prometheus metric names broken out for reuse.
const (
	BlockImportTimeName           = "average_import_time_sec"
	CumulativeImportTimeName      = "cumulative_import_time_sec"
	BlockUploadTimeName           = "average_block_upload_time_sec"
	CumulativeBlockUploadTimeName = "cumulative_block_upload_time_sec"
	ImportedTxnsPerBlockName      = "average_imported_tx_per_block"
	CumulativeTxnsName            = "cumulative_imported_tx"
	CurrentRoundGaugeName         = "current_round"
)

// Initialize the prometheus objects.
var (
	// AllMetricNames is a reference for all the custom metric names.
	AllMetricNames = []string{
		BlockImportTimeName,
		CumulativeImportTimeName,
		BlockUploadTimeName,
		CumulativeBlockUploadTimeName,
		ImportedTxnsPerBlockName,
		CumulativeTxnsName,
		CurrentRoundGaugeName}

	BlockImportTimeSeconds = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Subsystem: "indexer_daemon",
			Name:      BlockImportTimeName,
			Help:      "Total block upload and processing time in seconds.",
		})

	CumulativeImportTime = prometheus.NewCounter(
		prometheus.CounterOpts{
			Subsystem: "indexer_daemon",
			Name:      CumulativeImportTimeName,
			Help:      "Total time in seconds spent uploading and processing blocks.",
		})

	BlockUploadTime = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Subsystem: "indexer_daemon",
			Name:      BlockUploadTimeName,
			Help:      "Block upload time in seconds.",
		})

	CumulativeBlockUploadTime = prometheus.NewCounter(
		prometheus.CounterOpts{
			Subsystem: "indexer_daemon",
			Name:      CumulativeBlockUploadTimeName,
			Help:      "Total time in seconds spent uploading blocks.",
		})

	ImportedTxnsPerBlock = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Subsystem: "indexer_daemon",
			Name:      ImportedTxnsPerBlockName,
			Help:      "Transactions per block.",
		})

	CumulativeTxns = prometheus.NewCounter(
		prometheus.CounterOpts{
			Subsystem: "indexer_daemon",
			Name:      CumulativeTxnsName,
			Help:      "Cumulative transactions imported.",
		})

	CurrentRoundGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Subsystem: "indexer_daemon",
			Name:      CurrentRoundGaugeName,
			Help:      "The most recent round indexer has imported.",
		})
)
