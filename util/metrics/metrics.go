package metrics

import "github.com/prometheus/client_golang/prometheus"

// This is helpful for tests to ensure there are never uninitialized values.
func init() {
	RegisterPrometheusMetrics("uninitialized")
}

// RegisterPrometheusMetrics register all prometheus metrics with the global
// metrics handler.
func RegisterPrometheusMetrics(subsystem string) {
	// deregister metric objects in case the register function was called more than once.
	// This helps with testing.
	deregister()
	instantiateCollectors(subsystem)

	_ = prometheus.Register(GetAlgodRawBlockTimeSeconds)
	_ = prometheus.Register(BlockImportTimeSeconds)
	_ = prometheus.Register(BlockImportTimeSeconds)
	_ = prometheus.Register(ImportedTxnsPerBlock)
	_ = prometheus.Register(ImportedRoundGauge)
	_ = prometheus.Register(ImportedTxns)
	_ = prometheus.Register(ImporterTimeSeconds)
	_ = prometheus.Register(ProcessorTimeSeconds)
	_ = prometheus.Register(ExporterTimeSeconds)
	_ = prometheus.Register(PipelineRetryCount)
}
func deregister() {
	// Use ImportedTxns as a sentinel value. None or all should be initialized.
	if ImportedTxns != nil {
		prometheus.Unregister(GetAlgodRawBlockTimeSeconds)
		prometheus.Unregister(BlockImportTimeSeconds)
		prometheus.Unregister(BlockImportTimeSeconds)
		prometheus.Unregister(ImportedTxnsPerBlock)
		prometheus.Unregister(ImportedRoundGauge)
		prometheus.Unregister(ImportedTxns)
		prometheus.Unregister(ImporterTimeSeconds)
		prometheus.Unregister(ProcessorTimeSeconds)
		prometheus.Unregister(ExporterTimeSeconds)
		prometheus.Unregister(PipelineRetryCount)
	}
}

func instantiateCollectors(subsystem string) {
	// GetAlgodRawBlockTimeSeconds is used by fetcher
	GetAlgodRawBlockTimeSeconds = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Subsystem: subsystem,
			Name:      GetAlgodRawBlockTimeName,
			Help:      "Total response time from Algod's raw block endpoint in seconds.",
		})

	BlockImportTimeSeconds = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Subsystem: subsystem,
			Name:      BlockImportTimeName,
			Help:      "Total block upload and processing time in seconds.",
		})

	ImportedTxnsPerBlock = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Subsystem: subsystem,
			Name:      ImportedTxnsPerBlockName,
			Help:      "Transactions per block.",
		},
	)

	ImportedTxns = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: subsystem,
			Name:      ImportedTxnsName,
			Help:      "Imported transactions grouped by type",
		},
		[]string{"txn_type"},
	)

	ImportedRoundGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Subsystem: subsystem,
			Name:      ImportedRoundGaugeName,
			Help:      "The most recent round indexer has imported.",
		})

	ImporterTimeSeconds = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Subsystem: subsystem,
			Name:      ImporterTimeName,
			Help:      "Time spent at importer step",
		})

	ProcessorTimeSeconds = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Subsystem: subsystem,
			Name:      ProcessorTimeName,
			Help:      "Time spent running a processor",
		},
		[]string{"processor_name"},
	)

	ExporterTimeSeconds = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Subsystem: subsystem,
			Name:      ExporterTimeName,
			Help:      "Time spent at exporter step",
		})

	PipelineRetryCount = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Subsystem: subsystem,
			Name:      PipelineRetryCountName,
			Help:      "Total pipeline retries since last successful run",
		})
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
	// used by fetcher

	GetAlgodRawBlockTimeSeconds prometheus.Summary

	// used by pipeline

	BlockImportTimeSeconds prometheus.Summary
	ImportedTxnsPerBlock   prometheus.Summary
	ImportedTxns           *prometheus.GaugeVec
	ImportedRoundGauge     prometheus.Gauge
	ImporterTimeSeconds    prometheus.Summary
	ProcessorTimeSeconds   *prometheus.SummaryVec
	ExporterTimeSeconds    prometheus.Summary
	PipelineRetryCount     prometheus.Histogram
)
