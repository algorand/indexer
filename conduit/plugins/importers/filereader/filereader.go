package fileimporter

import (
	"context"
	_ "embed" // used to embed config
	"errors"
	"fmt"
	"io/fs"
	"path"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/algorand/indexer/conduit"
	"github.com/algorand/indexer/conduit/plugins"
	"github.com/algorand/indexer/conduit/plugins/exporters/filewriter"
	"github.com/algorand/indexer/conduit/plugins/importers"
	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/util"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
)

const importerName = "file_reader"

type fileReader struct {
	logger *logrus.Logger
	cfg    Config
	ctx    context.Context
	cancel context.CancelFunc
}

// New initializes an algod importer
func New() importers.Importer {
	return &fileReader{}
}

//go:embed sample.yaml
var sampleConfig string

var metadata = conduit.Metadata{
	Name:         importerName,
	Description:  "Importer for fetching blocks from files in a directory created by the 'file_writer' plugin.",
	Deprecated:   false,
	SampleConfig: sampleConfig,
}

func (r *fileReader) Metadata() conduit.Metadata {
	return metadata
}

// package-wide init function
func init() {
	importers.Register(importerName, importers.ImporterConstructorFunc(func() importers.Importer {
		return &fileReader{}
	}))
}

func (r *fileReader) Init(ctx context.Context, cfg plugins.PluginConfig, logger *logrus.Logger) (*sdk.Genesis, error) {
	r.ctx, r.cancel = context.WithCancel(ctx)
	r.logger = logger
	var err error
	err = cfg.UnmarshalConfig(&r.cfg)
	if err != nil {
		return nil, fmt.Errorf("invalid configuration: %v", err)
	}

	if r.cfg.FilenamePattern == "" {
		r.cfg.FilenamePattern = filewriter.FilePattern
	}

	genesisFile := path.Join(r.cfg.BlocksDir, "genesis.json")
	var genesis sdk.Genesis
	err = util.DecodeFromFile(genesisFile, &genesis, false)
	if err != nil {
		return nil, fmt.Errorf("Init(): failed to process genesis file: %w", err)
	}

	return &genesis, err
}

func (r *fileReader) Config() string {
	s, _ := yaml.Marshal(r.cfg)
	return string(s)
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
		start := time.Now()
		err := util.DecodeFromFile(filename, &blockData, false)
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
			r.logger.Infof("Block %d read time: %s", rnd, time.Since(start))
			// The read was fine, return the data.
			return blockData, nil
		}
	}
}
