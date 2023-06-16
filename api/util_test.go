package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/require"

	"github.com/algorand/go-algorand-sdk/v2/types"

	"github.com/algorand/indexer/v3/idb"
)

func TestCallWithTimeoutTimesOut(t *testing.T) {
	done := make(chan struct{})
	defer func() {
		close(done)
	}()

	logger, hook := test.NewNullLogger()
	err := callWithTimeout(context.Background(), logger, 1*time.Nanosecond, func(ctx context.Context) error {
		<-done
		return errors.New("should not return")
	})

	require.Error(t, err)
	require.ErrorIs(t, err, errTimeout)

	time.Sleep(2 * time.Second)
	require.Len(t, hook.Entries, 1)
	require.Equal(t, errMisbehavingHandler, hook.LastEntry().Message)
}

func TestCallWithTimeoutExitsWhenHandlerFinishes(t *testing.T) {
	done := make(chan struct{})
	defer func() {
		<-done
	}()

	callError := errors.New("this should be the result")
	err := callWithTimeout(context.Background(), nil, 1*time.Minute, func(ctx context.Context) error {
		defer close(done)
		return callError
	})

	require.Error(t, err)
	require.ErrorIs(t, err, callError)
}

func TestInvalidTxnRow(t *testing.T) {
	stxn := types.SignedTxnWithAD{}
	invalidRow := idb.TxnRow{Txn: &stxn, RootTxn: &stxn}
	_, err := txnRowToTransaction(invalidRow)
	require.Error(t, err)
	require.ErrorContains(t, err, "Txn and RootTxn should be mutually exclusive")
}
