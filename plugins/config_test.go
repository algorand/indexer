package plugins

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

type mockMetadata struct{}

func (m *mockMetadata) Type() PluginType {
	return "foobar"
}

func (m *mockMetadata) Name() string {
	return "baz"
}

var logger *logrus.Logger
var hook *test.Hook

func init() {
	logger, hook = test.NewNullLogger()
}

func createTempDir(t *testing.T) string {
	dir, err := os.MkdirTemp("", "indexer")
	if err != nil {
		t.Fatalf(err.Error())
	}
	return dir
}

func TestLoadConfigSuccess(t *testing.T) {
	defer hook.Reset()
	indexerDataDir := createTempDir(t)
	defer os.RemoveAll(indexerDataDir)
	configFile := filepath.Join(indexerDataDir + "/baz.yml")
	toWrite := []byte("foo: bar")
	os.WriteFile(configFile, toWrite, fs.ModePerm)
	assert.Equal(t, PluginConfig("foo: bar"), LoadConfig(logger, indexerDataDir, &mockMetadata{}))
	assert.Equal(t, 1, len(hook.Entries))
	assert.Equal(t, logrus.InfoLevel, hook.LastEntry().Level)
	logMessage := fmt.Sprintf("Loaded config file %s", configFile)
	assert.Equal(t, logMessage, hook.LastEntry().Message)
}

func TestLoadConfigDoesNotExist(t *testing.T) {
	defer hook.Reset()
	assert.Equal(t, PluginConfig(""), LoadConfig(logger, "doesNotExist", &mockMetadata{}))
	assert.Equal(t, 1, len(hook.Entries))
	assert.Equal(t, logrus.InfoLevel, hook.LastEntry().Level)
	assert.Equal(t, "Did not find config file for baz plugin. Continuing with empty config.", hook.LastEntry().Message)
}

func TestLoadConfigDoesNotRead(t *testing.T) {
	defer hook.Reset()
	indexerDataDir := createTempDir(t)
	defer os.RemoveAll(indexerDataDir)
	configFile := filepath.Join(indexerDataDir + "/baz.yml")
	os.WriteFile(configFile, []byte{}, fs.FileMode(0000))
	assert.Equal(t, PluginConfig(""), LoadConfig(logger, indexerDataDir, &mockMetadata{}))
	assert.Equal(t, 1, len(hook.Entries))
	assert.Equal(t, logrus.WarnLevel, hook.LastEntry().Level)
	logMessage := fmt.Sprintf("Found config file %s, but failed to read it into memory.", configFile)
	assert.Equal(t, logMessage, hook.LastEntry().Message)
}

func TestPluginConfigPath(t *testing.T) {
	indexerDataDir := "foo"
	expectedPath := filepath.Join(indexerDataDir, "baz.yml")
	assert.Equal(t, expectedPath, PluginConfigPath(indexerDataDir, &mockMetadata{}))
}
