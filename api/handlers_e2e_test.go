package api

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"

	"github.com/algorand/go-algorand-sdk/encoding/json"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"

	"github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/postgres"
	pgtest "github.com/algorand/indexer/idb/postgres/testing"
	"github.com/algorand/indexer/util/test"
)

func setupIdb(t *testing.T, genesis bookkeeping.Genesis, genesisBlock bookkeeping.Block) (*postgres.IndexerDb /*db*/, func() /*shutdownFunc*/) {
	_, connStr, shutdownFunc := pgtest.SetupPostgres(t)

	db, _, err := postgres.OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	require.NoError(t, err)

	err = db.LoadGenesis(genesis)
	require.NoError(t, err)

	err = db.AddBlock(&genesisBlock)
	require.NoError(t, err)

	return db, shutdownFunc
}

func TestApplicationHander(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	///////////
	// Given // A block containing an app call txn with ExtraProgramPages
	///////////

	const expectedAppIdx = 1 // must be 1 since this is the first txn
	const extraPages = 2
	txn := transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "appl",
				Header: transactions.Header{
					Sender:      test.AccountA,
					GenesisHash: test.GenesisHash,
				},
				ApplicationCallTxnFields: transactions.ApplicationCallTxnFields{
					ApprovalProgram:   []byte{0x02, 0x20, 0x01, 0x01, 0x22},
					ClearStateProgram: []byte{0x02, 0x20, 0x01, 0x01, 0x22},
					ExtraProgramPages: extraPages,
				},
			},
			Sig: test.Signature,
		},
		ApplyData: transactions.ApplyData{
			ApplicationID: expectedAppIdx,
		},
	}

	block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &txn)
	require.NoError(t, err)

	err = db.AddBlock(&block)
	require.NoError(t, err, "failed to commit")

	//////////
	// When // We query the app
	//////////

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/v2/applications/:appidx")
	c.SetParamNames("appidx")
	c.SetParamValues(strconv.Itoa(expectedAppIdx))

	api := &ServerImplementation{db: db}
	params := generated.LookupApplicationByIDParams{}
	err = api.LookupApplicationByID(c, expectedAppIdx, params)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)

	//////////
	// Then // The response has non-zero ExtraProgramPages
	//////////

	var response generated.ApplicationResponse
	data := rec.Body.Bytes()
	err = json.Decode(data, &response)
	require.NoError(t, err)
	require.NotNil(t, response.Application.Params.ExtraProgramPages)
	require.Equal(t, uint64(extraPages), *response.Application.Params.ExtraProgramPages)
}

func TestBlockNotFound(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	///////////
	// Given // An empty database.
	///////////

	//////////
	// When // We query for a non-existent block.
	//////////
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/v2/blocks/:round-number")
	c.SetParamNames("round-number")
	c.SetParamValues(strconv.Itoa(100))

	api := &ServerImplementation{db: db}
	err := api.LookupBlock(c, 100)
	require.NoError(t, err)

	//////////
	// Then // A 404 gets returned.
	//////////
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, "{\"message\":\"error while looking up block for round '100': block not found\"}\n", rec.Body.String())
}

func TestInnerTxnDeduplication(t *testing.T) {
	var appAddr basics.Address
	appAddr[1] = 99
	appAddrStr := appAddr.String()

	pay := "pay"
	axfer := "axfer"
	testcases := []struct {
		name    string
		matches int
		filter  generated.SearchForTransactionsParams
	}{
		{
			name:   "match on root",
			matches: 1,
			filter: generated.SearchForTransactionsParams{Address: &appAddrStr, TxType: &pay},
		},
		{
			name:   "match on inner",
			matches: 1,
			filter: generated.SearchForTransactionsParams{Address: &appAddrStr, TxType: &pay},
		},
		{
			name:   "match on inner-inner",
			matches: 1,
			filter: generated.SearchForTransactionsParams{Address: &appAddrStr, TxType: &axfer},
		},
		{
			name:   "match all",
			matches: 3,
			filter: generated.SearchForTransactionsParams{Address: &appAddrStr},
		},
	}

	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	///////////
	// Given // a DB with some inner txns in it.
	///////////

	appCall := test.MakeAppCallWithInnerTxn(test.AccountA, appAddr, test.AccountB, appAddr, test.AccountC)
	expectedID := appCall.Txn.ID().String()

	block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &appCall)
	require.NoError(t, err)

	err = db.AddBlock(&block)
	require.NoError(t, err, "failed to commit")

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			//////////
			// When // we run a query that matches the Root Txn and Inner Txns
			//////////
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetPath("/v2/transactions/")

			api := &ServerImplementation{db: db}
			err = api.SearchForTransactions(c, tc.filter)
			require.NoError(t, err)

			//////////
			// Then // The only result is the root transaction.
			//////////
			require.Equal(t, http.StatusOK, rec.Code)
			var response generated.TransactionsResponse
			json.Decode(rec.Body.Bytes(), &response)

			require.Len(t, response.Transactions, 1)
			require.Equal(t, expectedID, *(response.Transactions[0].Id))
		})
	}
}
