package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	atypes "github.com/algorand/go-algorand-sdk/types"

	"github.com/algorand/indexer/idb"
	_ "github.com/algorand/indexer/idb/postgres"
	"github.com/algorand/indexer/types"
	testutil "github.com/algorand/indexer/util/test"
)

var (
	quiet = false
	pgdb  = "dbname=i2b sslmode=disable"
	truev = true
)

var maybeFail = testutil.MaybeFail
var printAccountQuery = testutil.PrintAccountQuery
var printTxnQuery = testutil.PrintTxnQuery

func b64(x string) []byte {
	v, err := base64.StdEncoding.DecodeString(x)
	if err != nil {
		panic(err)
	}
	return v
}

func main() {
	start := time.Now()
	flag.BoolVar(&quiet, "q", false, "")
	flag.StringVar(&pgdb, "pg", "dbname=i2b sslmode=disable", "postgres connect string; e.g. \"dbname=foo sslmode=disable\"")
	flag.Parse()
	testutil.SetQuiet(quiet)

	db, err := idb.IndexerDbByName("postgres", pgdb, &idb.IndexerDbOptions{ReadOnly: true})
	maybeFail(err, "open postgres, %v", err)

	rekeyTxnQuery := idb.TransactionFilter{RekeyTo: &truev, Limit: 1}
	printTxnQuery(db, rekeyTxnQuery)

	rowchan := db.Transactions(context.Background(), rekeyTxnQuery)
	var rekeyTo atypes.Address
	for txnrow := range rowchan {
		maybeFail(txnrow.Error, "err rekey txn %v\n", txnrow.Error)
		var stxn types.SignedTxnWithAD
		err := msgpack.Decode(txnrow.TxnBytes, &stxn)
		maybeFail(err, "decode txnbytes %v\n", err)
		rekeyTo = stxn.Txn.RekeyTo
	}

	printAccountQuery(db, idb.AccountQueryOptions{EqualToAuthAddr: rekeyTo[:], Limit: 1})

	dt := time.Now().Sub(start)
	exitValue := testutil.ExitValue()
	if exitValue == 0 {
		fmt.Printf("wat OK %s\n", dt.String())
	} else {
		fmt.Printf("wat ERROR %s\n", dt.String())
	}
	os.Exit(exitValue)
}
