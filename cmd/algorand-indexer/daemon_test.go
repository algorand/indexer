package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/algorand/go-algorand-sdk/encoding/json"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/rpcs"
	"github.com/algorand/indexer/processor/blockprocessor"
	itest "github.com/algorand/indexer/util/test"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
)

type mockImporter struct {
}

var errMockImportBlock = errors.New("Process() invalid round blockCert.Block.Round(): 1234 proc.nextRoundToProcess: 1")

func (imp *mockImporter) ImportBlock(vb *ledgercore.ValidatedBlock) error {
	return nil
}

func TestImportRetryAndCancel(t *testing.T) {
	// connect debug logger
	nullLogger, hook := test.NewNullLogger()
	logger = nullLogger

	// cancellable context
	ctx := context.Background()
	cctx, cancel := context.WithCancel(ctx)

	// create handler with mock importer and start, it should generate errors until cancelled.
	imp := &mockImporter{}
	l := itest.MakeTestLedger("ledger")
	defer l.Close()
	proc, err := blockprocessor.MakeProcessor(l, nil)
	assert.Nil(t, err)
	proc.SetHandler(imp.ImportBlock)
	handler := blockHandler(proc, 50*time.Millisecond)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		block := rpcs.EncodedBlockCert{
			Block: bookkeeping.Block{
				BlockHeader: bookkeeping.BlockHeader{
					Round: 1234,
				},
			},
		}
		handler(cctx, &block)
	}()

	// accumulate some errors
	for len(hook.Entries) < 5 {
		time.Sleep(25 * time.Millisecond)
	}

	for _, entry := range hook.Entries {
		assert.Equal(t, entry.Message, "block 1234 import failed")
		assert.Equal(t, entry.Data["error"], errMockImportBlock)
	}

	// Wait for handler to exit.
	cancel()
	wg.Wait()
}

func TestReadGenesis(t *testing.T) {
	var reader io.Reader
	// nil reader
	_, err := readGenesis(reader)
	assert.Contains(t, err.Error(), "readGenesis() err: reader is nil")
	// no match struct field
	genesisStr := "{\"version\": 2}"
	reader = strings.NewReader(genesisStr)
	_, err = readGenesis(reader)
	assert.Contains(t, err.Error(), "json decode error")

	genesis := bookkeeping.Genesis{
		SchemaID:    "1",
		Network:     "test",
		Proto:       "test",
		RewardsPool: "AAAA",
		FeeSink:     "AAAA",
	}

	// read and decode genesis
	reader = strings.NewReader(string(json.Encode(genesis)))
	_, err = readGenesis(reader)
	assert.Nil(t, err)
	// read from empty reader
	_, err = readGenesis(reader)
	assert.Contains(t, err.Error(), "readGenesis() err: EOF")

}
