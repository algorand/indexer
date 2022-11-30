package metrics

import "github.com/prometheus/client_golang/prometheus"

// RegisterPrometheusMetrics register all prometheus metrics with the global
// metrics handler.
func RegisterPrometheusMetrics() {
	for _, c := range collectors {
		_ = prometheus.Register(c)
	}
}

// Prometheus metric names broken out for reuse.
const (
	BlockImportTimeName      = "import_time_sec"
	ImportedTxnsPerBlockName = "imported_tx_per_block"
	ImportedRoundGaugeName   = "imported_round"
	GetAlgodRawBlockTimeName = "get_algod_raw_block_time_sec"
	ImportedTxnsName         = "imported_txns"
	ImporterTimeName         = "importer_time_sec"
	ProcessorTimeName        = "processor_time_sec"
	ExporterTimeName         = "exporter_time_sec"
	PipelineRetryCountName   = "pipeline_retry_count"
)

// AllMetricNames is a reference for all the custom metric names.
var AllMetricNames = []string{
	BlockImportTimeName,
	ImportedTxnsPerBlockName,
	ImportedRoundGaugeName,
	GetAlgodRawBlockTimeName,
	ImporterTimeName,
	ProcessorTimeName,
	ExporterTimeName,
	PipelineRetryCountName,
}

// Initialize the prometheus objects.
var (
	BlockImportTimeSeconds = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Subsystem: "indexer_daemon",
			Name:      BlockImportTimeName,
			Help:      "Total block upload and processing time in seconds.",
		})

	ImportedTxnsPerBlock = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Subsystem: "indexer_daemon",
			Name:      ImportedTxnsPerBlockName,
			Help:      "Transactions per block.",
		},
	)

	ImportedTxns = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: "indexer_daemon",
			Name:      ImportedTxnsName,
			Help:      "Imported transactions grouped by type",
		},
		[]string{"txn_type"},
	)

	ImportedRoundGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Subsystem: "indexer_daemon",
			Name:      ImportedRoundGaugeName,
			Help:      "The most recent round indexer has imported.",
		})

	GetAlgodRawBlockTimeSeconds = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Subsystem: "indexer_daemon",
			Name:      GetAlgodRawBlockTimeName,
			Help:      "Total response time from Algod's raw block endpoint in seconds.",
		})

	ImporterTimeSeconds = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Subsystem: "indexer_daemon",
			Name:      ImporterTimeName,
			Help:      "Time spent at importer step",
		})

	ProcessorTimeSeconds = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Subsystem: "indexer_daemon",
			Name:      ProcessorTimeName,
			Help:      "Time spent running a processor",
		},
		[]string{"processor_name"},
	)

	ExporterTimeSeconds = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Subsystem: "indexer_daemon",
			Name:      ExporterTimeName,
			Help:      "Time spent at exporter step",
		})

	PipelineRetryCount = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Subsystem: "indexer_daemon",
			Name:      PipelineRetryCountName,
			Help:      "Total pipeline retries since last successful run",
		})
)

var collectors = []prometheus.Collector{
	BlockImportTimeSeconds,
	BlockImportTimeSeconds,
	ImportedTxnsPerBlock,
	ImportedRoundGauge,
	GetAlgodRawBlockTimeSeconds,
	ImportedTxns,
	ImporterTimeSeconds,
	ProcessorTimeSeconds,
	ExporterTimeSeconds,
	PipelineRetryCount,
}
