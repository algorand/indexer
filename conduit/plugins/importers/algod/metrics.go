package algodimporter

import "github.com/prometheus/client_golang/prometheus"

// getAlgodRawBlockTimeSeconds is used to record how long it took to fetch the block.
var getAlgodRawBlockTimeSeconds prometheus.Summary
