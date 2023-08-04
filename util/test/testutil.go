package test

import (
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/algorand/go-codec/codec"
	"github.com/algorand/indexer/v3/idb"
	"github.com/algorand/indexer/v3/types"
	"github.com/algorand/indexer/v3/util"

	"github.com/algorand/go-algorand-sdk/v2/encoding/msgpack"
	sdk "github.com/algorand/go-algorand-sdk/v2/types"
)

var quiet = false
var exitValue = 0

// SetQuiet quiet mode of this logging thing.
func SetQuiet(q bool) {
	quiet = q
}

// ExitValue returns the captured exit value.
func ExitValue() int {
	return exitValue
}

func info(format string, a ...interface{}) {
	if quiet {
		return
	}
	fmt.Printf(format, a...)
}

// Info is the the only logging level for this thing.
var Info = info

func infoln(s string) {
	if quiet {
		return
	}
	fmt.Println(s)
}

func myStackTrace() {
	for skip := 1; skip < 3; skip++ {
		_, file, line, ok := runtime.Caller(skip)
		if !ok {
			return
		}
		fmt.Fprintf(os.Stderr, "%s:%d\n", file, line)
	}
}

// PrintAssetQuery prints information about an asset query.
func PrintAssetQuery(db idb.IndexerDb, q idb.AssetsQuery) {
	count := uint64(0)
	assetchan, _ := db.Assets(context.Background(), q)
	for ar := range assetchan {
		util.MaybeFail(ar.Error, "asset query %v\n", ar.Error)
		pjs, err := json.Marshal(ar.Params)
		util.MaybeFail(err, "json.Marshal params %v\n", err)
		var creator sdk.Address
		copy(creator[:], ar.Creator)
		info("%d %s %s\n", ar.AssetID, creator.String(), pjs)
		count++
	}
	info("%d rows\n", count)
	if q.Limit != 0 && q.Limit != count {
		fmt.Fprintf(os.Stderr, "asset q CAME UP SHORT, limit=%d actual=%d, q=%#v\n", q.Limit, count, q)
		myStackTrace()
		exitValue = 1
	}
}

// PrintAccountQuery prints information about an account query.
func PrintAccountQuery(db idb.IndexerDb, q idb.AccountQueryOptions) {
	accountchan, _ := db.GetAccounts(context.Background(), q)
	count := uint64(0)
	for ar := range accountchan {
		util.MaybeFail(ar.Error, "GetAccounts err %v\n", ar.Error)
		jb, err := json.Marshal(ar.Account)
		util.MaybeFail(err, "err %v\n", err)
		infoln(string(jb))
		//fmt.Printf("%#v\n", ar.Account)
		count++
	}
	info("%d accounts\n", count)
	if q.Limit != 0 && q.Limit != count {
		fmt.Fprintf(os.Stderr, "account q CAME UP SHORT, limit=%d actual=%d, q=%#v\n", q.Limit, count, q)
		myStackTrace()
		exitValue = 1
	}
}

// PrintTxnQuery prints information about a transaction query.
func PrintTxnQuery(db idb.IndexerDb, q idb.TransactionFilter) {
	rowchan, _ := db.Transactions(context.Background(), q)
	count := uint64(0)
	for txnrow := range rowchan {
		util.MaybeFail(txnrow.Error, "err %v\n", txnrow.Error)
		stxn := txnrow.Txn
		if stxn != nil {
			tjs := util.JSONOneLine(stxn.Txn)
			info("%d:%d %s sr=%d rr=%d ca=%d cr=%d t=%s\n", txnrow.Round, txnrow.Intra, tjs, stxn.SenderRewards, stxn.ReceiverRewards, stxn.ClosingAmount, stxn.CloseRewards, txnrow.RoundTime.String())
			count++
		}
	}
	info("%d txns\n", count)
	if q.Limit != 0 && count < 2 || count > 100 {
		fmt.Fprintf(os.Stderr, "txn q CAME UP SHORT, limit=%d actual=%d, q=%#v\n", q.Limit, count, q)
		myStackTrace()
		exitValue = 1
	}
}

// ReadValidatedBlockFromFile reads a validated block from file
func ReadValidatedBlockFromFile(filename string) (types.ValidatedBlock, error) {
	var vb types.ValidatedBlock
	dat, _ := os.ReadFile(filename)
	err := msgpack.Decode(dat, &vb)
	if err != nil {
		return vb, fmt.Errorf("ReadValidatedBlockFromFile err: %v", err)
	}

	return vb, nil

}

// CountingReader wraps an io.Reader to count the number of bytes read.
type CountingReader struct {
	r io.Reader
	n uint64
}

func (cr *CountingReader) Read(p []byte) (int, error) {
	n, err := cr.r.Read(p)
	cr.n += uint64(n)
	return n, err
}

// ReadConduitBlockFromFile reads a (validated) conduit block from a file
// and returns the block casted as a ValidatedBlock along with the number of bytes read (after decompression).
func ReadConduitBlockFromFile(filename string) (types.ValidatedBlock, uint64, error) {
	file, err := os.Open(filename)
	if err != nil {
		return types.ValidatedBlock{}, 0, fmt.Errorf("ReadConduitBlockFromFile err: %w", err)
	}
	defer file.Close()

	isGzip := filename[len(filename)-3:] == ".gz"
	var reader io.Reader
	if isGzip {
		gz, err := gzip.NewReader(file)
		if err != nil {
			return types.ValidatedBlock{}, 0, fmt.Errorf("ReadConduitBlockFromFile err: failed to make gzip reader: %w", err)
		}
		defer gz.Close()
		reader = gz
	} else {
		reader = file
	}

	cr := &CountingReader{r: reader}
	var cb types.BlockData
	if err := codec.NewDecoder(cr, msgpack.LenientCodecHandle).Decode(&cb); err != nil {
		return types.ValidatedBlock{}, 0, fmt.Errorf("ReadConduitBlockFromFile err: %w", err)
	}
	return types.ValidatedBlock{
		Block: sdk.Block{BlockHeader: cb.BlockHeader, Payset: cb.Payset},
		Delta: *cb.Delta,
	}, cr.n, nil
}


// AppAddress generates Address for the given appID
func AppAddress(app sdk.AppIndex) sdk.Address {
	hashid := "appID"
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(app))
	b := []byte(hashid)
	b = append(b, buf...)
	account := sha512.Sum512_256(b)

	return account
}
