package filterprocessor

import (
	"context"
	"os"
	"testing"

	"github.com/algorand/indexer/conduit"
	"github.com/algorand/indexer/plugins"
	"github.com/algorand/indexer/processors"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// TestSample validates that all fields are set and that the config is valid for the filter processor
func TestFilterProcessorSampleConfig(t *testing.T) {

	fpBuilder, err := processors.ProcessorBuilderByName("filter_processor")
	assert.NoError(t, err)

	sampleConfigStr, err := os.ReadFile("sample.yaml")
	assert.NoError(t, err)

	fp := fpBuilder.New()
	err = fp.Init(context.Background(), &conduit.PipelineInitProvider{}, plugins.PluginConfig(sampleConfigStr), logrus.New())
	assert.NoError(t, err)
}
