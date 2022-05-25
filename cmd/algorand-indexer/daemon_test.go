package main

import (
	"context"
	"errors"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/ledger"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/logging"
	"github.com/algorand/go-algorand/rpcs"
	"github.com/algorand/indexer/processor/blockprocessor"
	"github.com/algorand/indexer/util"
	test_util "github.com/algorand/indexer/util/test"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
)

type mockImporter struct {
}

var errMockImportBlock = errors.New("Process() invalid round blockCert.Block.Round(): 1234 proc.nextRoundToProcess: 1")

func (imp *mockImporter) ImportBlock(blockContainer *rpcs.EncodedBlockCert) error {
	return nil
}

func (imp *mockImporter) ImportValidatedBlock(vb *ledgercore.ValidatedBlock) error {
	return errMockImportBlock
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
	l := makeTestLedger(t, "local_ledger")
	defer l.Close()
	proc, err := blockprocessor.MakeProcessor(l, nil)
	assert.Nil(t, err)
	proc.SetHandler(imp.ImportValidatedBlock)
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

func makeTestLedger(t *testing.T, prefix string) *ledger.Ledger {
	// initialize local ledger
	genesis := test_util.MakeGenesis()
	genesisBlock := test_util.MakeGenesisBlock()
	initState, err := util.CreateInitState(&genesis, &genesisBlock)
	if err != nil {
		log.Panicf("test init err: %v", err)
	}
	l, err := ledger.OpenLedger(logging.NewLogger(), prefix, true, initState, config.GetDefaultLocal())
	if err != nil {
		log.Panicf("test init err: %v", err)
	}
	return l
}
