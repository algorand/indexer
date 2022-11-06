package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gopkg.in/yaml.v3"
)

import (
	_ "github.com/algorand/indexer/conduit/plugins/exporters/all"
	_ "github.com/algorand/indexer/conduit/plugins/exporters/example"
	_ "github.com/algorand/indexer/conduit/plugins/importers/all"
	_ "github.com/algorand/indexer/conduit/plugins/processors/all"
)

// TestSamples ensures that all plugins contain a sample file with valid yaml.
func TestSamples(t *testing.T) {
	metadata := AllMetadata()
	for _, data := range metadata {
		data := data
		t.Run(data.Name, func(t *testing.T) {
			t.Parallel()
			var config NameConfigPair
			assert.NoError(t, yaml.Unmarshal([]byte(data.SampleConfig), &config))
			assert.Equal(t, data.Name, config.Name)
		})
	}
}
