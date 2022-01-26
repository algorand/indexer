package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/algorand/go-algorand/data/basics"

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

	db, availableCh, err := idb.IndexerDbByName("postgres", pgdb, idb.IndexerDbOptions{ReadOnly: true}, nil)
	maybeFail(err, "open postgres, %v", err)
	<-availableCh

	// TODO: this is fragile, I don't need to hit exactly 4 here; just more than 1, less than 100. Refactor PrintTxnQuery ?
	rekeyTxnQuery := idb.TransactionFilter{RekeyTo: &truev, Limit: 4}
	printTxnQuery(db, rekeyTxnQuery)

	var rekeyedAuthAddrs []basics.Address
	rowchan, _ := db.Transactions(context.Background(), rekeyTxnQuery)
	for txnrow := range rowchan {
		maybeFail(txnrow.Error, "err rekey txn %v\n", txnrow.Error)
		rekeyedAuthAddrs = append(rekeyedAuthAddrs, txnrow.Txn.Txn.RekeyTo)
	}

	// some rekeys get rekeyed back; some rekeyed accounts get deleted, just want to find at least one
	foundRekey := false
	for _, rekeyTo := range rekeyedAuthAddrs {
		// TODO: refactor? this is like PrintAccountQuery but without setting the global error.
		accountchan, _ := db.GetAccounts(context.Background(), idb.AccountQueryOptions{EqualToAuthAddr: rekeyTo[:], Limit: 1})
		count := uint64(0)
		for ar := range accountchan {
			util.MaybeFail(ar.Error, "GetAccounts err %v\n", ar.Error)
			count++
		}
		if count == 1 {
			foundRekey = true
		}
	}
	if !foundRekey {
		// this will report the error in a handy way
		printAccountQuery(db, idb.AccountQueryOptions{EqualToAuthAddr: rekeyedAuthAddrs[0][:], Limit: 1})
	}

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

	dt := time.Since(start)
	exitValue := testutil.ExitValue()
	if exitValue == 0 {
		fmt.Printf("wat OK %s\n", dt.String())
	} else {
		fmt.Printf("wat ERROR %s\n", dt.String())
	}
	os.Exit(exitValue)
}
