package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/protocol"

	"github.com/algorand/indexer/idb"
	_ "github.com/algorand/indexer/idb/postgres"
	"github.com/algorand/indexer/util"
	testutil "github.com/algorand/indexer/util/test"
)

var (
	quiet = false
	pgdb  = "dbname=i2b sslmode=disable"
	truev = true
)

var maybeFail = util.MaybeFail
var printAccountQuery = testutil.PrintAccountQuery
var printTxnQuery = testutil.PrintTxnQuery

func main() {
	start := time.Now()
	flag.BoolVar(&quiet, "q", false, "")
	flag.StringVar(&pgdb, "pg", "dbname=i2b sslmode=disable", "postgres connect string; e.g. \"dbname=foo sslmode=disable\"")
	flag.Parse()
	testutil.SetQuiet(quiet)

	db, err := idb.IndexerDbByName("postgres", pgdb, idb.IndexerDbOptions{ReadOnly: true}, nil)
	maybeFail(err, "open postgres, %v", err)

	rekeyTxnQuery := idb.TransactionFilter{RekeyTo: &truev, Limit: 1}
	printTxnQuery(db, rekeyTxnQuery)

	rowchan, _ := db.Transactions(context.Background(), rekeyTxnQuery)
	var rekeyTo basics.Address
	for txnrow := range rowchan {
		maybeFail(txnrow.Error, "err rekey txn %v\n", txnrow.Error)
		var stxn transactions.SignedTxnWithAD
		err := protocol.Decode(txnrow.TxnBytes, &stxn)
		maybeFail(err, "decode txnbytes %v\n", err)
		rekeyTo = stxn.Txn.RekeyTo
	}

	printAccountQuery(db, idb.AccountQueryOptions{EqualToAuthAddr: rekeyTo[:], Limit: 1})

	// find an asset with > 1 account
	countByAssetID := make(map[uint64]uint64)
	assetchan, _ := db.AssetBalances(context.Background(), idb.AssetBalanceQuery{})
	for abr := range assetchan {
		countByAssetID[abr.AssetID] = countByAssetID[abr.AssetID] + 1
	}
	var bestid uint64
	var bestcount uint64 = 0
	for assetid, count := range countByAssetID {
		if (bestcount == 0) || (count > 1 && count < bestcount) {
			bestcount = count
			bestid = assetid
		}
	}
	if bestcount != 0 {
		printAccountQuery(db, idb.AccountQueryOptions{HasAssetID: bestid, Limit: bestcount})
	}

	dt := time.Now().Sub(start)
	exitValue := testutil.ExitValue()
	if exitValue == 0 {
		fmt.Printf("wat OK %s\n", dt.String())
	} else {
		fmt.Printf("wat ERROR %s\n", dt.String())
	}
	os.Exit(exitValue)
}
