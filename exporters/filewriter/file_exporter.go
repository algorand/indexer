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
	fh     *os.File
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
	exp.fh, err = os.OpenFile(exp.cfg.BlockFilepath, os.O_APPEND|os.O_WRONLY, 0755)
	if err != nil {
		return fmt.Errorf("error opening file: %w", err)
	}
	return err
}

func (exp *fileExporter) Config() plugins.PluginConfig {
	ret, _ := yaml.Marshal(exp.cfg)
	return plugins.PluginConfig(ret)
}

func (exp *fileExporter) Close() error {
	if exp.fh != nil {
		return exp.fh.Close()
	}
	return nil
}

func (exp *fileExporter) Receive(exportData data.BlockData) error {
	//write block to file
	block, err := json.MarshalIndent(exportData, "", " ")
	if err != nil {
		fmt.Errorf("error getting block file stats: %w", err)
	}
	_, err = fmt.Fprintln(exp.fh, block)
	return nil
}

func (exp *fileExporter) HandleGenesis(genesis bookkeeping.Genesis) error {
	// check genesis hash
	gh := crypto.HashObj(genesis).String()
	stat, err := exp.fh.Stat()
	if err != nil {
		return fmt.Errorf("error getting block file stats: %w", err)
	}
	if size := stat.Size(); size == 0 {
		// if block file is empty, record genesis hash
		exp.fh.WriteString(fmt.Sprintf("# Genesis Hash:%s", gh))
	} else {
		var genesisTag string
		scanner := bufio.NewScanner(exp.fh)
		for scanner.Scan() {
			genesisTag = scanner.Text()
			break
		}
		if err = scanner.Err(); err != nil {
			return fmt.Errorf("error reading file: %w", err)
		}
		ghFromFile := strings.Split(genesisTag, ":")[1]
		if ghFromFile != gh {
			return fmt.Errorf("genesis hash in file %s does not match expected value. genesis hash %s, expected %s ", exp.cfg.BlockFilepath, ghFromFile, gh)
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
