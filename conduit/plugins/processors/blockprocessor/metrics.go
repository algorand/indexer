package blockprocessor

import "github.com/prometheus/client_golang/prometheus"

// evalTimeSeconds is used to record how much time is spent running eval.
var evalTimeSeconds prometheus.Summary
