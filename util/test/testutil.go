package test

import (
	"context"
	"encoding/json"
	//"flag"
	"fmt"
	"os"
	//"time"

	//ajson "github.com/algorand/go-algorand-sdk/encoding/json"
	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	atypes "github.com/algorand/go-algorand-sdk/types"
	"github.com/algorand/go-codec/codec"

	//"github.com/algorand/indexer/accounting"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/types"
)

var quiet = false
var exitValue = 0

func SetQuiet(q bool) {
	quiet = q
}

func ExitValue() int {
	return exitValue
}

func info(format string, a ...interface{}) {
	if quiet {
		return
	}
	fmt.Printf(format, a...)
}

var Info = info

func infoln(s string) {
	if quiet {
		return
	}
	fmt.Println(s)
}

func PrintAssetQuery(db idb.IndexerDb, q idb.AssetsQuery) {
	count := uint64(0)
	for ar := range db.Assets(context.Background(), q) {
		MaybeFail(ar.Error, "asset query %v\n", ar.Error)
		pjs, err := json.Marshal(ar.Params)
		MaybeFail(err, "json.Marshal params %v\n", err)
		var creator atypes.Address
		copy(creator[:], ar.Creator)
		info("%d %s %s\n", ar.AssetId, creator.String(), pjs)
		count++
	}
	info("%d rows\n", count)
	if q.Limit != 0 && q.Limit != count {
		fmt.Fprintf(os.Stderr, "asset q CAME UP SHORT, limit=%d actual=%d, q=%#v\n", q.Limit, count, q)
		exitValue = 1
	}
}

func PrintAccountQuery(db idb.IndexerDb, q idb.AccountQueryOptions) {
	accountchan := db.GetAccounts(context.Background(), q)
	count := uint64(0)
	for ar := range accountchan {
		MaybeFail(ar.Error, "err %v\n", ar.Error)
		jb, err := json.Marshal(ar.Account)
		MaybeFail(err, "err %v\n", err)
		infoln(string(jb))
		//fmt.Printf("%#v\n", ar.Account)
		count++
	}
	info("%d accounts\n", count)
	if q.Limit != 0 && q.Limit != count {
		fmt.Fprintf(os.Stderr, "account q CAME UP SHORT, limit=%d actual=%d, q=%#v\n", q.Limit, count, q)
		exitValue = 1
	}
}

func PrintTxnQuery(db idb.IndexerDb, q idb.TransactionFilter) {
	rowchan := db.Transactions(context.Background(), q)
	count := uint64(0)
	for txnrow := range rowchan {
		MaybeFail(txnrow.Error, "err %v\n", txnrow.Error)
		var stxn types.SignedTxnWithAD
		err := msgpack.Decode(txnrow.TxnBytes, &stxn)
		MaybeFail(err, "decode txnbytes %v\n", err)
		//tjs, err := json.Marshal(stxn.Txn) // nope, ugly
		//MaybeFail(err, "err %v\n", err)
		tjs := string(JsonOneLine(stxn.Txn))
		info("%d:%d %s sr=%d rr=%d ca=%d cr=%d t=%s\n", txnrow.Round, txnrow.Intra, tjs, stxn.SenderRewards, stxn.ReceiverRewards, stxn.ClosingAmount, stxn.CloseRewards, txnrow.RoundTime.String())
		count++
	}
	info("%d txns\n", count)
	if q.Limit != 0 && q.Limit != count {
		fmt.Fprintf(os.Stderr, "txn q CAME UP SHORT, limit=%d actual=%d, q=%#v\n", q.Limit, count, q)
		exitValue = 1
	}
}

func MaybeFail(err error, errfmt string, params ...interface{}) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, errfmt, params...)
	os.Exit(1)
}

func JsonOneLine(obj interface{}) []byte {
	var b []byte
	enc := codec.NewEncoderBytes(&b, OneLineJsonCodecHandle)
	enc.MustEncode(obj)
	return b
}

var OneLineJsonCodecHandle *codec.JsonHandle

func init() {
	OneLineJsonCodecHandle = new(codec.JsonHandle)
	OneLineJsonCodecHandle.ErrorIfNoField = true
	OneLineJsonCodecHandle.ErrorIfNoArrayExpand = true
	OneLineJsonCodecHandle.Canonical = true
	OneLineJsonCodecHandle.RecursiveEmptyCheck = true
	OneLineJsonCodecHandle.HTMLCharsAsIs = true
	OneLineJsonCodecHandle.Indent = 0
}
