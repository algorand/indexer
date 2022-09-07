package filewriter

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/exporters"
	"github.com/algorand/indexer/plugins"
	"github.com/sirupsen/logrus"
)

const exporterName = "filewriter"

type fileExporter struct {
	round  uint64
	fd     *os.File
	cfg    ExporterConfig
	logger *logrus.Logger
}

var fileExporterMetadata = exporters.ExporterMetadata{
	ExpName:        exporterName,
	ExpDescription: "Exporter for writing data to a file.",
	ExpDeprecated:  false,
}

// Constructor is the ExporterConstructor implementation for the filewriter exporter
type Constructor struct{}

// New initializes a fileExporter
func (c *Constructor) New() exporters.Exporter {
	return &fileExporter{
		round: 0,
	}
}

func (exp *fileExporter) Metadata() exporters.ExporterMetadata {
	return fileExporterMetadata
}

func (exp *fileExporter) Init(cfg plugins.PluginConfig, logger *logrus.Logger) error {
	exp.logger = logger
	var err error
	if err = exp.unmarhshalConfig(string(cfg)); err != nil {
		return fmt.Errorf("connect failure in unmarshalConfig: %w", err)
	}
	exp.fd, err = os.OpenFile(exp.cfg.BlockFilepath, os.O_APPEND|os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		return fmt.Errorf("error opening file: %w", err)
	}
	exp.round = exp.cfg.Round
	return err
}

func (exp *fileExporter) Config() plugins.PluginConfig {
	ret, _ := yaml.Marshal(exp.cfg)
	return plugins.PluginConfig(ret)
}

func (exp *fileExporter) Close() error {
	exp.logger.Infof("latest round on file: %d", exp.round)
	if err := updateRoundInConfigs(exp.cfg.ConfigFilePath, exp.cfg, exp.round); err != nil {
		exp.logger.Warnf("unable to update round in configuration file %s", exp.cfg.ConfigFilePath)
	}
	if exp.fd != nil {
		err := exp.fd.Close()
		if err != nil {
			return fmt.Errorf("error closing file %s: %w", exp.fd.Name(), err)
		}
	}
	return nil
}

func (exp *fileExporter) Receive(exportData data.BlockData) error {
	if exp.fd == nil {
		return fmt.Errorf("exporter not initialized")
	}
	if exportData.Round() != exp.round {
		return fmt.Errorf("wrong block. received round %d, expected round %d", exportData.Round(), exp.round)
	}
	//write block to file
	blockData, err := json.Marshal(exportData)
	if err != nil {
		return fmt.Errorf("error serializing block data: %w", err)
	}
	_, err = fmt.Fprintln(exp.fd, string(blockData))
	exp.logger.Infof("Added block %d", exportData.Round())
	exp.round++
	return nil
}

func (exp *fileExporter) HandleGenesis(genesis bookkeeping.Genesis) error {
	// check genesis hash
	gh := crypto.HashObj(genesis).String()
	stat, err := exp.fd.Stat()
	if err != nil {
		return fmt.Errorf("error getting block file stats: %w", err)
	}
	if size := stat.Size(); size == 0 {
		// if block file is empty, record genesis hash
		fmt.Fprintln(exp.fd, fmt.Sprintf("# Genesis Hash:%s", gh))
	} else {
		var genesisTag string
		scanner := bufio.NewScanner(exp.fd)
		for scanner.Scan() {
			genesisTag = scanner.Text()
			break
		}
		if err = scanner.Err(); err != nil {
			return fmt.Errorf("error reading file: %w", err)
		}
		ghFromFile := strings.Split(genesisTag, ":")[1]
		if ghFromFile != gh {
			return fmt.Errorf("genesis hash in file %s does not match expected value. actual %s, expected %s ", exp.cfg.BlockFilepath, gh, ghFromFile)
		}
	}
	return nil
}

func (exp *fileExporter) Round() uint64 {
	return exp.round
}

func (exp *fileExporter) unmarhshalConfig(cfg string) error {
	return yaml.Unmarshal([]byte(cfg), &exp.cfg)
}

func init() {
	exporters.RegisterExporter(exporterName, &Constructor{})
}

func updateRoundInConfigs(path string, cfg ExporterConfig, round uint64) error {
	fd, err := os.OpenFile(path, os.O_WRONLY, 0755)
	if err != nil {
		return fmt.Errorf("error opening file: %w", err)
	}
	defer fd.Close()
	exporterConfigs := ExporterConfig{
		Round:          round,
		BlockFilepath:  cfg.ConfigFilePath,
		ConfigFilePath: cfg.ConfigFilePath,
	}

	ret, _ := yaml.Marshal(exporterConfigs)
	_, err = fd.WriteString(string(ret))
	if err != nil {
		return fmt.Errorf("error writing configurations: %w", err)
	}
	return nil
}
