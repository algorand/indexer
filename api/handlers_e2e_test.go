package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/algorand/go-algorand-sdk/encoding/json"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"

	"github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/mocks"
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
	require.Contains(t, rec.Body.String(), errLookingUpBlock)
}

// TestInnerTxn runs queries that return one or more root/inner transactions,
// and verifies that only a single root transaction is returned.
func TestInnerTxn(t *testing.T) {
	var appAddr basics.Address
	appAddr[1] = 99
	appAddrStr := appAddr.String()

	pay := "pay"
	axfer := "axfer"
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

func TestTimeouts(t *testing.T) {
	// function pointers to execute the different DB operations. We really only
	// care that they timeout with WaitUntil, but the return arguments need to
	// be correct to avoid a panic.
	mostMockFunctions := func(method string) func(mockIndexer *mocks.IndexerDb, timeout <-chan time.Time) {
		return func(mockIndexer *mocks.IndexerDb, timeout <-chan time.Time) {
			mockIndexer.
				On(method, mock.Anything, mock.Anything, mock.Anything).
				WaitUntil(timeout).
				Return(nil, uint64(0))
		}
	}
	transactionFunc := mostMockFunctions("Transactions")
	applicationsFunc := mostMockFunctions("Applications")
	accountsFunc := mostMockFunctions("GetAccounts")
	assetsFunc := mostMockFunctions("Assets")
	balancesFunc := mostMockFunctions("AssetBalances")
	blockFunc := func(mockIndexer *mocks.IndexerDb, timeout <-chan time.Time) {
		mockIndexer.
			On("GetBlock", mock.Anything, mock.Anything, mock.Anything).
			WaitUntil(timeout).
			Return(bookkeeping.BlockHeader{}, nil, nil)
	}
	healthFunc := func(mockIndexer *mocks.IndexerDb, timeout <-chan time.Time) {
		mockIndexer.
			On("Health", mock.Anything, mock.Anything, mock.Anything).
			WaitUntil(timeout).
			Return(idb.Health{}, nil)
	}

	// Call each of the handlers and let the database timeout.
	testcases := []struct {
		name        string
		errString   string
		mockCall    func(mockIndexer *mocks.IndexerDb, timeout <-chan time.Time)
		callHandler func(ctx echo.Context, si ServerImplementation) error
	}{
		{
			name:      "SearchForTransactions",
			errString: errTransactionSearch,
			mockCall:  transactionFunc,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.SearchForTransactions(ctx, generated.SearchForTransactionsParams{})
			},
		},
		{
			name:      "LookupAccountTransactions",
			errString: errTransactionSearch,
			mockCall:  transactionFunc,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.LookupAccountTransactions(ctx, "", generated.LookupAccountTransactionsParams{})
			},
		},
		{
			name:      "LookupAssetTransactions",
			errString: errTransactionSearch,
			mockCall:  transactionFunc,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.LookupAssetTransactions(ctx, 1, generated.LookupAssetTransactionsParams{})
			},
		},
		{
			name:      "LookupApplicaitonLogsByID",
			errString: errTransactionSearch,
			mockCall:  transactionFunc,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.LookupApplicationLogsByID(ctx, 1, generated.LookupApplicationLogsByIDParams{})
			},
		},
		{
			name:      "LookupApplicationByID",
			errString: errFailedSearchingApplication,
			mockCall:  applicationsFunc,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.LookupApplicationByID(ctx, 0, generated.LookupApplicationByIDParams{})
			},
		},
		{
			name:      "SearchForApplications",
			errString: errFailedSearchingApplication,
			mockCall:  applicationsFunc,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.SearchForApplications(ctx, generated.SearchForApplicationsParams{})
			},
		},
		{
			name:      "SearchForAccount",
			errString: errFailedSearchingAccount,
			mockCall:  accountsFunc,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.SearchForAccounts(ctx, generated.SearchForAccountsParams{})
			},
		},
		{
			name:      "LookupAccountByID",
			errString: errFailedSearchingAccount,
			mockCall:  accountsFunc,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.LookupAccountByID(ctx,
					"PBH2JQNVP5SBXLTOWNHHPGU6FUMBVS4ZDITPK5RA5FG2YIIFS6UYEMFM2Y",
					generated.LookupAccountByIDParams{})
			},
		},
		{
			name:      "SearchForAssets",
			errString: errFailedSearchingAsset,
			mockCall:  assetsFunc,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.SearchForAssets(ctx, generated.SearchForAssetsParams{})
			},
		},
		{
			name:      "LookupAssetByID",
			errString: errFailedSearchingAsset,
			mockCall:  assetsFunc,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.LookupAssetByID(ctx, 1, generated.LookupAssetByIDParams{})
			},
		},
		{
			name:      "LookupAssetBalances",
			errString: errFailedSearchingAssetBalances,
			mockCall:  balancesFunc,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.LookupAssetBalances(ctx, 1, generated.LookupAssetBalancesParams{})
			},
		},
		{
			name:      "LookupBlock",
			errString: errLookingUpBlock,
			mockCall:  blockFunc,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.LookupBlock(ctx, 100)
			},
		},
		{
			name:      "Health",
			errString: errFailedLookingUpHealth,
			mockCall:  healthFunc,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.MakeHealthCheck(ctx)
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			timeout := make(chan time.Time, 1)
			defer func() {
				timeout <- time.Now()
				close(timeout)
			}()

			// Make a mock indexer and tell the mock to timeout.
			mockIndexer := &mocks.IndexerDb{}

			si := ServerImplementation{
				db:      mockIndexer,
				timeout: 5 * time.Millisecond,
			}

			// Setup context...
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec1 := httptest.NewRecorder()
			c := e.NewContext(req, rec1)

			// configure the mock to timeout, then call the handler.
			tc.mockCall(mockIndexer, timeout)
			err := tc.callHandler(c, si)

			require.NoError(t, err)
			bodyStr := rec1.Body.String()
			require.Equal(t, 500, rec1.Code)
			require.Contains(t, bodyStr, tc.errString)
			require.Contains(t, bodyStr, "timeout")
		})
	}
}
