package api

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/algorand/go-algorand/crypto"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"

	"github.com/algorand/go-algorand-sdk/encoding/json"
	"github.com/algorand/go-algorand/crypto/merklesignature"
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

	newShutdownFunc := func() {
		db.Close()
		shutdownFunc()
	}

	err = db.LoadGenesis(genesis)
	require.NoError(t, err)

	err = db.AddBlock(&genesisBlock)
	require.NoError(t, err)

	return db, newShutdownFunc
}

func TestApplicationHandler(t *testing.T) {
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

	api := &ServerImplementation{db: db, timeout: 30 * time.Second}
	params := generated.LookupApplicationByIDParams{}
	err = api.LookupApplicationByID(c, expectedAppIdx, params)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code, fmt.Sprintf("unexpected return code, body: %s", rec.Body.String()))

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

	api := &ServerImplementation{db: db, timeout: 30 * time.Second}
	err := api.LookupBlock(c, 100)
	require.NoError(t, err)

	//////////
	// Then // A 404 gets returned.
	//////////
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Contains(t, rec.Body.String(), errLookingUpBlockForRound)
}

// TestInnerTxn runs queries that return one or more root/inner transactions,
// and verifies that only a single root transaction is returned.
func TestInnerTxn(t *testing.T) {
	var appAddr basics.Address
	appAddr[1] = 99
	appAddrStr := appAddr.String()

	pay := "pay"
	axfer := "axfer"
	appl := "appl"
	testcases := []struct {
		name   string
		filter generated.SearchForTransactionsParams
	}{
		{
			name:   "match on root",
			filter: generated.SearchForTransactionsParams{Address: &appAddrStr, TxType: &pay},
		},
		{
			name:   "match on inner",
			filter: generated.SearchForTransactionsParams{Address: &appAddrStr, TxType: &pay},
		},
		{
			name:   "match on inner-inner",
			filter: generated.SearchForTransactionsParams{Address: &appAddrStr, TxType: &axfer},
		},
		{
			name:   "match on inner-inner",
			filter: generated.SearchForTransactionsParams{Address: &appAddrStr, TxType: &appl},
		},
		{
			name:   "match all",
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
			// When // we run a query that matches the Root Txn and/or Inner Txns
			//////////
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetPath("/v2/transactions/")

			api := &ServerImplementation{db: db, timeout: 30 * time.Second}
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

// TestPagingRootTxnDeduplication checks that paging in the middle of an inner
// transaction group does not allow the root transaction to be returned on both
// pages.
func TestPagingRootTxnDeduplication(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	///////////
	// Given // a DB with some inner txns in it.
	///////////
	var appAddr basics.Address
	appAddr[1] = 99
	appAddrStr := appAddr.String()

	appCall := test.MakeAppCallWithInnerTxn(test.AccountA, appAddr, test.AccountB, appAddr, test.AccountC)
	expectedID := appCall.Txn.ID().String()

	block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &appCall)
	require.NoError(t, err)

	err = db.AddBlock(&block)
	require.NoError(t, err, "failed to commit")

	testcases := []struct {
		name   string
		params generated.SearchForTransactionsParams
	}{
		{
			name:   "descending transaction search, middle of inner txns",
			params: generated.SearchForTransactionsParams{Address: &appAddrStr, Limit: uint64Ptr(1)},
		},
		{
			name:   "ascending transaction search, middle of inner txns",
			params: generated.SearchForTransactionsParams{Limit: uint64Ptr(2)},
		},
		{
			name:   "ascending transaction search, match root skip over inner txns",
			params: generated.SearchForTransactionsParams{Limit: uint64Ptr(1)},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			//////////
			// When // we match the first inner transaction and page to the next.
			//////////
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec1 := httptest.NewRecorder()
			c := e.NewContext(req, rec1)
			c.SetPath("/v2/transactions/")

			// Get first page with limit 1.
			// Address filter causes results to return newest to oldest.
			api := &ServerImplementation{db: db, timeout: 30 * time.Second}
			err = api.SearchForTransactions(c, tc.params)
			require.NoError(t, err)

			require.Equal(t, http.StatusOK, rec1.Code)
			var response generated.TransactionsResponse
			json.Decode(rec1.Body.Bytes(), &response)
			require.Len(t, response.Transactions, 1)
			require.Equal(t, expectedID, *(response.Transactions[0].Id))
			pageOneNextToken := *response.NextToken

			// Second page, using "NextToken" from first page.
			req = httptest.NewRequest(http.MethodGet, "/", nil)
			rec2 := httptest.NewRecorder()
			c = e.NewContext(req, rec2)
			c.SetPath("/v2/transactions/")

			// Set the next token
			tc.params.Next = &pageOneNextToken
			// In the debugger I see the internal call returning the inner tx + root tx
			err = api.SearchForTransactions(c, tc.params)
			require.NoError(t, err)

			//////////
			// Then // There are no new results on the next page.
			//////////
			var response2 generated.TransactionsResponse
			require.Equal(t, http.StatusOK, rec2.Code)
			json.Decode(rec2.Body.Bytes(), &response2)

			require.Len(t, response2.Transactions, 0)
			// The fact that NextToken changes indicates that the search results were different.
			if response2.NextToken != nil {
				require.NotEqual(t, pageOneNextToken, *response2.NextToken)
			}
		})
	}

	// Test block endpoint deduplication
	t.Run("Deduplicate Transactions In Block", func(t *testing.T) {
		//////////
		// When // we fetch the block
		//////////
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/v2/blocks/")

		// Get first page with limit 1.
		// Address filter causes results to return newest to oldest.
		api := &ServerImplementation{db: db, timeout: 30 * time.Second}
		err = api.LookupBlock(c, uint64(block.Round()))
		require.NoError(t, err)

		//////////
		// Then // There should be a single transaction which has inner transactions
		//////////
		var response generated.BlockResponse
		require.Equal(t, http.StatusOK, rec.Code)
		json.Decode(rec.Body.Bytes(), &response)

		require.NotNil(t, response.Transactions)
		require.Len(t, *response.Transactions, 1)
		require.NotNil(t, (*response.Transactions)[0])
		require.Len(t, *(*response.Transactions)[0].InnerTxns, 2)
	})
}

func TestKeyregTransactionWithStateProofKeys(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	///////////
	// Given // A block containing a key reg txn with state proof key
	///////////
	var votePK crypto.OneTimeSignatureVerifier
	votePK[0] = 1

	var selectionPK crypto.VRFVerifier
	selectionPK[0] = 1

	var stateProofPK merklesignature.Verifier
	stateProofPK[0] = 1

	txn := transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "keyreg",
				Header: transactions.Header{
					Sender:      test.AccountA,
					GenesisHash: test.GenesisHash,
				},
				KeyregTxnFields: transactions.KeyregTxnFields{
					VotePK:           votePK,
					SelectionPK:      selectionPK,
					StateProofPK:     stateProofPK,
					VoteFirst:        basics.Round(0),
					VoteLast:         basics.Round(100),
					VoteKeyDilution:  1000,
					Nonparticipation: false,
				},
			},
			Sig: test.Signature,
		},
	}

	block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &txn)
	require.NoError(t, err)

	err = db.AddBlock(&block)
	require.NoError(t, err, "failed to commit")

	e := echo.New()
	{
		//////////
		// When // We query the txn
		//////////
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/v2/transactions/:txid")
		api := &ServerImplementation{db: db}
		err = api.LookupTransaction(c, txn.Txn.ID().String())
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, rec.Code)
		//////////
		// Then // The key reg txn response has state proof key
		//////////
		var response generated.TransactionResponse
		data := rec.Body.Bytes()
		err = json.Decode(data, &response)
		require.NoError(t, err)
		require.NotNil(t, response.Transaction.KeyregTransaction.StateProofKey)
		require.Equal(t, stateProofPK[:], *response.Transaction.KeyregTransaction.StateProofKey)
	}
	{
		//////////
		// And // Account is online with state proof key
		//////////
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/v2/accounts/:account-id")
		api := &ServerImplementation{db: db}
		params := generated.LookupAccountByIDParams{}
		err = api.LookupAccountByID(c, test.AccountA.String(), params)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, rec.Code)

		var acctResp generated.AccountResponse
		data := rec.Body.Bytes()
		err = json.Decode(data, &acctResp)
		require.NoError(t, err)
		require.NotNil(t, acctResp.Account)
		require.NotNil(t, acctResp.Account.Participation.StateProofKey)
		require.Equal(t, stateProofPK[:], *acctResp.Account.Participation.StateProofKey)
	}
}

func TestVersion(t *testing.T) {
	///////////
	// Given // An API and context
	///////////
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()
	api := &ServerImplementation{db: db, timeout: 30 * time.Second}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec1 := httptest.NewRecorder()
	c := e.NewContext(req, rec1)

	//////////
	// When // we call the health endpoint
	//////////
	err := api.MakeHealthCheck(c)

	//////////
	// Then // We get the health information.
	//////////
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec1.Code)
	var response generated.HealthCheckResponse
	json.Decode(rec1.Body.Bytes(), &response)

	require.Equal(t, uint64(0), response.Round)
	require.False(t, response.IsMigrating)
	// This is weird looking because the version is set with -ldflags
	require.Equal(t, response.Version, "(unknown version)")
}

func TestAccountClearsNonUTF8(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	///////////
	// Given // a DB with some inner txns in it.
	///////////
	//var createAddr basics.Address
	//createAddr[1] = 99
	//createAddrStr := createAddr.String()

	assetName := "valid"
	//url := "https://my.embedded.\000.null.asset"
	urlBytes, _ := base64.StdEncoding.DecodeString("8J+qmSBNb25leQ==")
	url := string(urlBytes)
	unitName := "asset\rwith\nnon-printable\tcharacters"
	createAsset := test.MakeAssetConfigTxn(0, 100, 0, false, unitName, assetName, url, test.AccountA)

	block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &createAsset)
	require.NoError(t, err)

	err = db.AddBlock(&block)
	require.NoError(t, err, "failed to commit")

	verify := func(params generated.AssetParams) {
		compareB64 := func(expected string, actual *[]byte) {
			actualStr := string(*actual)
			require.Equal(t, expected, actualStr)
		}

		// In all cases, the B64 encoded names should be the same.
		compareB64(assetName, params.NameB64)
		compareB64(unitName, params.UnitNameB64)
		compareB64(url, params.UrlB64)

		require.Equal(t, assetName, *params.Name, "valid utf8 should remain")
		require.Nil(t, params.UnitName, "null bytes should not be displayed")
		require.Nil(t, params.Url, "non printable characters should not be displayed")
	}

	{
		//////////
		// When // we lookup the asset
		//////////
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/v2/assets/")

		api := &ServerImplementation{db: db, timeout: 30 * time.Second}
		err = api.SearchForAssets(c, generated.SearchForAssetsParams{})
		require.NoError(t, err)

		require.Equal(t, http.StatusOK, rec.Code)
		var response generated.AssetsResponse
		json.Decode(rec.Body.Bytes(), &response)

		//////////
		// Then // we should find one asset with the expected string encodings
		//////////
		require.Len(t, response.Assets, 1)
		verify(response.Assets[0].Params)
	}

	{
		//////////
		// When // we lookup the account
		//////////
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/v2/accounts/")

		api := &ServerImplementation{db: db, timeout: 30 * time.Second}
		err = api.LookupAccountByID(c, test.AccountA.String(), generated.LookupAccountByIDParams{})
		require.NoError(t, err)

		require.Equal(t, http.StatusOK, rec.Code)
		var response generated.AccountResponse
		json.Decode(rec.Body.Bytes(), &response)

		//////////
		// Then // we should find one asset with the expected string encodings
		//////////
		require.NotNil(t, response.Account.CreatedAssets, 1)
		require.Len(t, *response.Account.CreatedAssets, 1)
		verify((*response.Account.CreatedAssets)[0].Params)
	}
}

// TestLookupInnerLogs runs queries for logs given application ids,
// and checks that logs in inner transactions match properly.
func TestLookupInnerLogs(t *testing.T) {
	var appAddr basics.Address
	appAddr[1] = 99

	params := generated.LookupApplicationLogsByIDParams{}

	testcases := []struct {
		name  string
		appID uint64
		logs  []string
	}{
		{
			name:  "match on root",
			appID: 123,
			logs: []string{
				"testing outer appl log",
				"appId 123 log",
			},
		},
		{
			name:  "match on inner",
			appID: 789,
			logs: []string{
				"testing inner log",
				"appId 789 log",
			},
		},
		{
			name:  "match on inner-inner",
			appID: 999,
			logs: []string{
				"testing inner-inner log",
				"appId 999 log",
			},
		},
	}

	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	///////////
	// Given // a DB with some inner txns in it.
	///////////
	appCall := test.MakeAppCallWithInnerAppCall(test.AccountA)

	block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &appCall)
	require.NoError(t, err)

	err = db.AddBlock(&block)
	require.NoError(t, err, "failed to commit")

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			//////////
			// When // we run a query that queries logs based on appID
			//////////
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetPath("/v2/applications/:appIdx/logs")
			c.SetParamNames("appIdx")
			c.SetParamValues(fmt.Sprintf("%d", tc.appID))

			api := &ServerImplementation{db: db, timeout: 30 * time.Second}
			err = api.LookupApplicationLogsByID(c, tc.appID, params)
			require.NoError(t, err)

			//////////
			// Then // The result is the log from the app
			//////////
			var response generated.ApplicationLogsResponse
			require.Equal(t, http.StatusOK, rec.Code)
			json.Decode(rec.Body.Bytes(), &response)
			require.NoError(t, err)

			require.Equal(t, uint64(tc.appID), response.ApplicationId)
			require.NotNil(t, response.LogData)
			ld := *response.LogData
			require.Equal(t, 1, len(ld))
			require.Equal(t, len(tc.logs), len(ld[0].Logs))
			for i, log := range ld[0].Logs {
				require.Equal(t, []byte(tc.logs[i]), log)
			}
		})
	}
}
