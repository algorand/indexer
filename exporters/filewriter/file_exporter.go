package filewriter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/exporters"
	"github.com/algorand/indexer/plugins"
)

const exporterName = "file_writer"

type fileExporter struct {
	round  uint64
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
	return &fileExporter{}
}

func (exp *fileExporter) Metadata() exporters.ExporterMetadata {
	return fileExporterMetadata
}

func (exp *fileExporter) Init(_ context.Context, initProvider data.InitProvider, cfg plugins.PluginConfig, logger *logrus.Logger) error {
	exp.logger = logger
	if err := exp.unmarhshalConfig(string(cfg)); err != nil {
		return fmt.Errorf("connect failure in unmarshalConfig: %w", err)
	}
	// create block directory
	err := os.Mkdir(exp.cfg.BlocksDir, 0755)
	if err != nil && errors.Is(err, os.ErrExist) {
	} else if err != nil {
		return fmt.Errorf("Init() error: %w", err)
	}
	exp.round = uint64(initProvider.NextDBRound())
	return err
}

func (exp *fileExporter) Config() plugins.PluginConfig {
	ret, _ := yaml.Marshal(exp.cfg)
	return plugins.PluginConfig(ret)
}

func (exp *fileExporter) Close() error {
	exp.logger.Infof("latest round on file: %d", exp.round)
	return nil
}

func (exp *fileExporter) Receive(exportData data.BlockData) error {
	if exp.logger == nil {
		return fmt.Errorf("exporter not initialized")
	}
	if exportData.Round() != exp.round {
		return fmt.Errorf("Receive(): wrong block: received round %d, expected round %d", exportData.Round(), exp.round)
	}
	//write block to file
	blockFile := path.Join(exp.cfg.BlocksDir, fmt.Sprintf("block_%d.json", exportData.Round()))
	file, err := os.OpenFile(blockFile, os.O_WRONLY|os.O_CREATE, 0755)
	if err != nil {
		return fmt.Errorf("Receive(): error opening file %s: %w", blockFile, err)
	}
	defer file.Close()
	err = json.NewEncoder(file).Encode(exportData)
	if err != nil {
		return fmt.Errorf("Receive(): error encoding exportData: %w", err)
	}
	exp.logger.Infof("Added block %d", exportData.Round())
	exp.round++
	return nil
}

func (exp *fileExporter) unmarhshalConfig(cfg string) error {
	return yaml.Unmarshal([]byte(cfg), &exp.cfg)
}

func init() {
	exporters.RegisterExporter(exporterName, &Constructor{})
}
