package filterprocessor

import (
	"context"
	"gopkg.in/yaml.v3"
	"os"
	"reflect"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"github.com/algorand/indexer/conduit"
	"github.com/algorand/indexer/conduit/plugins"
	"github.com/algorand/indexer/conduit/plugins/processors"
)

// TestFilterProcessorSampleConfigInit validates that all fields in the sample config are valid for a filter processor
func TestFilterProcessorSampleConfigInit(t *testing.T) {

	fpBuilder, err := processors.ProcessorBuilderByName("filter_processor")
	assert.NoError(t, err)

	sampleConfigStr, err := os.ReadFile("sample.yaml")
	assert.NoError(t, err)

	fp := fpBuilder.New()
	err = fp.Init(context.Background(), &conduit.PipelineInitProvider{}, plugins.MakePluginConfig(string(sampleConfigStr)), logrus.New())
	assert.NoError(t, err)
}

// TestFilterProcessorSampleConfigNotEmpty tests that all fields in the sample config are filled in
func TestFilterProcessorSampleConfigNotEmpty(t *testing.T) {

	sampleConfigStr, err := os.ReadFile("sample.yaml")
	assert.NoError(t, err)
	pCfg := Config{}

	err = yaml.Unmarshal(sampleConfigStr, &pCfg)
	assert.NoError(t, err)

	v := reflect.ValueOf(pCfg)

	for i := 0; i < v.NumField(); i++ {
		assert.True(t, v.Field(i).Interface() != nil)
	}

}
