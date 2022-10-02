package fileimporter

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/exporters/filewriter"
	"github.com/algorand/indexer/importers"
	"github.com/algorand/indexer/plugins"
	"github.com/algorand/indexer/util"
)

const importerName = "filereader"

type fileReader struct {
	logger *logrus.Logger
	cfg    Config
	ctx    context.Context
	cancel context.CancelFunc
}

var metadata = importers.ImporterMetadata{
	ImpName:        importerName,
	ImpDescription: "Importer for fetching blocks from files in a directory created by the 'file_writer' plugin.",
	ImpDeprecated:  false,
}

// New initializes an algod importer
func New() importers.Importer {
	return &fileReader{}
}

// Constructor is the Constructor implementation for the "algod" importer
type Constructor struct{}

// New initializes a blockProcessorConstructor
func (c *Constructor) New() importers.Importer {
	return &fileReader{}
}

func (r *fileReader) Metadata() importers.ImporterMetadata {
	return metadata
}

// package-wide init function
func init() {
	importers.RegisterImporter(importerName, &Constructor{})
}

func (r *fileReader) Init(ctx context.Context, cfg plugins.PluginConfig, logger *logrus.Logger) (*bookkeeping.Genesis, error) {
	r.ctx, r.cancel = context.WithCancel(ctx)
	r.logger = logger
	var err error
	r.cfg, err = unmarshalConfig(string(cfg))
	if err != nil {
		return nil, fmt.Errorf("invalid configuration: %v", err)
	}

	if r.cfg.FilenamePattern == "" {
		r.cfg.FilenamePattern = filewriter.FilePattern
	}

	genesisFile := path.Join(r.cfg.BlocksDir, "genesis.json")
	var genesis bookkeeping.Genesis
	err = util.DecodeFromFile(genesisFile, &genesis)
	if err != nil {
		return nil, fmt.Errorf("Init(): failed to process genesis file: %w", err)
	}

	return &genesis, err
}

func (r *fileReader) Config() plugins.PluginConfig {
	s, _ := yaml.Marshal(r.cfg)
	return plugins.PluginConfig(s)
}

func (r *fileReader) Close() error {
	if r.cancel != nil {
		r.cancel()
	}
	return nil
}

func (r *fileReader) GetBlock(rnd uint64) (data.BlockData, error) {
	attempts := r.cfg.RetryCount
	for {
		filename := path.Join(r.cfg.BlocksDir, fmt.Sprintf(r.cfg.FilenamePattern, rnd))
		var blockData data.BlockData
		err := util.DecodeFromFile(filename, &blockData)
		if err != nil && errors.Is(err, fs.ErrNotExist) {
			// If the file read failed because the file didn't exist, wait before trying again
			if attempts == 0 {
				return data.BlockData{}, fmt.Errorf("GetBlock(): block not found after (%d) attempts", r.cfg.RetryCount)
			}
			attempts--

			select {
			case <-time.After(r.cfg.RetryDuration):
			case <-r.ctx.Done():
				return data.BlockData{}, fmt.Errorf("GetBlock() context finished: %w", r.ctx.Err())
			}
		} else if err != nil {
			// Other error, return error to pipeline
			return data.BlockData{}, fmt.Errorf("GetBlock(): unable to read block file '%s': %w", filename, err)
		} else {
			// The read was fine, return the data.
			return blockData, nil
		}
	}
}

func unmarshalConfig(cfg string) (Config, error) {
	var config Config
	err := yaml.Unmarshal([]byte(cfg), &config)
	return config, err
}
