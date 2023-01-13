package fetcher

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/protocol"
	"github.com/algorand/go-algorand/rpcs"

	"github.com/algorand/indexer/util/test"
)

type BlockHandler struct {
	mock.Mock
}

func (handler *BlockHandler) handlerFunc(ctx context.Context, cert *rpcs.EncodedBlockCert) error {
	args := handler.Called(ctx, cert)
	return args.Error(0)
}

func TestFetcherImplErrorInitialization(t *testing.T) {
	aclient, err := test.MockAClient(test.NewAlgodHandler())
	assert.NoError(t, err)
	fetcher := &fetcherImpl{aclient: aclient, log: logrus.New()}
	require.Equal(t, "", fetcher.Error(), "Initialization of fetcher caused an unexpected error.")
}

func TestFetcherImplAlgodReturnsClient(t *testing.T) {
	aclient, err := test.MockAClient(test.NewAlgodHandler())
	assert.NoError(t, err)
	fetcher := &fetcherImpl{aclient: aclient, log: logrus.New()}
	require.Equal(t, aclient, fetcher.Algod(), "Algod client returned from fetcherImpl does not match expected instance.")
}

func TestFetcherImplSetError(t *testing.T) {
	aclient, err := test.MockAClient(test.NewAlgodHandler())
	assert.NoError(t, err)
	fetcher := &fetcherImpl{aclient: aclient, log: logrus.New()}
	expectedErr := fmt.Errorf("foobar")
	fetcher.setError(expectedErr)
	require.Equal(t, expectedErr.Error(), fetcher.Error(), "Error produced by setError was not reflected in Error output.")
}

func TestFetcherImplProcessQueueHandlerError(t *testing.T) {
	aclient, err := test.MockAClient(test.NewAlgodHandler(test.BlockResponder))
	assert.NoError(t, err)
	fetcher := &fetcherImpl{aclient: aclient, log: logrus.New()}
	bHandler := &BlockHandler{}
	expectedError := fmt.Errorf("handlerError")
	// The block handler function will immediately return an error on any block passed to it
	bHandler.On("handlerFunc", mock.Anything, mock.Anything).Return(expectedError)
	fetcher.SetBlockHandler(bHandler.handlerFunc)
	require.ErrorIsf(t, fetcher.Run(context.Background()), expectedError, "FetcherImpl did not return expected error in processQueue handler.")
}

func TestFetcherImplCatchupLoopBlockError(t *testing.T) {
	passingCalls := 5
	aclient, err := test.MockAClient(test.NewAlgodHandler(
		// Our mock algod client will process /v2/blocks/{round} calls
		// returning an empty block `passingCalls` times before throwing 500s
		func(path string, w http.ResponseWriter) bool {
			if strings.Contains(path, "v2/blocks/") {
				if passingCalls == 0 {
					w.WriteHeader(http.StatusInternalServerError)
				} else {
					var block bookkeeping.Block
					w.WriteHeader(http.StatusOK)
					w.Write(protocol.Encode(&block))
					passingCalls--
				}
				return true
			}
			return false
		}),
	)
	assert.NoError(t, err)
	// Initializing blockQueue here needs buffer since we have no other goroutines receiving from it
	fetcher := &fetcherImpl{aclient: aclient, log: logrus.New(), blockQueue: make(chan *rpcs.EncodedBlockCert, 256)}
	bHandler := &BlockHandler{}
	// the handler will do nothing here
	bHandler.On("handlerFunc", mock.Anything, mock.Anything).Return(nil)
	fetcher.SetBlockHandler(bHandler.handlerFunc)
	err = fetcher.catchupLoop(context.Background())
	require.NoError(t, err, "FetcherImpl returned an unexpected error from catchupLoop")
	require.Equal(t, "", fetcher.Error(), "FetcherImpl set an unexpected error from algod client during catchupLoop")
}

func TestAlgodArgsForDataDirNetDoesNotExist(t *testing.T) {
	_, _, _, err := AlgodArgsForDataDir("foobar")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "foobar/algod.net: ")
}

func TestAlgodArgsForDataDirTokenDoesNotExist(t *testing.T) {
	dir, err := os.MkdirTemp("", "datadir")
	if err != nil {
		t.Fatalf(err.Error())
	}
	err = os.WriteFile(filepath.Join(dir, "algod.net"), []byte("127.0.0.1:8080"), fs.ModePerm)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer os.RemoveAll(dir)
	_, _, _, err = AlgodArgsForDataDir(dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), fmt.Sprintf("%v/algod.token: ", dir))
}

func TestAlgodArgsForDataDirSuccess(t *testing.T) {
	dir, err := os.MkdirTemp("", "datadir")
	if err != nil {
		t.Fatalf(err.Error())
	}
	err = os.WriteFile(filepath.Join(dir, "algod.net"), []byte("127.0.0.1:8080"), fs.ModePerm)
	if err != nil {
		t.Fatalf(err.Error())
	}
	err = os.WriteFile(filepath.Join(dir, "algod.token"), []byte("abc123"), fs.ModePerm)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer os.RemoveAll(dir)
	netAddr, token, lastmod, err := AlgodArgsForDataDir(dir)
	assert.NoError(t, err)
	assert.Equal(t, netAddr, "http://127.0.0.1:8080")
	assert.Equal(t, token, "abc123")
	assert.NotNil(t, lastmod)

}
