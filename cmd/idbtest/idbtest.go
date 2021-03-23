// Test idb package back-end interface
// Requires a Postgres database with mainnet data indexed
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	ajson "github.com/algorand/go-algorand-sdk/encoding/json"
	atypes "github.com/algorand/go-algorand-sdk/types"

	"github.com/algorand/indexer/accounting"
	"github.com/algorand/indexer/idb"
	_ "github.com/algorand/indexer/idb/postgres"
	"github.com/algorand/indexer/types"
	testutil "github.com/algorand/indexer/util/test"
)

var (
	quiet       = false
	txntest     = true
	pagingtest  = true
	assettest   = true
	accounttest = true
	pgdb        = "dbname=i2b sslmode=disable"
)

var exitValue = 0

var maybeFail = testutil.MaybeFail
var printAssetQuery func(db idb.IndexerDb, q idb.AssetsQuery) = testutil.PrintAssetQuery
var printAccountQuery = testutil.PrintAccountQuery
var printTxnQuery = testutil.PrintTxnQuery
var info = testutil.Info

func doAssetQueryTests(db idb.IndexerDb) {
	printAssetQuery(db, idb.AssetsQuery{Query: "us", Limit: 9})
	printAssetQuery(db, idb.AssetsQuery{Name: "Tether USDt", Limit: 1})
	printAssetQuery(db, idb.AssetsQuery{Unit: "USDt", Limit: 2})
	printAssetQuery(db, idb.AssetsQuery{AssetId: 312769, Limit: 1})
	printAssetQuery(db, idb.AssetsQuery{AssetIdGreaterThan: 312769, Query: "us", Limit: 2})
	tcreator, err := atypes.DecodeAddress("XIU7HGGAJ3QOTATPDSIIHPFVKMICXKHMOR2FJKHTVLII4FAOA3CYZQDLG4")
	maybeFail(err, "addr decode, %v\n", err)
	printAssetQuery(db, idb.AssetsQuery{Creator: tcreator[:], Limit: 1})
}

// like TxnRow
type roundIntra struct {
	Round uint64
	Intra int
}

func testTxnPaging(db idb.IndexerDb, q idb.TransactionFilter) {
	q.Limit = 1000
	all := make([]roundIntra, 0, q.Limit)
	rowchan := db.Transactions(context.Background(), q)
	for txnrow := range rowchan {
		var ri roundIntra
		ri.Round = txnrow.Round
		ri.Intra = txnrow.Intra
		all = append(all, ri)
	}

	q.Limit = uint64((len(all) / 3) + 1)
	pos := 0

	page := 0
	any := true
	for any {
		any = false
		rowchan := db.Transactions(context.Background(), q)
		next := ""
		for txnrow := range rowchan {
			any = true
			ri := all[pos]
			if ri.Round != txnrow.Round {
				fmt.Printf("page %d result[%d] round mismatch orig %d, paged %d\n", page, pos, ri.Round, txnrow.Round)
				exitValue = 1
			}
			if ri.Intra != txnrow.Intra {
				fmt.Printf("page %d result[%d] intra mismatch orig %d, paged %d\n", page, pos, ri.Intra, txnrow.Intra)
				exitValue = 1
			}
			pos++
			if pos == len(all) {
				// done, paging might continue but we got all we wanted
				any = false // get out
				break
			}
			next = txnrow.Next()
		}
		q.NextToken = next
		page++
	}
	if pos != len(all) {
		fmt.Printf("oneshot had %d results but paged %d\n", len(all), pos)
		exitValue = 1
	}
	if exitValue == 0 {
		info("ok fetching %d entries over %d pages\n", len(all), page)
	}
}

func printAssetBalanceQuery(db idb.IndexerDb, assetId uint64) {
	rows := db.AssetBalances(context.Background(), idb.AssetBalanceQuery{AssetId: assetId})
	count := 0
	for row := range rows {
		maybeFail(row.Error, "err %v\n", row.Error)
		var addr types.Address
		copy(addr[:], row.Address)
		fmt.Printf("%s %d %12d %t\n", addr.String(), row.AssetId, row.Amount, row.Frozen)
		count++
	}
	fmt.Printf("%d asset balances\n", count)
}

func main() {
	start := time.Now()
	flag.BoolVar(&accounttest, "accounts", true, "")
	flag.BoolVar(&assettest, "assets", true, "")
	flag.BoolVar(&pagingtest, "paging", true, "")
	flag.BoolVar(&txntest, "txn", true, "")

	flag.BoolVar(&quiet, "q", false, "")
	flag.StringVar(&pgdb, "pg", "dbname=i2b sslmode=disable", "postgres connect string; e.g. \"dbname=foo sslmode=disable\"")
	flag.Parse()
	testutil.SetQuiet(quiet)

	db, err := idb.IndexerDbByName("postgres", pgdb, nil)
	maybeFail(err, "open postgres, %v", err)

	if accounttest {
		printAccountQuery(db, idb.AccountQueryOptions{IncludeAssetHoldings: true, IncludeAssetParams: true, AlgosGreaterThan: 10000000000, Limit: 20})
		printAccountQuery(db, idb.AccountQueryOptions{HasAssetId: 312769, Limit: 19})
	}
	if assettest {
		doAssetQueryTests(db)
	}

	if false {
		// account rewind debug
		xa, _ := atypes.DecodeAddress("QRP4AJLQXHJ42VJ5PSGAH53IVVACYCI6ZDRJMF4JPRFY5VKSYKFWKKMFVU")
		account, err := idb.GetAccount(db, xa[:])
		fmt.Printf("account %s\n", string(ajson.Encode(account)))
		maybeFail(err, "addr lookup, %v", err)
		round := uint64(5426258)
		tf := idb.TransactionFilter{
			Address:  xa[:],
			MinRound: round - 100,
			MaxRound: account.Round,
		}
		printTxnQuery(db, tf)
		raccount, err := accounting.AccountAtRound(account, round, db)
		maybeFail(err, "AccountAtRound, %v", err)
		fmt.Printf("raccount %s\n", string(ajson.Encode(raccount)))
	}

	if txntest {
		// txn query tests
		xa, _ := atypes.DecodeAddress("QRP4AJLQXHJ42VJ5PSGAH53IVVACYCI6ZDRJMF4JPRFY5VKSYKFWKKMFVU")
		printTxnQuery(db, idb.TransactionFilter{Limit: 2})
		printTxnQuery(db, idb.TransactionFilter{MinRound: 5000000, Limit: 2})
		printTxnQuery(db, idb.TransactionFilter{MaxRound: 100000, Limit: 2})
		printTxnQuery(db, idb.TransactionFilter{AssetId: 604, Limit: 2})
		printTxnQuery(db, idb.TransactionFilter{TypeEnum: 2, Limit: 2}) // keyreg
		offset := uint64(3)
		printTxnQuery(db, idb.TransactionFilter{Offset: &offset, Limit: 2})
		printTxnQuery(db, idb.TransactionFilter{SigType: "lsig", Limit: 2})
		printTxnQuery(db, idb.TransactionFilter{NotePrefix: []byte("a"), Limit: 2})
		printTxnQuery(db, idb.TransactionFilter{AlgosGT: 10000000, Limit: 2})
		printTxnQuery(db, idb.TransactionFilter{EffectiveAmountGt: 10000000, Limit: 2})
		printTxnQuery(db, idb.TransactionFilter{EffectiveAmountLt: 1000000, Limit: 2})
		printTxnQuery(db, idb.TransactionFilter{Address: xa[:], Limit: 6})
		printTxnQuery(db, idb.TransactionFilter{Address: xa[:], AddressRole: idb.AddressRoleSender, Limit: 2})
		printTxnQuery(db, idb.TransactionFilter{Address: xa[:], AddressRole: idb.AddressRoleReceiver, Limit: 2})
		printTxnQuery(db, idb.TransactionFilter{AssetAmountGT: 99, Limit: 2})
		printTxnQuery(db, idb.TransactionFilter{AssetAmountLT: 100, Limit: 2})
	}

	//printTxnQuery(db, idb.TransactionFilter{AssetId: 312769, Limit: 30})
	//printTxnQuery(db, idb.TransactionFilter{Address: xa[:], AssetId: 312769, Limit: 30})
	//printAssetBalanceQuery(db, 312769)

	if pagingtest {
		xa, _ := atypes.DecodeAddress("QRP4AJLQXHJ42VJ5PSGAH53IVVACYCI6ZDRJMF4JPRFY5VKSYKFWKKMFVU")
		testTxnPaging(db, idb.TransactionFilter{Address: xa[:]})
		testTxnPaging(db, idb.TransactionFilter{TypeEnum: 2})
		testTxnPaging(db, idb.TransactionFilter{AlgosGT: 1})
	}

	dt := time.Now().Sub(start)
	exitValue += testutil.ExitValue()
	if exitValue == 0 {
		fmt.Printf("wat OK %s\n", dt.String())
	} else {
		fmt.Printf("wat ERROR %s\n", dt.String())
	}
	os.Exit(exitValue)
}
