package conduit

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/algorand/indexer/data"
)

// OnCompleteFunc is the signature for the Completed functional interface.
type OnCompleteFunc func(input data.BlockData) error

// Completed is called by the conduit pipeline after every exporter has
// finished. It can be used for things like finalizing state.
type Completed interface {
	// OnComplete will be called by the Conduit framework when the pipeline
	// finishes processing a round.
	OnComplete(input data.BlockData) error
}

// ProvideMetricsFunc is the signature for the PluginMetrics interface.
type ProvideMetricsFunc func() []prometheus.Collector

// PluginMetrics is for defining plugin specific metrics
type PluginMetrics interface {
	ProvideMetrics() []prometheus.Collector
}
