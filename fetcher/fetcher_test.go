package fetcher

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/algorand/go-algorand-sdk/client/v2/algod"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/protocol"
	"github.com/algorand/go-algorand/rpcs"
)

type AlgodHandler struct {
	mock.Mock
}

func (handler *AlgodHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	handler.Called(w, req)
	return
}

type BlockHandler struct {
	mock.Mock
}

func (handler *BlockHandler) handlerFunc(ctx context.Context, cert *rpcs.EncodedBlockCert) error {
	args := handler.Called(ctx, cert)
	return args.Error(0)
}

func mockAClient(t *testing.T, algodHandler *AlgodHandler) *algod.Client {
	mockServer := httptest.NewServer(algodHandler)
	aclient, err := algod.MakeClient(mockServer.URL, "")
	if err != nil {
		t.FailNow()
	}
	return aclient
}

func TestFetcherImplErrorInitialization(t *testing.T) {
	aclient := mockAClient(t, &AlgodHandler{})
	fetcher := &fetcherImpl{aclient: aclient, log: logrus.New()}
	require.Equal(t, "", fetcher.Error(), "Initialization of fetcher caused an unexpected error.")
}

func TestFetcherImplAlgodReturnsClient(t *testing.T) {
	aclient := mockAClient(t, &AlgodHandler{})
	fetcher := &fetcherImpl{aclient: aclient, log: logrus.New()}
	require.Equal(t, aclient, fetcher.Algod(), "Algod client returned from fetcherImpl does not match expected instance.")
}

func TestFetcherImplSetError(t *testing.T) {
	aclient := mockAClient(t, &AlgodHandler{})
	fetcher := &fetcherImpl{aclient: aclient, log: logrus.New()}
	expectedErr := fmt.Errorf("foobar")
	fetcher.setError(expectedErr)
	require.Equal(t, expectedErr.Error(), fetcher.Error(), "Error produced by setError was not reflected in Error output.")
}

func TestFetcherImplProcessQueueHandlerError(t *testing.T) {
	mockAlgodHandler := &AlgodHandler{}
	aclient := mockAClient(t, mockAlgodHandler)
	fetcher := &fetcherImpl{aclient: aclient, log: logrus.New()}
	bHandler := &BlockHandler{}
	expectedError := fmt.Errorf("handlerError")
	// The block handler function will immediately return an error on any block passed to it
	bHandler.On("handlerFunc", mock.Anything, mock.Anything).Return(expectedError)
	fetcher.SetBlockHandler(bHandler.handlerFunc)
	// Mock algod server to continually return empty blocks
	mockAlgodHandler.On("ServeHTTP", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		respWriter := args.Get(0).(http.ResponseWriter)
		req := args.Get(1).(*http.Request)
		path := req.URL.Path
		if strings.Contains(path, "v2/blocks/") {
			var block bookkeeping.Block
			respWriter.Write(protocol.Encode(&block))
		}
	})
	require.ErrorIsf(t, fetcher.Run(context.Background()), expectedError, "FetcherImpl did not return expected error in processQueue handler.")
}

func TestFetcherImplCatchupLoopBlockError(t *testing.T) {
	mockAlgodHandler := &AlgodHandler{}
	aclient := mockAClient(t, mockAlgodHandler)
	passingCalls := 5
	// Initializing blockQueue here needs buffer since we have no other goroutines receiving from it
	fetcher := &fetcherImpl{aclient: aclient, log: logrus.New(), blockQueue: make(chan *rpcs.EncodedBlockCert, 256)}
	bHandler := &BlockHandler{}
	// the handler will do nothing here
	bHandler.On("handlerFunc", mock.Anything, mock.Anything).Return(nil)
	fetcher.SetBlockHandler(bHandler.handlerFunc)

	// Our mock algod client will process /v2/blocks/{round} calls
	// returning an empty block `passingCalls` times before throwing 500s
	mockAlgodHandler.On("ServeHTTP", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		respWriter := args.Get(0).(http.ResponseWriter)
		req := args.Get(1).(*http.Request)
		path := req.URL.Path
		if strings.Contains(path, "v2/blocks/") {
			if passingCalls <= 0 {
				respWriter.WriteHeader(http.StatusInternalServerError)
			} else {
				var block bookkeeping.Block
				respWriter.WriteHeader(http.StatusOK)
				respWriter.Write(protocol.Encode(&block))
				passingCalls--
			}
		}
	})
	err := fetcher.catchupLoop(context.Background())
	require.NoError(t, err, "FetcherImpl returned an unexpected error from catchupLoop")
	require.Equal(t, "", fetcher.Error(), "FetcherImpl set an unexpected error from algod client during catchupLoop")
}

func TestDirectCatchupService(t *testing.T) {
	nextRound := uint64(0)
	ctx, _ := context.WithCancel(context.Background())

	// load genesis from disk
	genesisFile := "/Users/ganesh/go-algorand/installer/genesis/mainnet/genesis.json"
	genesisText, _ := ioutil.ReadFile(genesisFile)
	var genesis bookkeeping.Genesis
	_ = protocol.DecodeJSON(genesisText, &genesis)

	// make catchup service
	serviceDr := MakeCatchupService(ctx, genesis)
	serviceDr.net.Start()
	serviceDr.cfg.NetAddress, _ = serviceDr.net.Address()

	// making peerselector, makes sure that dns records are loaded
	serviceDr.net.RequestConnectOutgoing(false, ctx.Done())
	serviceDr.peerSelector = serviceDr.createPeerSelector(false)
	if _, err := serviceDr.peerSelector.getNextPeer(); err != nil {
		fmt.Println(err)
	}
	psp, _ := serviceDr.peerSelector.getNextPeer()
	for nextRound < 10 {
		if psp.Peer == nil {
			psp, _ = serviceDr.peerSelector.getNextPeer()
		} else {
			blk, cert, err1 := serviceDr.DirectNetworkFetch(ctx, nextRound, psp, psp.Peer)
			if err1 != nil {
				psp, _ = serviceDr.peerSelector.getNextPeer()
			} else if uint64(blk.Round()) == nextRound {
				block := new(rpcs.EncodedBlockCert)
				block.Block = *blk
				block.Certificate = *cert
				nextRound++
			}
		}
	}
}
