package networkimporter

// ImporterConfig specific to the network importer
type ImporterConfig struct {
	// Path to genesis file
	GenesisPath string `yaml:"genesis-path"`
	// Path to config file
	ConfigPath string `yaml:"config-path"`
}
