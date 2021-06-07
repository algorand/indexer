package middlewares

import (
	"net/http"
	"sort"

	"github.com/labstack/echo/v4"
)

// PrometheusPathMapper404Sink if the status is 404, truncate the path to ensure no personal data is leaked.
func PrometheusPathMapper404Sink(c echo.Context) string {
	// It is easy to include private data in invalid endpoint URLs, so don't include them.
	// For example "/v2/accounts/<addr>/" should not have the trailing slash.
	if c.Response().Status == http.StatusNotFound {
		return ""
	}

	return c.Path()
}

// PrometheusPathMapperVerbose adds query parameters to the path in addition to the PrometheusPathMapper404Sink.
func PrometheusPathMapperVerbose(c echo.Context) string {
	if PrometheusPathMapper404Sink(c) == "" {
		return ""
	}

	// Sort the parameters
	keys := make([]string, 0, len(c.QueryParams()))
	for k := range c.QueryParams() {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	path := c.Path()
	sep := "?"
	for _, k := range keys {
		path += sep + k
		sep = "&"
	}

	return path
}
