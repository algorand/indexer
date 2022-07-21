package plugins

import (
	"github.com/algorand/go-algorand/util"
	"github.com/algorand/indexer/config"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"path/filepath"
)

// PluginConfig is a generic string which can be deserialized by each individual Plugin
type PluginConfig string

// LoadConfig attempts to retrieve plugin configuration from the expected filesystem path.
// Failure to load a config file will _NOT_ cause an error (since some plugins don't require
// additional config values.
// Unexpected errors such as permissions errors on existing config files will be logged as warnings.
func LoadConfig(log *logrus.Logger, indexerDataDir string, meta PluginMetadata) PluginConfig {
	var configs PluginConfig
	resolvedConfigFileName := meta.Name() + "." + config.FileType
	resolvedConfigPath := filepath.Join(indexerDataDir, resolvedConfigFileName)
	if !util.FileExists(resolvedConfigPath) {
		log.Infof("Did not find config file for %s plugin. Continuing with empty config.", meta.Name())
		return configs
	}
	buf, err := ioutil.ReadFile(resolvedConfigPath)
	if err == nil {
		log.Infof("Loaded config file %s", resolvedConfigPath)
		return PluginConfig(buf)
	}
	log.Warnf("Found config file %s, but failed to read it into memory.", resolvedConfigPath)
	return configs
}
