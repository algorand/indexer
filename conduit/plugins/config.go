package plugins

import "gopkg.in/yaml.v3"

// PluginConfig is a generic string which can be deserialized by each individual Plugin
type PluginConfig struct {
	// DataDir available to this plugin.
	DataDir string
	// Config specific to this plugin.
	Config string
}

// UnmarshalConfig attempts to Unmarshal the plugin config into an object.
func (pc PluginConfig) UnmarshalConfig(config interface{}) error {
	return yaml.Unmarshal([]byte(pc.Config), config)
}

// MakePluginConfig is a helper to create the struct.
func MakePluginConfig(config string) PluginConfig {
	return PluginConfig{Config: config}
}
