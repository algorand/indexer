package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/rpcs"

	"github.com/algorand/indexer/processor/blockprocessor"
	itest "github.com/algorand/indexer/util/test"
)

type mockImporter struct {
}

var errMockImportBlock = errors.New("Process() invalid round blockCert.Block.Round(): 1234 nextRoundToProcess: 1")

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
	l, err := itest.MakeTestLedger(nullLogger, "ledger")
	require.NoError(t, err)
	defer l.Close()
	proc, err := blockprocessor.MakeProcessorWithLedger(l, nil)
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
