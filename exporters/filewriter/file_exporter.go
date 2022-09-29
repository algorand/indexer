package filewriter

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/algorand/go-algorand/agreement"
	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/bookkeeping"

	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/exporters"
	"github.com/algorand/indexer/plugins"
)

const (
	exporterName = "filewriter"
	// FileExporterFileFormat is used to name the output files.
	// Argument index 1: "block" or "delta"
	// Argument index 2: round number
	//
	// The default format places the round first and pads with 0. This way it
	// is easy to sort. At 8.5 million rounds per year, 9 digits of padding
	// should last for over 100 years.
	FileExporterFileFormat = "%09[2]d_%[1]s.json"
)

type fileExporter struct {
	round             uint64
	blockMetadataFile string
	blockMetadata     BlockMetaData
	cfg               ExporterConfig
	logger            *logrus.Logger
}

var fileExporterMetadata = exporters.ExporterMetadata{
	ExpName:        exporterName,
	ExpDescription: "Exporter for writing data to a file.",
	ExpDeprecated:  false,
}

// BlockMetaData contains the metadata for block file storage
type BlockMetaData struct {
	GenesisHash string `json:"genesis-hash"`
	Network     string `json:"network"`
	NextRound   uint64 `json:"next-round"`
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
	if err := exp.unmarhshalConfig(string(cfg)); err != nil {
		return fmt.Errorf("connect failure in unmarshalConfig: %w", err)
	}
	// create block directory
	err := os.Mkdir(exp.cfg.BlocksDir, 0755)
	if err != nil && errors.Is(err, os.ErrExist) {
	} else if err != nil {
		return fmt.Errorf("Init() error: %w", err)
	}
	// initialize block metadata
	file := path.Join(exp.cfg.BlocksDir, "metadata.json")
	exp.blockMetadataFile = file
	var stat os.FileInfo
	if stat, err = os.Stat(file); errors.Is(err, os.ErrNotExist) || (stat != nil && stat.Size() == 0) {
		if stat != nil && stat.Size() == 0 {
			// somehow it did not finish initializing
			err = os.Remove(file)
			if err != nil {
				return fmt.Errorf("Init(): error creating file: %w", err)
			}
		}
		err = exp.encodeMetadataToFile()
		if err != nil {
			return fmt.Errorf("Init(): error creating file: %w", err)
		}
		exp.blockMetadata = BlockMetaData{
			GenesisHash: "",
			Network:     "",
			NextRound:   exp.round,
		}
	} else {
		if err != nil {
			return fmt.Errorf("error opening file: %w", err)
		}
		var data []byte
		data, err = os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("error reading metadata: %w", err)
		}
		err = json.Unmarshal(data, &exp.blockMetadata)
		if err != nil {
			return fmt.Errorf("error reading metadata: %w", err)
		}
	}
	exp.round = exp.blockMetadata.NextRound
	return err
}

func (exp *fileExporter) Config() plugins.PluginConfig {
	ret, _ := yaml.Marshal(exp.cfg)
	return plugins.PluginConfig(ret)
}

func (exp *fileExporter) encodeMetadataToFile() error {
	tempFilename := fmt.Sprintf("%s.temp", exp.blockMetadataFile)
	file, err := os.Create(tempFilename)
	if err != nil {
		return fmt.Errorf("encodeMetadataToFile(): failed to create temp metadata file: %w", err)
	}
	defer file.Close()
	err = json.NewEncoder(file).Encode(exp.blockMetadata)
	if err != nil {
		return fmt.Errorf("encodeMetadataToFile(): failed to write temp metadata: %w", err)
	}

	err = os.Rename(tempFilename, exp.blockMetadataFile)
	if err != nil {
		return fmt.Errorf("encodeMetadataToFile(): failed to replace metadata file: %w", err)
	}

	return nil
}

func (exp *fileExporter) Close() error {
	exp.logger.Infof("latest round on file: %d", exp.round)
	return nil
}

func (exp *fileExporter) Receive(exportData data.BlockData) error {
	if exp.blockMetadataFile == "" {
		return fmt.Errorf("exporter not initialized")
	}
	if exportData.Round() != exp.round {
		return fmt.Errorf("Receive(): wrong block: received round %d, expected round %d", exportData.Round(), exp.round)
	}
	if exportData.Delta == nil && !exp.cfg.ExcludeStateDelta {
		return fmt.Errorf("exporter is misconfigured, set 'exclude-state-delta: true' or enable a plugin that provides state deltas data")
	}

	// write block to file
	{
		blockFile := path.Join(exp.cfg.BlocksDir, fmt.Sprintf(FileExporterFileFormat, "block", exportData.Round()))
		file, err := os.OpenFile(blockFile, os.O_WRONLY|os.O_CREATE, 0755)
		if err != nil {
			return fmt.Errorf("Receive(): error opening file %s: %w", blockFile, err)
		}
		defer file.Close()

		// rpcs.EncodedBlockCert, but with pointers
		type encodedBlockCert struct {
			_struct struct{} `codec:""`

			Block       *bookkeeping.Block     `codec:"block"`
			Certificate *agreement.Certificate `codec:"cert"`
		}

		block := encodedBlockCert{
			Block: &bookkeeping.Block{
				BlockHeader: exportData.BlockHeader,
				Payset:      exportData.Payset,
			},
			Certificate: exportData.Certificate,
		}

		err = json.NewEncoder(file).Encode(block)
		if err != nil {
			return fmt.Errorf("Receive(): error encoding exportData: %w", err)
		}
		exp.logger.Infof("Wrote block %d to %s", exportData.Round(), blockFile)
	}

	// write state delta to file
	if !exp.cfg.ExcludeStateDelta {
		deltaFile := path.Join(exp.cfg.BlocksDir, fmt.Sprintf(FileExporterFileFormat, "delta", exportData.Round()))
		file, err := os.OpenFile(deltaFile, os.O_WRONLY|os.O_CREATE, 0755)
		if err != nil {
			return fmt.Errorf("Receive(): error opening file %s: %w", deltaFile, err)
		}
		defer file.Close()
		block := bookkeeping.Block{
			BlockHeader: exportData.BlockHeader,
			Payset:      exportData.Payset,
		}
		err = json.NewEncoder(file).Encode(block)
		if err != nil {
			return fmt.Errorf("Receive(): error encoding exportData: %w", err)
		}
		exp.logger.Infof("Wrote block %d to %s", exportData.Round(), deltaFile)
	}

	exp.round++
	exp.blockMetadata.NextRound = exp.round
	err := exp.encodeMetadataToFile()
	if err != nil {
		return fmt.Errorf("Receive() metadata encoding err %w", err)
	}
	return nil
}

func (exp *fileExporter) HandleGenesis(genesis bookkeeping.Genesis) error {
	// check genesis hash
	gh := crypto.HashObj(genesis).String()
	if exp.blockMetadata.GenesisHash == "" {
		exp.blockMetadata.GenesisHash = gh
		exp.blockMetadata.Network = string(genesis.Network)
		err := exp.encodeMetadataToFile()
		if err != nil {
			return fmt.Errorf("HandleGenesis() metadata encoding err %w", err)
		}
	} else {
		if exp.blockMetadata.GenesisHash != gh {
			return fmt.Errorf("HandleGenesis() genesis hash in metadata does not match expected value: actual %s, expected %s", gh, exp.blockMetadata.GenesisHash)
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
