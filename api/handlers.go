// Copyright (C) 2019-2020 Algorand, Inc.
// This file is part of the Algorand Indexer
//
// Algorand Indexer is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// Algorand Indexer is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with Algorand Indexer.  If not, see <https://www.gnu.org/licenses/>.

package api

import (
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/algorand/go-algorand-sdk/client/algod/models"
	"github.com/algorand/go-algorand-sdk/crypto"
	algojson "github.com/algorand/go-algorand-sdk/encoding/json"
	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	atypes "github.com/algorand/go-algorand-sdk/types"
	"github.com/gorilla/mux"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/importer"
	"github.com/algorand/indexer/types"
)

func b64decode(x string) (out []byte, err error) {
	if len(x) == 0 {
		return nil, nil
	}
	out, err = base64.URLEncoding.WithPadding(base64.StdPadding).DecodeString(x)
	if err == nil {
		return
	}
	out, err = base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(x)
	return
}

// parseTime same as go-algorand/daemon/algod/api/server/v1/handlers/handlers.go
func parseTime(t string) (res time.Time, err error) {
	// check for just date
	res, err = time.Parse("2006-01-02", t)
	if err == nil {
		return
	}

	// check for date and time
	res, err = time.Parse(time.RFC3339, t)
	if err == nil {
		return
	}

	return
}

func formUint64(r *http.Request, keySynonyms []string, defaultValue uint64) (value uint64, err error) {
	value = defaultValue
	for _, key := range keySynonyms {
		svalues, any := r.Form[key]
		if !any || len(svalues) < 1 {
			continue
		}
		// last value wins. or should we make repetition a 400 err?
		svalue := svalues[len(svalues)-1]
		if len(svalue) == 0 {
			continue
		}
		value, err = strconv.ParseUint(svalue, 10, 64)
		return
	}
	return
}

func formInt64(r *http.Request, keySynonyms []string, defaultValue int64) (value int64, err error) {
	value = defaultValue
	for _, key := range keySynonyms {
		svalues, any := r.Form[key]
		if !any || len(svalues) < 1 {
			continue
		}
		// last value wins. or should we make repetition a 400 err?
		svalue := svalues[len(svalues)-1]
		if len(svalue) == 0 {
			continue
		}
		value, err = strconv.ParseInt(svalue, 10, 64)
		return
	}
	return
}

func formString(r *http.Request, keySynonyms []string, defaultValue string) (value string) {
	value = defaultValue
	for _, key := range keySynonyms {
		svalues, any := r.Form[key]
		if !any || len(svalues) < 1 {
			continue
		}
		// last value wins. or should we make repetition a 400 err?
		value = svalues[len(svalues)-1]
		return
	}
	return
}

/*
func formUint64(r *http.Request, key string, defaultValue uint64) (value uint64, err error) {
	return formUint64Synonyms(r, []string{key}, defaultValue)
}
*/

func formTime(r *http.Request, keySynonyms []string) (value time.Time, err error) {
	for _, key := range keySynonyms {
		svalues, any := r.Form[key]
		if !any || len(svalues) < 1 {
			continue
		}
		// last value wins. or should we make repetition a 400 err?
		svalue := svalues[len(svalues)-1]
		if len(svalue) == 0 {
			continue
		}
		value, err = parseTime(svalue)
		return
	}
	return
}

type listAccountsReply struct {
	Accounts []models.Account `json:"accounts,omitempty"`
}

// ListAccounts is the http api handler that lists accounts and basic data
// /v1/accounts
// ?gt={addr} // return assets greater than some addr, for paging
// ?assets=1 // return AssetHolding for assets owned by this account
// ?assetParams=1 // return AssetParams for assets created by this account
// ?limit=N
// return {"accounts":[]models.Account}
func ListAccounts(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	var gtAddr types.Address
	accounts, err := IndexerDb.GetAccounts(r.Context(), gtAddr, 10000)
	if err != nil {
		log.Println("ListAccounts ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	out := listAccountsReply{Accounts: accounts}
	enc := json.NewEncoder(w)

	err = enc.Encode(out)
}

type stringInt struct {
	str string
	i   int
}

const (
	jsonInternalFormat = 1
	algodApiFormat     = 2
	msgpackFormat      = 3
)

// collapse format=str to int for switch
var transactionFormatMapList = []stringInt{
	{"json", jsonInternalFormat},
	{"algod", algodApiFormat},
	{"msgp", msgpackFormat},
	{"1", jsonInternalFormat},
	{"2", algodApiFormat},
	{"3", msgpackFormat},
	{"", jsonInternalFormat}, // default
}
var transactionFormatMap map[string]int

var sigTypeEnumMapList = []stringInt{
	{"sig", 1},
	{"msig", 2},
	{"lsig", 3},
}

var sigTypeEnumMap map[string]int

func enumListToMap(list []stringInt) map[string]int {
	out := make(map[string]int, len(list))
	for _, tf := range list {
		out[tf.str] = tf.i
	}
	return out
}

func init() {
	transactionFormatMap = enumListToMap(transactionFormatMapList)
	sigTypeEnumMap = enumListToMap(sigTypeEnumMapList)
}

type SignedTxnWrapper struct {
	Round  uint64                 `codec:"r,omitempty"`
	Offset int                    `codec:"o,omitempty"`
	Stxn   types.SignedTxnInBlock `codec:"stxn,omitempty"`
}

type transactionsListReturnObject struct {
	Transactions []models.Transaction
	Stxns        []SignedTxnWrapper
	// TODO: msgpack chunks
}

func (out *transactionsListReturnObject) storeTransactionOutput(stxn *types.SignedTxnInBlock, round uint64, intra int, format int) {
	switch format {
	case algodApiFormat:
		if out.Transactions == nil {
			out.Transactions = make([]models.Transaction, 0, 10)
		}
		var mtxn models.Transaction
		setApiTxn(&mtxn, stxn)
		mtxn.ConfirmedRound = round
		out.Transactions = append(out.Transactions, mtxn)
	case jsonInternalFormat, msgpackFormat:
		if out.Stxns == nil {
			out.Stxns = make([]SignedTxnWrapper, 0, 10)
		}
		out.Stxns = append(out.Stxns, SignedTxnWrapper{
			Round:  round,
			Offset: intra,
			Stxn:   *stxn,
		})
	}
}

type algodTransactionsListReturnObject struct {
	Transactions []models.Transaction `json:"transactions,omitempty"`
}

type rawTransactionsListReturnObject struct {
	Transactions []SignedTxnWrapper `codec:"txns,omitemptyarray"`
}

func (out *transactionsListReturnObject) writeTxnReturn(w http.ResponseWriter, format int) (err error) {
	switch format {
	case algodApiFormat:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		retob := algodTransactionsListReturnObject{Transactions: out.Transactions}
		enc := json.NewEncoder(w)
		return enc.Encode(retob)
	case jsonInternalFormat:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		retob := rawTransactionsListReturnObject{Transactions: out.Stxns}
		bytes := algojson.Encode(retob)
		_, err = w.Write(bytes)
		return
	case msgpackFormat:
		w.Header().Set("Content-Type", "application/msgpack")
		w.WriteHeader(http.StatusOK)
		retob := rawTransactionsListReturnObject{Transactions: out.Stxns}
		bytes := msgpack.Encode(retob)
		_, err = w.Write(bytes)
		return
	default:
		panic("unknown format")
	}
}

const txnQueryLimit = 1000

func requestFilter(r *http.Request) (tf idb.TransactionFilter, err error) {
	tf.Init()
	tf.MinRound, err = formUint64(r, []string{"minround", "rl"}, 0)
	if err != nil {
		err = fmt.Errorf("bad minround, %v", err)
		return
	}
	tf.MaxRound, err = formUint64(r, []string{"maxround", "rh"}, 0)
	if err != nil {
		err = fmt.Errorf("bad maxround, %v", err)
		return
	}
	tf.BeforeTime, err = formTime(r, []string{"beforeTime", "bt", "toDate"})
	if err != nil {
		err = fmt.Errorf("bad beforeTime, %v", err)
		return
	}
	tf.AfterTime, err = formTime(r, []string{"afterTime", "at", "fromDate"})
	if err != nil {
		err = fmt.Errorf("bad aftertime, %v", err)
		return
	}
	tf.AssetId, err = formUint64(r, []string{"asset"}, 0)
	if err != nil {
		err = fmt.Errorf("bad asset, %v", err)
		return
	}
	typeStr := formString(r, []string{"type"}, "")
	// explicitly using the map lookup miss returning the zero value:
	tf.TypeEnum = importer.TypeEnumMap[typeStr]
	txidStr := formString(r, []string{"txid"}, "")
	if len(txidStr) > 0 {
		tf.Txid, err = base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(txidStr)
		if err != nil {
			err = fmt.Errorf("bad txid, %v", err)
			return
		}
	}
	tf.Round, err = formInt64(r, []string{"round", "r"}, -1)
	if err != nil {
		err = fmt.Errorf("bad round, %v", err)
		return
	}
	tf.Offset, err = formInt64(r, []string{"offset", "o"}, -1)
	if err != nil {
		err = fmt.Errorf("bad offset, %v", err)
		return
	}
	tf.SigType = formString(r, []string{"sig"}, "")
	tf.NotePrefix, err = b64decode(formString(r, []string{"noteprefix"}, ""))
	if err != nil {
		err = fmt.Errorf("bad noteprefix, %v", err)
		return
	}
	tf.MinAlgos, err = formUint64(r, []string{"minalgos"}, 0)
	if err != nil {
		err = fmt.Errorf("bad minalgos, %v", err)
		return
	}
	tf.Limit, err = formUint64(r, []string{"limit", "l"}, txnQueryLimit)
	if err != nil {
		err = fmt.Errorf("bad limit, %v", err)
		return
	}
	return
}

// TransactionsForAddress returns transactions for some account.
// most-recent first, into the past.
// ?limit=N  default 100? 10? 50? 20 kB?
// ?minround=N
// ?maxround=N
// ?afterTime=timestamp string
// ?beforeTime=timestamp string
// ?format=json(default)/algod/msgp aka 1/2/3
// TODO: ?type=pay/keyreg/acfg/axfr/afrz
// Algod had ?fromDate ?toDate
// Where “timestamp string” is either YYYY-MM-DD or RFC3339 = "2006-01-02T15:04:05Z07:00"
//
// return {"transactions":[]models.Transaction}
// /v1/account/{address}/transactions TransactionsForAddress
func TransactionsForAddress(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	queryAddr := mux.Vars(r)["address"]
	addr, err := atypes.DecodeAddress(queryAddr)
	if err != nil {
		log.Println("bad addr, ", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	tf, err := requestFilter(r)
	if err != nil {
		log.Println("bad tf, ", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	tf.Address = addr[:]
	formatStr := formString(r, []string{"format"}, "json")
	format, ok := transactionFormatMap[formatStr]
	if !ok {
		log.Println("bad format: ", formatStr)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	txns := IndexerDb.Transactions(r.Context(), tf)

	result := transactionsListReturnObject{}
	for txnRow := range txns {
		var stxn types.SignedTxnInBlock
		if txnRow.Error != nil {
			log.Println("error fetching txns, ", txnRow.Error)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		err = msgpack.Decode(txnRow.TxnBytes, &stxn)
		if err != nil {
			log.Println("error decoding txnbytes, ", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		result.storeTransactionOutput(&stxn, txnRow.Round, txnRow.Intra, format)
	}
	err = result.writeTxnReturn(w, format)
	if err != nil {
		log.Println("transactions json out, ", err)
	}
}

func addrJson(addr atypes.Address) string {
	if addr.IsZero() {
		return ""
	}
	return addr.String()
}

func setApiTxn(out *models.Transaction, stxn *types.SignedTxnInBlock) {
	out.Type = stxn.Txn.Type
	out.TxID = crypto.TransactionIDString(stxn.Txn)
	out.From = addrJson(stxn.Txn.Sender)
	out.Fee = uint64(stxn.Txn.Fee)
	out.FirstRound = uint64(stxn.Txn.FirstValid)
	out.LastRound = uint64(stxn.Txn.LastValid)
	out.Note = models.Bytes(stxn.Txn.Note)
	// out.ConfirmedRound // TODO
	// TODO: out.TransactionResults = &TransactionResults{CreatedAssetIndex: 0}
	// TODO: add Group field
	// TODO: add other txn types!
	switch stxn.Txn.Type {
	case atypes.PaymentTx:
		out.Payment = &models.PaymentTransactionType{
			To:               addrJson(stxn.Txn.Receiver),
			CloseRemainderTo: addrJson(stxn.Txn.CloseRemainderTo),
			CloseAmount:      uint64(stxn.ClosingAmount),
			Amount:           uint64(stxn.Txn.Amount),
			ToRewards:        uint64(stxn.ReceiverRewards),
		}
	case atypes.KeyRegistrationTx:
		log.Println("WARNING TODO implement keyreg")
	case atypes.AssetConfigTx:
		log.Println("WARNING TODO implement acfg")
	case atypes.AssetTransferTx:
		log.Println("WARNING TODO implement axfer")
	case atypes.AssetFreezeTx:
		log.Println("WARNING TODO implement afrz")
	}
	out.FromRewards = uint64(stxn.SenderRewards)
	out.GenesisID = stxn.Txn.GenesisID
	out.GenesisHash = stxn.Txn.GenesisHash[:]

}
