package test

import (
	"github.com/algorand/go-algorand-sdk/client/v2/algod"
	"github.com/algorand/go-algorand-sdk/client/v2/common/models"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/protocol"
	"github.com/algorand/go-algorand/rpcs"
	"net/http"
	"net/http/httptest"
	"path"
	"strconv"
	"strings"
)

type algodHandler struct {
	responders []func(path string, w http.ResponseWriter) bool
}

func NewAlgodServer(responders ...func(path string, w http.ResponseWriter) bool) *httptest.Server {
	return httptest.NewServer(&algodHandler{responders})
}

func NewAlgodHandler(responders ...func(path string, w http.ResponseWriter) bool) *algodHandler {
	return &algodHandler{responders}
}

func (handler *algodHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	for _, responder := range handler.responders {
		if responder(req.URL.Path, w) {
			return
		}
	}
	w.WriteHeader(http.StatusBadRequest)
}

func MockAClient(algodHandler *algodHandler) (*algod.Client, error) {
	mockServer := httptest.NewServer(algodHandler)
	return algod.MakeClient(mockServer.URL, "")
}

func BlockResponder(reqPath string, w http.ResponseWriter) bool {
	if strings.Contains(reqPath, "v2/blocks/") {
		rnd, _ := strconv.Atoi(path.Base(reqPath))
		blk := rpcs.EncodedBlockCert{Block: bookkeeping.Block{BlockHeader: bookkeeping.BlockHeader{Round: basics.Round(rnd)}}}
		blockbytes := protocol.Encode(&blk)
		w.WriteHeader(http.StatusOK)
		w.Write(blockbytes)
		return true
	}
	return false
}

func GenesisResponder(reqPath string, w http.ResponseWriter) bool {
	if strings.Contains(reqPath, "/genesis") {
		w.WriteHeader(http.StatusOK)
		genesis := &bookkeeping.Genesis{}
		blockbytes := protocol.EncodeJSON(*genesis)
		w.Write(blockbytes)
		return true
	}
	return false
}

func BlockAfterResponder(reqPath string, w http.ResponseWriter) bool {
	if strings.Contains(reqPath, "/wait-for-block-after") {
		w.WriteHeader(http.StatusOK)
		nStatus := models.NodeStatus{}
		w.Write(protocol.EncodeJSON(nStatus))
		return true
	}
	return false
}
