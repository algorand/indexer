package test

import (
	"net/http"
	"net/http/httptest"
	"path"
	"strconv"
	"strings"

	"github.com/algorand/go-algorand-sdk/client/v2/algod"
	"github.com/algorand/go-algorand-sdk/client/v2/common/models"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/protocol"
	"github.com/algorand/go-algorand/rpcs"
)

// AlgodHandler is used to handle http requests to a mock algod server
type AlgodHandler struct {
	responders []func(path string, w http.ResponseWriter) bool
}

// NewAlgodServer creates an httptest server with an algodHandler using the provided responders
func NewAlgodServer(responders ...func(path string, w http.ResponseWriter) bool) *httptest.Server {
	return httptest.NewServer(&AlgodHandler{responders})
}

// NewAlgodHandler creates an AlgodHandler using the provided responders
func NewAlgodHandler(responders ...func(path string, w http.ResponseWriter) bool) *AlgodHandler {
	return &AlgodHandler{responders}
}

// ServeHTTP implements the http.Handler interface for AlgodHandler
func (handler *AlgodHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	for _, responder := range handler.responders {
		if responder(req.URL.Path, w) {
			return
		}
	}
	w.WriteHeader(http.StatusBadRequest)
}

// MockAClient creates an algod client using an AlgodHandler based server
func MockAClient(handler *AlgodHandler) (*algod.Client, error) {
	mockServer := httptest.NewServer(handler)
	return algod.MakeClient(mockServer.URL, "")
}

// BlockResponder handles /v2/blocks requests and returns an empty Block object
func BlockResponder(reqPath string, w http.ResponseWriter) bool {
	if strings.Contains(reqPath, "v2/blocks/") {
		rnd, _ := strconv.Atoi(path.Base(reqPath))
		blk := rpcs.EncodedBlockCert{Block: bookkeeping.Block{BlockHeader: bookkeeping.BlockHeader{Round: basics.Round(rnd)}}}
		blockbytes := protocol.Encode(&blk)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(blockbytes)
		return true
	}
	return false
}

// GenesisResponder handles /v2/genesis requests and returns an empty Genesis object
func GenesisResponder(reqPath string, w http.ResponseWriter) bool {
	if strings.Contains(reqPath, "/genesis") {
		w.WriteHeader(http.StatusOK)
		genesis := &bookkeeping.Genesis{}
		blockbytes := protocol.EncodeJSON(*genesis)
		_, _ = w.Write(blockbytes)
		return true
	}
	return false
}

// BlockAfterResponder handles /v2/wait-for-block-after requests and returns an empty NodeStatus object
func BlockAfterResponder(reqPath string, w http.ResponseWriter) bool {
	if strings.Contains(reqPath, "/wait-for-block-after") {
		w.WriteHeader(http.StatusOK)
		nStatus := models.NodeStatus{}
		_, _ = w.Write(protocol.EncodeJSON(nStatus))
		return true
	}
	return false
}
