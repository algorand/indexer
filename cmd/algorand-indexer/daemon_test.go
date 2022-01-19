package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/rpcs"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
)

type mockImporter struct {
}

var errMockImportBlock = errors.New("mock import block error")

func (imp *mockImporter) ImportBlock(blockContainer *rpcs.EncodedBlockCert) error {
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
	handler := blockHandler(imp, 50*time.Millisecond)
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
		assert.Equal(t, entry.Message, "adding block 1234 to database failed")
		assert.Equal(t, entry.Data["error"], errMockImportBlock)
	}

	// Wait for handler to exit.
	cancel()
	wg.Wait()
}
