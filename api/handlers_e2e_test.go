package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/algorand/indexer/v3/api/generated/v2"
	"github.com/algorand/indexer/v3/idb"
	"github.com/algorand/indexer/v3/idb/postgres"
	pgtest "github.com/algorand/indexer/v3/idb/postgres/testing"
	"github.com/algorand/indexer/v3/types"
	"github.com/algorand/indexer/v3/util"
	"github.com/algorand/indexer/v3/util/test"

	"github.com/algorand/avm-abi/apps"
	"github.com/algorand/go-algorand-sdk/v2/crypto"
	"github.com/algorand/go-algorand-sdk/v2/encoding/json"
	sdk "github.com/algorand/go-algorand-sdk/v2/types"
)

var defaultOpts = ExtraOptions{
	MaxAPIResourcesPerAccount: 1000,

	MaxTransactionsLimit:     10000,
	DefaultTransactionsLimit: 1000,

	MaxAccountsLimit:     1000,
	DefaultAccountsLimit: 100,

	MaxAssetsLimit:     1000,
	DefaultAssetsLimit: 100,

	MaxBalancesLimit:     10000,
	DefaultBalancesLimit: 1000,

	MaxApplicationsLimit:     1000,
	DefaultApplicationsLimit: 100,

	MaxBoxesLimit:     10000,
	DefaultBoxesLimit: 1000,

	DisabledMapConfig: MakeDisabledMapConfig(),
}

type boxTestComparator func(t *testing.T, db *postgres.IndexerDb, appBoxes map[sdk.AppIndex]map[string]string,
	deletedBoxes map[sdk.AppIndex]map[string]bool, verifyTotals bool)

func testServerImplementation(db idb.IndexerDb) *ServerImplementation {
	return &ServerImplementation{db: db, timeout: 30 * time.Second, opts: defaultOpts}
}

func setupIdb(t *testing.T, genesis sdk.Genesis) (*postgres.IndexerDb, func()) {
	_, connStr, shutdownFunc := pgtest.SetupPostgres(t)

	db, _, err := postgres.OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	require.NoError(t, err)

	newShutdownFunc := func() {
		db.Close()
		shutdownFunc()
	}

	err = db.LoadGenesis(genesis)
	vb := types.ValidatedBlock{
		Block: test.MakeGenesisBlock(),
		Delta: sdk.LedgerStateDelta{},
	}
	db.AddBlock(&vb)
	require.NoError(t, err)

	return db, newShutdownFunc
}

func TestApplicationHandlers(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	///////////
	// Given // A block containing an app call txn with ExtraProgramPages, that the creator and another account have opted into
	///////////

	//txn := transactions.SignedTxnWithAD{
	//	SignedTxn: transactions.SignedTxn{
	//		Txn: transactions.Transaction{
	//			Type: "appl",
	//			Header: transactions.Header{
	//				Sender:      test.AccountA,
	//				GenesisHash: test.GenesisHash,
	//			},
	//			ApplicationCallTxnFields: transactions.ApplicationCallTxnFields{
	//				ApprovalProgram:   []byte{0x02, 0x20, 0x01, 0x01, 0x22},
	//				ClearStateProgram: []byte{0x02, 0x20, 0x01, 0x01, 0x22},
	//				ExtraProgramPages: extraPages,
	//			},
	//		},
	//		Sig: test.Signature,
	//	},
	//	ApplyData: transactions.ApplyData{
	//		ApplicationID: expectedAppIdx,
	//	},
	//}
	//optInTxnA := test.MakeAppOptInTxn(expectedAppIdx, test.AccountA)
	//optInTxnB := test.MakeAppOptInTxn(expectedAppIdx, test.AccountB)
	const expectedAppIdx = 1 // must be 1 since this is the first txn
	const extraPages = 2
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/ApplicationHandlers.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	//////////
	// When // We query the app
	//////////

	setupReq := func(path, paramName, paramValue string) (echo.Context, *ServerImplementation, *httptest.ResponseRecorder) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath(path)
		c.SetParamNames(paramName)
		c.SetParamValues(paramValue)
		api := testServerImplementation(db)
		return c, api, rec
	}

	c, api, rec := setupReq("/v2/applications/:appidx", "appidx", strconv.Itoa(expectedAppIdx))
	params := generated.LookupApplicationByIDParams{}
	err = api.LookupApplicationByID(c, expectedAppIdx, params)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code, fmt.Sprintf("unexpected return code, body: %s", rec.Body.String()))

	//////////
	// Then // The response has non-zero ExtraProgramPages and other app data
	//////////

	checkApp := func(t *testing.T, app *generated.Application) {
		require.NotNil(t, app)
		require.NotNil(t, app.Params.ExtraProgramPages)
		require.Equal(t, uint64(extraPages), *app.Params.ExtraProgramPages)
		require.Equal(t, app.Id, uint64(expectedAppIdx))
		require.NotNil(t, app.Params.Creator)
		require.Equal(t, *app.Params.Creator, test.AccountA.String())
		require.Equal(t, app.Params.ApprovalProgram, []byte{0x02, 0x20, 0x01, 0x01, 0x22})
		require.Equal(t, app.Params.ClearStateProgram, []byte{0x02, 0x20, 0x01, 0x01, 0x22})
	}

	var response generated.ApplicationResponse
	data := rec.Body.Bytes()
	err = json.Decode(data, &response)
	require.NoError(t, err)
	checkApp(t, response.Application)

	t.Run("created-applications", func(t *testing.T) {
		//////////
		// When // We look up the app by creator address
		//////////

		c, api, rec := setupReq("/v2/accounts/:accountid/created-applications", "accountid", test.AccountA.String())
		params := generated.LookupAccountCreatedApplicationsParams{}
		err = api.LookupAccountCreatedApplications(c, test.AccountA.String(), params)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, rec.Code, fmt.Sprintf("unexpected return code, body: %s", rec.Body.String()))

		//////////
		// Then // The response has non-zero ExtraProgramPages and other app data
		//////////

		var response generated.ApplicationsResponse
		data := rec.Body.Bytes()
		err = json.Decode(data, &response)
		require.NoError(t, err)
		require.Len(t, response.Applications, 1)
		checkApp(t, &response.Applications[0])
	})

	checkAppLocalState := func(t *testing.T, ls *generated.ApplicationLocalState) {
		require.NotNil(t, ls)
		require.NotNil(t, ls.Deleted)
		require.False(t, *ls.Deleted)
		require.Equal(t, ls.Id, uint64(expectedAppIdx))
	}

	for _, tc := range []struct{ name, addr string }{
		{"creator", test.AccountA.String()},
		{"opted-in-account", test.AccountB.String()},
	} {
		t.Run("app-local-state-"+tc.name, func(t *testing.T) {
			//////////
			// When // We look up the app's local state for an address that has opted in
			//////////

			c, api, rec := setupReq("/v2/accounts/:accountid/apps-local-state", "accountid", test.AccountA.String())
			params := generated.LookupAccountAppLocalStatesParams{}
			err = api.LookupAccountAppLocalStates(c, tc.addr, params)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, rec.Code, fmt.Sprintf("unexpected return code, body: %s", rec.Body.String()))

			//////////
			// Then // AppLocalState is available for that address
			//////////

			var response generated.ApplicationLocalStatesResponse
			data := rec.Body.Bytes()
			err = json.Decode(data, &response)
			require.NoError(t, err)
			require.Len(t, response.AppsLocalStates, 1)
			checkAppLocalState(t, &response.AppsLocalStates[0])
		})
	}
}

func TestAccountExcludeParameters(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	///////////
	// Given // A block containing a creator of an app, an asset, who also holds and has opted-into those apps.
	///////////

	//const expectedAppIdx = 1 // must be 1 since this is the first txn
	//const expectedAssetIdx = 2
	//createAppTxn := test.MakeCreateAppTxn(test.AccountA)
	//createAssetTxn := test.MakeAssetConfigTxn(0, 100, 0, false, "UNIT", "Asset 2", "http://asset2.com", test.AccountA)
	//appOptInTxnA := test.MakeAppOptInTxn(expectedAppIdx, test.AccountA)
	//appOptInTxnB := test.MakeAppOptInTxn(expectedAppIdx, test.AccountB)
	//assetOptInTxnA := test.MakeAssetOptInTxn(expectedAssetIdx, test.AccountA)
	//assetOptInTxnB := test.MakeAssetOptInTxn(expectedAssetIdx, test.AccountB)

	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/AccountExcludeParameters.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	//////////
	// When // We look up the address using various exclude parameters.
	//////////

	setupReq := func(path, paramName, paramValue string) (echo.Context, *ServerImplementation, *httptest.ResponseRecorder) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath(path)
		c.SetParamNames(paramName)
		c.SetParamValues(paramValue)
		api := testServerImplementation(db)
		return c, api, rec
	}

	//////////
	// Then // Those parameters are excluded.
	//////////

	testCases := []struct {
		address        sdk.Address
		exclude        []generated.LookupAccountByIDParamsExclude
		check          func(*testing.T, generated.AccountResponse)
		errStatus      int
		includeDeleted bool
	}{{
		address: sdk.Address(test.AccountA),
		exclude: []generated.LookupAccountByIDParamsExclude{"all"},
		check: func(t *testing.T, r generated.AccountResponse) {
			require.Nil(t, r.Account.CreatedAssets)
			require.Nil(t, r.Account.CreatedApps)
			require.Nil(t, r.Account.Assets)
			require.Nil(t, r.Account.AppsLocalState)
		}}, {
		address: sdk.Address(test.AccountA),
		exclude: []generated.LookupAccountByIDParamsExclude{"none"},
		check: func(t *testing.T, r generated.AccountResponse) {
			require.NotNil(t, r.Account.CreatedAssets)
			require.NotNil(t, r.Account.CreatedApps)
			require.NotNil(t, r.Account.Assets)
			require.NotNil(t, r.Account.AppsLocalState)
		}}, {
		address:        sdk.Address(test.AccountA),
		exclude:        []generated.LookupAccountByIDParamsExclude{},
		includeDeleted: true,
		check: func(t *testing.T, r generated.AccountResponse) {
			require.NotNil(t, r.Account.CreatedAssets)
			require.NotNil(t, r.Account.CreatedApps)
			require.NotNil(t, r.Account.Assets)
			require.NotNil(t, r.Account.AppsLocalState)
		}}, {
		address: sdk.Address(test.AccountA),
		check: func(t *testing.T, r generated.AccountResponse) {
			require.NotNil(t, r.Account.CreatedAssets)
			require.NotNil(t, r.Account.CreatedApps)
			require.NotNil(t, r.Account.Assets)
			require.NotNil(t, r.Account.AppsLocalState)
		}}, {
		address: sdk.Address(test.AccountA),
		exclude: []generated.LookupAccountByIDParamsExclude{"created-assets", "created-apps", "apps-local-state", "assets"},
		check: func(t *testing.T, r generated.AccountResponse) {
			require.Nil(t, r.Account.CreatedAssets)
			require.Nil(t, r.Account.CreatedApps)
			require.Nil(t, r.Account.Assets)
			require.Nil(t, r.Account.AppsLocalState)
		}}, {
		address: sdk.Address(test.AccountA),
		exclude: []generated.LookupAccountByIDParamsExclude{"created-assets"},
		check: func(t *testing.T, r generated.AccountResponse) {
			require.Nil(t, r.Account.CreatedAssets)
			require.NotNil(t, r.Account.CreatedApps)
			require.NotNil(t, r.Account.Assets)
			require.NotNil(t, r.Account.AppsLocalState)
		}}, {
		address: sdk.Address(test.AccountA),
		exclude: []generated.LookupAccountByIDParamsExclude{"created-apps"},
		check: func(t *testing.T, r generated.AccountResponse) {
			require.NotNil(t, r.Account.CreatedAssets)
			require.Nil(t, r.Account.CreatedApps)
			require.NotNil(t, r.Account.Assets)
			require.NotNil(t, r.Account.AppsLocalState)
		}}, {
		address: sdk.Address(test.AccountA),
		exclude: []generated.LookupAccountByIDParamsExclude{"apps-local-state"},
		check: func(t *testing.T, r generated.AccountResponse) {
			require.NotNil(t, r.Account.CreatedAssets)
			require.NotNil(t, r.Account.CreatedApps)
			require.NotNil(t, r.Account.Assets)
			require.Nil(t, r.Account.AppsLocalState)
		}}, {
		address: sdk.Address(test.AccountA),
		exclude: []generated.LookupAccountByIDParamsExclude{"assets"},
		check: func(t *testing.T, r generated.AccountResponse) {
			require.NotNil(t, r.Account.CreatedAssets)
			require.NotNil(t, r.Account.CreatedApps)
			require.Nil(t, r.Account.Assets)
			require.NotNil(t, r.Account.AppsLocalState)
		}}, {
		address: sdk.Address(test.AccountB),
		exclude: []generated.LookupAccountByIDParamsExclude{"assets", "apps-local-state"},
		check: func(t *testing.T, r generated.AccountResponse) {
			require.Nil(t, r.Account.CreatedAssets)
			require.Nil(t, r.Account.CreatedApps)
			require.Nil(t, r.Account.Assets)
			require.Nil(t, r.Account.AppsLocalState)
		}},
		{
			address:   sdk.Address(test.AccountA),
			exclude:   []generated.LookupAccountByIDParamsExclude{"abc"},
			errStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("exclude %v", tc.exclude), func(t *testing.T) {
			c, api, rec := setupReq("/v2/accounts/:account-id", "account-id", tc.address.String())
			err := api.LookupAccountByID(c, tc.address.String(), generated.LookupAccountByIDParams{IncludeAll: &tc.includeDeleted, Exclude: &tc.exclude})
			require.NoError(t, err)
			if tc.errStatus != 0 {
				require.Equal(t, tc.errStatus, rec.Code)
				return
			}
			require.Equal(t, http.StatusOK, rec.Code, fmt.Sprintf("unexpected return code, body: %s", rec.Body.String()))
			data := rec.Body.Bytes()
			var response generated.AccountResponse
			err = json.Decode(data, &response)
			require.NoError(t, err)
			tc.check(t, response)
		})
	}

}

type accountsErrorResponse struct {
	Data struct {
		Address            *string `json:"address,omitempty"`
		MaxResults         *uint64 `json:"max-results,omitempty"`
		Message            string  `json:"message"`
		TotalAppsOptedIn   *uint64 `json:"total-apps-opted-in,omitempty"`
		TotalAssetsOptedIn *uint64 `json:"total-assets-opted-in,omitempty"`
		TotalCreatedApps   *uint64 `json:"total-created-apps,omitempty"`
		TotalCreatedAssets *uint64 `json:"total-created-assets,omitempty"`
	} `json:"data,omitempty"`
	Message string `json:"message"`
}

func TestAccountMaxResultsLimit(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	///////////
	// Given // A block containing an address that has created 10 apps, deleted 5 apps, and created 10 assets,
	//       // deleted 5 assets, and another address that has opted into the 5 apps and 5 assets remaining
	///////////

	deletedAppIDs := []uint64{1, 2, 3, 4, 5}
	deletedAssetIDs := []uint64{6, 7, 8, 9, 10}
	expectedAppIDs := []uint64{11, 12, 13, 14, 15}
	expectedAssetIDs := []uint64{16, 17, 18, 19, 20}

	var txns []sdk.SignedTxnWithAD
	// make apps and assets
	for range deletedAppIDs {
		txns = append(txns, test.MakeCreateAppTxn(sdk.Address(test.AccountA)))
	}
	for _, id := range deletedAssetIDs {
		txns = append(txns, test.MakeAssetConfigTxn(0, 100, 0, false, "UNIT",
			fmt.Sprintf("Asset %d", id), "http://asset.com", sdk.Address(test.AccountA)))
	}
	for range expectedAppIDs {
		txns = append(txns, test.MakeCreateAppTxn(sdk.Address(test.AccountA)))
	}
	for _, id := range expectedAssetIDs {
		txns = append(txns, test.MakeAssetConfigTxn(0, 100, 0, false, "UNIT",
			fmt.Sprintf("Asset %d", id), "http://asset.com", sdk.Address(test.AccountA)))
	}
	// delete some apps and assets
	for _, id := range deletedAppIDs {
		txns = append(txns, test.MakeAppDestroyTxn(id, sdk.Address(test.AccountA)))
	}
	for _, id := range deletedAssetIDs {
		txns = append(txns, test.MakeAssetDestroyTxn(id, sdk.Address(test.AccountA)))
	}

	// opt in to the remaining ones
	for _, id := range expectedAppIDs {
		txns = append(txns, test.MakeAppOptInTxn(id, sdk.Address(test.AccountA)))
		txns = append(txns, test.MakeAppOptInTxn(id, sdk.Address(test.AccountB)))
	}
	for _, id := range expectedAssetIDs {
		txns = append(txns, test.MakeAssetOptInTxn(id, sdk.Address(test.AccountA)))
		txns = append(txns, test.MakeAssetOptInTxn(id, sdk.Address(test.AccountB)))
	}

	ptxns := make([]*sdk.SignedTxnWithAD, len(txns))
	for i := range txns {
		ptxns[i] = &txns[i]
	}

	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/AccountMaxResultsLimit.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	//////////
	// When // We look up the address using a ServerImplementation with a maxAccountsAPIResults limit set,
	//      // and addresses with max # apps over & under the limit
	//////////

	maxResults := 14
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()
	opts := defaultOpts
	opts.MaxAPIResourcesPerAccount = uint64(maxResults)
	listenAddr := "localhost:8989"
	go Serve(serverCtx, listenAddr, db, nil, logrus.New(), opts)

	waitForServer(t, listenAddr)

	// make a real HTTP request (to additionally test generated param parsing logic)
	makeReq := func(t *testing.T, path string, exclude []string, includeDeleted bool, next *string, limit *uint64) (*http.Response, []byte) {
		var query []string
		if len(exclude) > 0 {
			query = append(query, "exclude="+strings.Join(exclude, ","))
		}
		if includeDeleted {
			query = append(query, "include-all=true")
		}
		if next != nil {
			query = append(query, "next="+*next)
		}
		if limit != nil {
			query = append(query, fmt.Sprintf("limit=%d", *limit))
		}
		if len(query) > 0 {
			path += "?" + strings.Join(query, "&")
		}
		t.Log("making HTTP request path", path)
		resp, err := http.Get("http://" + listenAddr + path)
		require.NoError(t, err)
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp, body
	}

	//////////
	// Then // The limit is enforced, leading to a 400 error
	//////////

	checkExclude := func(t *testing.T, acct generated.Account, exclude []string) {
		for _, exc := range exclude {
			switch exc {
			case "all":
				assert.Nil(t, acct.CreatedApps)
				assert.Nil(t, acct.AppsLocalState)
				assert.Nil(t, acct.CreatedAssets)
				assert.Nil(t, acct.Assets)
			case "created-assets":
				assert.Nil(t, acct.CreatedAssets)
			case "apps-local-state":
				assert.Nil(t, acct.AppsLocalState)
			case "created-apps":
				assert.Nil(t, acct.CreatedApps)
			case "assets":
				assert.Nil(t, acct.Assets)
			}
		}
	}

	testCases := []struct {
		address        sdk.Address
		exclude        []string
		includeDeleted bool
		errStatus      int
	}{
		{address: sdk.Address(test.AccountA), exclude: []string{}, errStatus: http.StatusBadRequest},
		{address: sdk.Address(test.AccountA), exclude: []string{"all"}},
		{address: sdk.Address(test.AccountA), exclude: []string{"created-assets", "created-apps", "apps-local-state", "assets"}},
		{address: sdk.Address(test.AccountA), exclude: []string{"assets", "created-apps"}},
		{address: sdk.Address(test.AccountA), exclude: []string{"assets", "apps-local-state"}},
		{address: sdk.Address(test.AccountA), exclude: []string{"assets", "apps-local-state"}, includeDeleted: true, errStatus: http.StatusBadRequest},
		{address: sdk.Address(test.AccountB), exclude: []string{"created-assets", "apps-local-state"}},
		{address: sdk.Address(test.AccountB), exclude: []string{"assets", "apps-local-state"}},
		{address: sdk.Address(test.AccountA), exclude: []string{"created-assets"}, errStatus: http.StatusBadRequest},
		{address: sdk.Address(test.AccountA), exclude: []string{"created-apps"}, errStatus: http.StatusBadRequest},
		{address: sdk.Address(test.AccountA), exclude: []string{"apps-local-state"}, errStatus: http.StatusBadRequest},
		{address: sdk.Address(test.AccountA), exclude: []string{"assets"}, errStatus: http.StatusBadRequest},
	}

	for _, tc := range testCases {
		maxResults := 14
		t.Run(fmt.Sprintf("LookupAccountByID exclude %v", tc.exclude), func(t *testing.T) {
			path := "/v2/accounts/" + tc.address.String()
			resp, data := makeReq(t, path, tc.exclude, tc.includeDeleted, nil, nil)
			if tc.errStatus != 0 { // was a 400 error expected? check error response
				require.Equal(t, tc.errStatus, resp.StatusCode)
				var response accountsErrorResponse
				err = json.Decode(data, &response)
				require.NoError(t, err)
				assert.Equal(t, tc.address.String(), *response.Data.Address)
				assert.Equal(t, uint64(maxResults), *response.Data.MaxResults)
				if tc.includeDeleted {
					assert.Equal(t, uint64(10), *response.Data.TotalCreatedApps)
					assert.Equal(t, uint64(10), *response.Data.TotalCreatedAssets)
				} else {
					assert.Equal(t, uint64(5), *response.Data.TotalAppsOptedIn)
					assert.Equal(t, uint64(5), *response.Data.TotalAssetsOptedIn)
					assert.Equal(t, uint64(5), *response.Data.TotalCreatedApps)
					assert.Equal(t, uint64(5), *response.Data.TotalCreatedAssets)
				}
				return
			}
			require.Equal(t, http.StatusOK, resp.StatusCode, fmt.Sprintf("unexpected return code, body: %s", string(data)))
			var response generated.AccountResponse
			err = json.Decode(data, &response)
			require.NoError(t, err)
			checkExclude(t, response.Account, tc.exclude)
		})
	}

	//////////
	// When // We search all addresses using a ServerImplementation with a maxAccountsAPIResults limit set,
	//      // and one of those addresses is over the limit, but another address is not
	//////////

	for _, tc := range []struct {
		exclude    []string
		errStatus  int
		errAddress sdk.Address
	}{
		{exclude: []string{"all"}},
		{exclude: []string{"created-assets", "created-apps", "apps-local-state", "assets"}},
		{exclude: []string{"assets", "apps-local-state"}},
		{errAddress: sdk.Address(test.AccountA), exclude: nil, errStatus: 400},
		{errAddress: sdk.Address(test.AccountA), exclude: []string{"created-assets"}, errStatus: http.StatusBadRequest},
		{errAddress: sdk.Address(test.AccountA), exclude: []string{"created-apps"}, errStatus: http.StatusBadRequest},
		{errAddress: sdk.Address(test.AccountA), exclude: []string{"apps-local-state"}, errStatus: http.StatusBadRequest},
		{errAddress: sdk.Address(test.AccountA), exclude: []string{"assets"}, errStatus: http.StatusBadRequest},
	} {
		t.Run(fmt.Sprintf("SearchForAccounts exclude %v", tc.exclude), func(t *testing.T) {
			maxResults := 14
			resp, data := makeReq(t, "/v2/accounts", tc.exclude, false, nil, nil)
			if tc.errStatus != 0 { // was a 400 error expected? check error response
				require.Equal(t, tc.errStatus, resp.StatusCode)
				var response accountsErrorResponse
				err = json.Decode(data, &response)
				require.NoError(t, err)
				require.Equal(t, *response.Data.Address, tc.errAddress.String())
				require.Equal(t, *response.Data.MaxResults, uint64(maxResults))
				require.Equal(t, *response.Data.TotalAppsOptedIn, uint64(5))
				require.Equal(t, *response.Data.TotalCreatedApps, uint64(5))
				require.Equal(t, *response.Data.TotalAssetsOptedIn, uint64(5))
				require.Equal(t, *response.Data.TotalCreatedAssets, uint64(5))
				return
			}
			require.Equal(t, http.StatusOK, resp.StatusCode, fmt.Sprintf("unexpected return code, body: %s", string(data)))
			var response generated.AccountsResponse
			err = json.Decode(data, &response)
			require.NoError(t, err)

			// check that the accounts are in there
			var sawAccountA, sawAccountB bool
			for _, acct := range response.Accounts {
				switch acct.Address {
				case test.AccountA.String():
					sawAccountA = true
					require.Equal(t, acct.TotalAppsOptedIn, uint64(5))
					require.Equal(t, acct.TotalCreatedApps, uint64(5))
					require.Equal(t, acct.TotalAssetsOptedIn, uint64(5))
					require.Equal(t, acct.TotalCreatedAssets, uint64(5))
				case test.AccountB.String():
					sawAccountB = true
					require.Equal(t, acct.TotalAppsOptedIn, uint64(5))
					require.Equal(t, acct.TotalCreatedApps, uint64(0))
					require.Equal(t, acct.TotalAssetsOptedIn, uint64(5))
					require.Equal(t, acct.TotalCreatedAssets, uint64(0))
				}
				checkExclude(t, acct, tc.exclude)
			}
			require.True(t, sawAccountA && sawAccountB)
		})
	}

	//////////
	// When // We look up the assets an account holds, and paginate through them using "Next"
	//////////

	t.Run("LookupAccountAssets", func(t *testing.T) {
		var next *string   // nil/unset to start
		limit := uint64(2) // 2 at a time
		var assets []generated.AssetHolding
		for {
			resp, data := makeReq(t, "/v2/accounts/"+test.AccountB.String()+"/assets", nil, false, next, &limit)
			require.Equal(t, http.StatusOK, resp.StatusCode, fmt.Sprintf("unexpected return code, body: %s", string(data)))
			var response generated.AssetHoldingsResponse
			err = json.Decode(data, &response)
			require.NoError(t, err)
			if len(response.Assets) == 0 {
				require.Nil(t, response.NextToken)
				break
			}
			require.NotEmpty(t, response.Assets)
			assets = append(assets, response.Assets...)
			next = response.NextToken // paginate
		}
		//////////
		// Then // We can see all the assets, even though there were more than the limit
		//////////
		require.Len(t, assets, 5)
		for i, asset := range assets {
			require.Equal(t, expectedAssetIDs[i], asset.AssetId)
		}
	})

	//////////
	// When // We look up the assets an account has created, and paginate through them using "Next"
	//////////

	t.Run("LookupAccountCreatedAssets", func(t *testing.T) {
		var next *string   // nil/unset to start
		limit := uint64(2) // 2 at a time
		var assets []generated.Asset
		for {
			resp, data := makeReq(t, "/v2/accounts/"+test.AccountA.String()+"/created-assets", nil, false, next, &limit)
			require.Equal(t, http.StatusOK, resp.StatusCode, fmt.Sprintf("unexpected return code, body: %s", string(data)))
			var response generated.AssetsResponse
			err = json.Decode(data, &response)
			require.NoError(t, err)
			if len(response.Assets) == 0 {
				require.Nil(t, response.NextToken)
				break
			}
			require.NotEmpty(t, response.Assets)
			assets = append(assets, response.Assets...)
			next = response.NextToken // paginate
		}
		//////////
		// Then // We can see all the assets, even though there were more than the limit
		//////////
		require.Len(t, assets, 5)
		for i, asset := range assets {
			require.Equal(t, expectedAssetIDs[i], asset.Index)
		}
	})

	//////////
	// When // We look up the apps an account has opted in to, and paginate through them using "Next"
	//////////

	t.Run("LookupAccountAppLocalStates", func(t *testing.T) {
		var next *string   // nil/unset to start
		limit := uint64(2) // 2 at a time
		var apps []generated.ApplicationLocalState
		for {
			resp, data := makeReq(t, "/v2/accounts/"+test.AccountA.String()+"/apps-local-state", nil, false, next, &limit)
			require.Equal(t, http.StatusOK, resp.StatusCode, fmt.Sprintf("unexpected return code, body: %s", string(data)))
			var response generated.ApplicationLocalStatesResponse
			err = json.Decode(data, &response)
			require.NoError(t, err)
			if len(response.AppsLocalStates) == 0 {
				require.Nil(t, response.NextToken)
				break
			}
			require.NotEmpty(t, response.AppsLocalStates)
			apps = append(apps, response.AppsLocalStates...)
			next = response.NextToken // paginate
		}
		//////////
		// Then // We can see all the apps, even though there were more than the limit
		//////////
		require.Len(t, apps, 5)
		for i, app := range apps {
			require.Equal(t, expectedAppIDs[i], app.Id)
		}
	})

	//////////
	// When // We look up the apps an account has opted in to, and paginate through them using "Next"
	//////////

	t.Run("LookupAccountCreatedApplications", func(t *testing.T) {
		var next *string   // nil/unset to start
		limit := uint64(2) // 2 at a time
		var apps []generated.Application
		for {
			resp, data := makeReq(t, "/v2/accounts/"+test.AccountA.String()+"/created-applications", nil, false, next, &limit)
			require.Equal(t, http.StatusOK, resp.StatusCode, fmt.Sprintf("unexpected return code, body: %s", string(data)))
			var response generated.ApplicationsResponse
			err = json.Decode(data, &response)
			require.NoError(t, err)
			if len(response.Applications) == 0 {
				require.Nil(t, response.NextToken)
				break
			}
			require.NotEmpty(t, response.Applications)
			apps = append(apps, response.Applications...)
			next = response.NextToken // paginate
		}
		//////////
		// Then // We can see all the apps, even though there were more than the limit
		//////////
		require.Len(t, apps, 5)
		for i, app := range apps {
			require.Equal(t, expectedAppIDs[i], app.Id)
		}
	})
}

func TestBlockNotFound(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
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

	api := testServerImplementation(db)
	err := api.LookupBlock(c, 100, generated.LookupBlockParams{})
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
	var appAddr sdk.Address
	appAddr[1] = 99
	appAddrStr := appAddr.String()

	pay := generated.SearchForTransactionsParamsTxType("pay")
	axfer := generated.SearchForTransactionsParamsTxType("axfer")
	appl := generated.SearchForTransactionsParamsTxType("appl")
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

	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	///////////
	// Given // a DB with some inner txns in it.
	///////////
	// appCall := test.MakeAppCallWithInnerTxn(test.AccountA, appAddr, test.AccountB, appAddr, test.AccountC)
	//expectedID := appCall.Txn.ID().String()

	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/InnerTxn.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	stxn, _, err := util.DecodeSignedTxn(vb.Block.BlockHeader, vb.Block.Payset[0])
	require.NoError(t, err)
	expectedID := crypto.TransactionIDString(stxn.Txn)

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

			api := testServerImplementation(db)
			err = api.SearchForTransactions(c, tc.filter)
			require.NoError(t, err)

			//////////
			// Then // The only result is the root transaction.
			//////////
			require.Equal(t, http.StatusOK, rec.Code)
			var response generated.TransactionsResponse
			err = json.Decode(rec.Body.Bytes(), &response)
			require.NoError(t, err)

			require.Len(t, response.Transactions, 1)
			require.Equal(t, expectedID, *(response.Transactions[0].Id))
		})
	}
}

// TestPagingRootTxnDeduplication checks that paging in the middle of an inner
// transaction group does not allow the root transaction to be returned on both
// pages.
func TestPagingRootTxnDeduplication(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	///////////
	// Given // a DB with some inner txns in it.
	///////////
	var appAddr sdk.Address
	appAddr[1] = 99
	appAddrStr := appAddr.String()

	//appCall := test.MakeAppCallWithInnerTxn(test.AccountA, appAddr, test.AccountB, appAddr, test.AccountC)
	//expectedID := appCall.Txn.ID().String()

	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/PagingRootTxnDeduplication.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	stxn, _, err := util.DecodeSignedTxn(vb.Block.BlockHeader, vb.Block.Payset[0])
	require.NoError(t, err)
	expectedID := crypto.TransactionIDString(stxn.Txn)

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
			api := testServerImplementation(db)
			err = api.SearchForTransactions(c, tc.params)
			require.NoError(t, err)

			require.Equal(t, http.StatusOK, rec1.Code)
			var response generated.TransactionsResponse
			err = json.Decode(rec1.Body.Bytes(), &response)
			require.NoError(t, err)
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
			err = json.Decode(rec2.Body.Bytes(), &response2)
			require.NoError(t, err)

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
		api := testServerImplementation(db)
		err = api.LookupBlock(c, uint64(vb.Block.Round), generated.LookupBlockParams{})
		require.NoError(t, err)

		//////////
		// Then // There should be a single transaction which has inner transactions
		//////////
		var response generated.BlockResponse
		require.Equal(t, http.StatusOK, rec.Code)
		err = json.Decode(rec.Body.Bytes(), &response)
		require.NoError(t, err)

		require.NotNil(t, response.Transactions)
		require.Len(t, *response.Transactions, 1)
		require.NotNil(t, (*response.Transactions)[0])
		require.Len(t, *(*response.Transactions)[0].InnerTxns, 2)
	})
}

func TestKeyregTransactionWithStateProofKeys(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	///////////
	// Given // A block containing a key reg txn with state proof key
	///////////
	var votePK [32]byte
	votePK[0] = 1

	var selectionPK [32]byte
	selectionPK[0] = 1

	var stateProofPK [64]byte
	stateProofPK[0] = 1

	//txn := transactions.SignedTxnWithAD{
	//	SignedTxn: transactions.SignedTxn{
	//		Txn: transactions.Transaction{
	//			Type: "keyreg",
	//			Header: transactions.Header{
	//				Sender:      test.AccountA,
	//				GenesisHash: test.GenesisHash,
	//			},
	//			KeyregTxnFields: transactions.KeyregTxnFields{
	//				VotePK:           votePK,
	//				SelectionPK:      selectionPK,
	//				StateProofPK:     stateProofPK,
	//				VoteFirst:        basics.Round(0),
	//				VoteLast:         basics.Round(100),
	//				VoteKeyDilution:  1000,
	//				Nonparticipation: false,
	//			},
	//		},
	//		Sig: test.Signature,
	//	},
	//}
	//

	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/KeyregTransactionWithStateProofKeys.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	txn, _, err := util.DecodeSignedTxn(vb.Block.BlockHeader, vb.Block.Payset[0])
	require.NoError(t, err)

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
		err = api.LookupTransaction(c, crypto.TransactionIDString(txn.Txn))
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
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()
	api := testServerImplementation(db)

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
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	///////////
	// Given // a DB with some inner txns in it.
	///////////
	//var createAddr basics.Address
	//createAddr[1] = 99
	//createAddrStr := createAddr.String()

	assetName := "valid"
	//url := "https://my.embedded.\000.null.asset"
	urlBytes, _ := base64.StdEncoding.DecodeString("8J+qmSBNb25leSwgd2FudAo=")
	url := string(urlBytes)
	unitName := "asset\rwith\nnon-printable\tcharacters"
	//createAsset := test.MakeAssetConfigTxn(0, 100, 0, false, unitName, assetName, url, test.AccountA)

	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/AccountClearsNonUTF8.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

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

		api := testServerImplementation(db)
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

		api := testServerImplementation(db)
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
	var appAddr sdk.Address
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

	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	///////////
	// Given // a DB with some inner txns in it.
	///////////
	//appCall := test.MakeAppCallWithInnerAppCall(test.AccountA)

	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/LookupInnerLogs.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	appCall, _, err := util.DecodeSignedTxn(vb.Block.BlockHeader, vb.Block.Payset[0])

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

			api := testServerImplementation(db)
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
			require.Equal(t, crypto.TransactionIDString(appCall.Txn), ld[0].Txid)
			require.Equal(t, len(tc.logs), len(ld[0].Logs))
			for i, log := range ld[0].Logs {
				require.Equal(t, []byte(tc.logs[i]), log)
			}
		})
	}
}

// TestLookupInnerLogs runs queries for logs given application ids,
// and checks that logs in inner transactions match properly.
func TestLookupMultiInnerLogs(t *testing.T) {
	var appAddr sdk.Address
	appAddr[1] = 99

	params := generated.LookupApplicationLogsByIDParams{}

	testcases := []struct {
		name            string
		appID           uint64
		numTxnsWithLogs int
		logs            []string
	}{
		{
			name:            "match on root with appId 123",
			appID:           123,
			numTxnsWithLogs: 1,
			logs: []string{
				"testing outer appl log",
				"appId 123 log",
			},
		},
		{
			name:            "match on inner with appId 789",
			appID:           789,
			numTxnsWithLogs: 1,
			logs: []string{
				"testing inner log",
				"appId 789 log",
			},
		},
		{
			name:            "match on inner with appId 222",
			appID:           222,
			numTxnsWithLogs: 3, // There are 6 logs over 3 transactions
			logs: []string{
				"testing multiple logs 1",
				"appId 222 log 1",
				"testing multiple logs 2",
				"appId 222 log 2",
				"testing multiple logs 3",
				"appId 222 log 3",
			},
		},
	}

	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	///////////
	// Given // a DB with some inner txns in it.
	///////////
	//appCall := test.MakeAppCallWithMultiLogs(test.AccountA)

	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/LookupMultiInnerLogs.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	appCall, _, err := util.DecodeSignedTxn(vb.Block.BlockHeader, vb.Block.Payset[0])

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
			require.Equal(t, tc.numTxnsWithLogs, len(ld))

			logCount := 0
			for txnIndex, result := range ld {
				require.Equal(t, crypto.TransactionIDString(appCall.Txn), result.Txid)
				for logIndex, log := range result.Logs {
					require.Equal(t, []byte(tc.logs[txnIndex*2+logIndex]), log)
					logCount++
				}
			}
			require.Equal(t, logCount, len(tc.logs))
		})
	}
}

func TestFetchBlockWithExpiredPartAccts(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	///////////
	// Given // a DB with a block containing expired participation accounts.
	///////////
	//appCreate := test.MakeCreateAppTxn(test.AccountA)
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/FetchBlockWithExpiredPartAccts.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	////////////
	// When // We query for a block containing expired participation accounts.
	////////////
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/v2/blocks/:round-number")
	c.SetParamNames("round-number")
	c.SetParamValues(strconv.Itoa(1))

	api := testServerImplementation(db)
	err = api.LookupBlock(c, 1, generated.LookupBlockParams{})
	require.NoError(t, err)

	////////////
	// Then // A 200 gets returned and expired participation accounts match the expected accounts.
	////////////
	require.Equal(t, http.StatusOK, rec.Code)
	var response generated.Block
	err = json.Decode(rec.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.NotNil(t, response.ParticipationUpdates)
	expiredPartAccts := *response.ParticipationUpdates.ExpiredParticipationAccounts
	assert.Equal(t, test.AccountB.String(), expiredPartAccts[0])
	assert.Equal(t, test.AccountC.String(), expiredPartAccts[1])
}

func TestFetchBlockWithOptions(t *testing.T) {

	testCases := []struct {
		name           string
		headerOnly     bool
		expectedTxnLen int
	}{
		{
			name:           "default",
			headerOnly:     false,
			expectedTxnLen: 3,
		},
		{
			name:           "header-only=false",
			headerOnly:     false,
			expectedTxnLen: 3,
		},
		{
			name:           "header-only=true",
			headerOnly:     true,
			expectedTxnLen: 0,
		},
	}

	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	///////////
	// Given // a DB with a block with a txn in it.
	///////////
	//txnA := test.MakeCreateAppTxn(test.AccountA)
	//txnB := test.MakeCreateAppTxn(test.AccountB)
	//txnC := test.MakeCreateAppTxn(test.AccountC)
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/FetchBlockWithOptions.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	//////////
	// When // We look up a block.
	//////////

	setupReq := func(path, paramName, paramValue string) (echo.Context, *ServerImplementation, *httptest.ResponseRecorder) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath(path)
		c.SetParamNames(paramName)
		c.SetParamValues(paramValue)
		api := testServerImplementation(db)
		return c, api, rec
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf(tc.name), func(t *testing.T) {
			c, api, rec := setupReq("/v2/blocks/:round-number", "round-number", "1")
			if tc.name == "default" {
				err = api.LookupBlock(c, 1, generated.LookupBlockParams{})
			} else {
				err = api.LookupBlock(c, 1, generated.LookupBlockParams{HeaderOnly: &tc.headerOnly})
			}
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, rec.Code)
			var response generated.Block
			err = json.Decode(rec.Body.Bytes(), &response)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedTxnLen, len(*response.Transactions))
		})
	}

}

func TestGetBlocksTransactionsLimit(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	///////////
	// Given // A block containing 2 transactions, a block containing 10 transactions
	//       // and a block containing 20 transactions
	///////////

	//ntxns := []int{2, 10, 20}
	//for i, n := range ntxns {
	//	var txns []transactions.SignedTxnWithAD
	//	for j := 0; j < n; j++ {
	//		txns = append(txns, test.MakePaymentTxn(1, 100, 0, 0, 0, 0, test.AccountA, test.AccountB, basics.Address{}, basics.Address{}))
	//	}
	//	ptxns := make([]*transactions.SignedTxnWithAD, n)
	//	for k := range txns {
	//		ptxns[k] = &txns[k]
	//	}
	//	block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, ptxns...)
	//	block.BlockHeader.Round = basics.Round(i + 1)
	//	require.NoError(t, err)
	//
	//	err = proc(&rpcs.EncodedBlockCert{Block: block})
	//	require.NoError(t, err)
	//}

	for _, n := range []int{2, 10, 20} {
		vb, err := test.ReadValidatedBlockFromFile(fmt.Sprintf("test_resources/validated_blocks/GetBlocksTransactionsLimit%d.vb", n))
		require.NoError(t, err)
		err = db.AddBlock(&vb)
		require.NoError(t, err)
	}

	//////////
	// When // We look up a block using a ServerImplementation with a maxTransactionsLimit set,
	//      // and blocks with # of transactions over & under the limit
	//////////

	maxTxns := 10
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()
	opts := defaultOpts
	opts.MaxTransactionsLimit = uint64(maxTxns)
	listenAddr := "localhost:8888"
	go Serve(serverCtx, listenAddr, db, nil, logrus.New(), opts)

	waitForServer(t, listenAddr)

	// make a real HTTP request (to additionally test generated param parsing logic)
	makeReq := func(t *testing.T, path string, headerOnly bool) (*http.Response, []byte) {
		if headerOnly {
			path += "?header-only=true"
		}

		t.Log("making HTTP request path", path)
		resp, err := http.Get("http://" + listenAddr + path)
		require.NoError(t, err)
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp, body
	}

	//////////
	// Then // The limit is enforced, leading to a 400 error
	//////////

	testCases := []struct {
		name       string
		round      string
		headerOnly bool
		ntxns      int
		errStatus  int
	}{
		{name: "block round 1 default", round: "1", headerOnly: false, ntxns: 2},
		{name: "block round 1  with header-only set", round: "1", headerOnly: true, ntxns: 0},

		{name: "block round 2 default", round: "2", headerOnly: false, ntxns: 10},
		{name: "block round 2 with header-only set", round: "2", headerOnly: true, ntxns: 0},

		{name: "block round 3 default", round: "3", headerOnly: false, errStatus: 400},
		{name: "block round 3 with header-only set", round: "3", headerOnly: true, ntxns: 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path := "/v2/blocks/" + tc.round
			resp, data := makeReq(t, path, tc.headerOnly)
			if tc.errStatus != 0 {
				require.Equal(t, tc.errStatus, resp.StatusCode)
				require.Contains(t, string(data), errTransactionsLimitReached)
				return
			}
			require.Equal(t, http.StatusOK, resp.StatusCode, fmt.Sprintf("unexpected return code, body: %s", string(data)))
			var response generated.BlockResponse
			err := json.Decode(data, &response)
			require.NoError(t, err)
			require.Equal(t, tc.ntxns, len(*response.Transactions))
		})
	}
}

func TestGetBlockWithCompression(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	///////////
	// Given // A block containing 20 transactions at round 1
	//       //
	///////////

	const numbOfTxns = 20
	//var txns []transactions.SignedTxnWithAD
	//for j := 0; j < numbOfTxns; j++ {
	//	txns = append(txns, test.MakePaymentTxn(1, 100, 0, 0, 0, 0, test.AccountA, test.AccountB, basics.Address{}, basics.Address{}))
	//}
	//ptxns := make([]*transactions.SignedTxnWithAD, numbOfTxns)
	//for k := range txns {
	//	ptxns[k] = &txns[k]
	//}
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/GetBlockWithCompression.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	//////////
	// When // We look up a block using a ServerImplementation with a compression flag on/off
	//////////

	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()
	opts := defaultOpts
	listenAddr := "localhost:8889"
	go Serve(serverCtx, listenAddr, db, nil, logrus.New(), opts)

	waitForServer(t, listenAddr)

	getBlockFunc := func(t *testing.T, headerOnly bool, useCompression bool) *generated.BlockResponse {
		path := "/v2/blocks/1"

		client := &http.Client{}
		req, err := http.NewRequest("GET", "http://"+listenAddr+path, nil)
		require.NoError(t, err)
		q := req.URL.Query()
		if headerOnly {
			q.Add("header-only", "true")
		}
		if useCompression {
			req.Header.Add(echo.HeaderAcceptEncoding, "gzip")
		}
		req.URL.RawQuery = q.Encode()
		t.Log("making HTTP request path", req.URL)
		resp, err := client.Do(req)
		require.NoError(t, err)

		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode, fmt.Sprintf("unexpected return code, body: %s", string(body)))

		var response generated.BlockResponse
		if useCompression {
			require.Equal(t, resp.Header.Get(echo.HeaderContentEncoding), "gzip")
			reader, err := gzip.NewReader(bytes.NewReader(body))
			require.NoError(t, err)

			output, e2 := ioutil.ReadAll(reader)
			require.NoError(t, e2)

			body = output
		}
		err = json.Decode(body, &response)
		require.NoError(t, err)

		return &response
	}

	//////////
	// Then // Get the same block content compared to uncompress block
	//////////
	notCompressedBlock := getBlockFunc(t, false, false)
	compressedBlock := getBlockFunc(t, false, true)
	require.Equal(t, notCompressedBlock, compressedBlock)
	require.Equal(t, len(*notCompressedBlock.Transactions), numbOfTxns)

	// we now make sure that compression flag works with other flags.
	notCompressedBlock = getBlockFunc(t, true, false)
	compressedBlock = getBlockFunc(t, true, true)
	require.Equal(t, len(*notCompressedBlock.Transactions), 0)
}

func TestNoCompressionSupportForNonBlockAPI(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	//////////
	// When // we call the health endpoint using compression flag on
	//////////

	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()
	opts := defaultOpts
	listenAddr := "localhost:8887"
	go Serve(serverCtx, listenAddr, db, nil, logrus.New(), opts)

	waitForServer(t, listenAddr)

	path := "/health"
	client := &http.Client{}
	req, err := http.NewRequest("GET", "http://"+listenAddr+path, nil)
	require.NoError(t, err)
	req.Header.Add(echo.HeaderAcceptEncoding, "gzip")

	t.Log("making HTTP request path", req.URL)

	resp, err := client.Do(req)
	require.NoError(t, err)

	//////////
	// Then // We expect the result not to be compressed.
	//////////

	require.Equal(t, resp.Header.Get(echo.HeaderContentEncoding), "")
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, fmt.Sprintf("unexpected return code, body: %s", string(body)))
	var response generated.HealthCheckResponse
	err = json.Decode(body, &response)
	require.NoError(t, err)
}

func waitForServer(t *testing.T, listenAddr string) {
	// wait at most a few seconds for server to come up
	serverUp := false
	for maxWait := 3 * time.Second; !serverUp && maxWait > 0; maxWait -= 50 * time.Millisecond {
		time.Sleep(50 * time.Millisecond)
		resp, err := http.Get("http://" + listenAddr + "/health")
		if err != nil {
			t.Log("waiting for server:", err)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Log("waiting for server OK:", resp.StatusCode)
			continue
		}
		serverUp = true // server is up now
	}
	require.True(t, serverUp, "api.Serve did not start server in time")
}

// compareAppBoxesAgainstHandler is of type BoxTestComparator
func compareAppBoxesAgainstHandler(t *testing.T, db *postgres.IndexerDb,
	appBoxes map[sdk.AppIndex]map[string]string, deletedBoxes map[sdk.AppIndex]map[string]bool, verifyTotals bool) {

	setupRequest := func(path, paramName, paramValue string) (echo.Context, *ServerImplementation, *httptest.ResponseRecorder) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath(path)
		c.SetParamNames(paramName)
		c.SetParamValues(paramValue)
		api := testServerImplementation(db)
		return c, api, rec
	}

	remainingBoxes := map[sdk.AppIndex]map[string]string{}
	numRequests := 0
	sumOfBoxes := 0
	sumOfBoxBytes := 0

	caseNum := 1
	var totalBoxes, totalBoxBytes int
	for appIdx, boxes := range appBoxes {
		totalBoxes = 0
		totalBoxBytes = 0

		remainingBoxes[appIdx] = map[string]string{}

		// compare expected against handler response one box at a time
		for key, expectedValue := range boxes {
			msg := fmt.Sprintf("caseNum=%d, appIdx=%d, key=%#v", caseNum, appIdx, key)
			expectedAppIdx, boxName, err := apps.SplitBoxKey(key)
			require.NoError(t, err, msg)
			require.Equal(t, uint64(appIdx), expectedAppIdx, msg)
			numRequests++

			boxDeleted := false
			if deletedBoxes != nil {
				if _, ok := deletedBoxes[appIdx][key]; ok {
					boxDeleted = true
				}
			}

			c, api, rec := setupRequest("/v2/applications/:appidx/box/", "appidx", strconv.Itoa(int(appIdx)))
			prefixedName := fmt.Sprintf("str:%s", boxName)
			params := generated.LookupApplicationBoxByIDAndNameParams{Name: prefixedName}
			err = api.LookupApplicationBoxByIDAndName(c, uint64(appIdx), params)
			require.NoError(t, err, msg)
			require.Equal(t, http.StatusOK, rec.Code, fmt.Sprintf("msg: %s. unexpected return code, body: %s", msg, rec.Body.String()))

			var resp generated.BoxResponse
			data := rec.Body.Bytes()
			err = json.Decode(data, &resp)

			if !boxDeleted {
				require.NoError(t, err, msg, msg)
				require.Equal(t, boxName, string(resp.Name), msg)
				require.Equal(t, expectedValue, string(resp.Value), msg)

				remainingBoxes[appIdx][boxName] = expectedValue

				totalBoxes++
				totalBoxBytes += len(boxName) + len(expectedValue)
			} else {
				require.ErrorContains(t, err, "no rows in result set", msg)
			}
		}

		msg := fmt.Sprintf("caseNum=%d, appIdx=%d", caseNum, appIdx)

		expectedBoxes := remainingBoxes[appIdx]

		c, api, rec := setupRequest("/v2/applications/:appidx/boxes", "appidx", strconv.Itoa(int(appIdx)))
		params := generated.SearchForApplicationBoxesParams{}

		err := api.SearchForApplicationBoxes(c, uint64(appIdx), params)
		require.NoError(t, err, msg)
		require.Equal(t, http.StatusOK, rec.Code, fmt.Sprintf("msg: %s. unexpected return code, body: %s", msg, rec.Body.String()))

		var resp generated.BoxesResponse
		data := rec.Body.Bytes()
		err = json.Decode(data, &resp)
		require.NoError(t, err, msg)

		require.Equal(t, uint64(appIdx), uint64(resp.ApplicationId), msg)

		boxes := resp.Boxes
		require.NotNil(t, boxes, msg)
		require.Len(t, boxes, len(expectedBoxes), msg)
		for _, box := range boxes {
			require.Contains(t, expectedBoxes, string(box.Name), msg)
		}

		if verifyTotals {
			// compare expected totals against handler account_data JSON fields
			msg := fmt.Sprintf("caseNum=%d, appIdx=%d", caseNum, appIdx)

			appAddr := crypto.GetApplicationAddress(uint64(appIdx)).String()
			c, api, rec = setupRequest("/v2/accounts/:addr", "addr", appAddr)
			fmt.Printf("appIdx=%d\nappAddr=%s\npath=/v2/accounts/%s\n", appIdx, appAddr, appAddr)
			tru := true
			params := generated.LookupAccountByIDParams{IncludeAll: &tru}
			err := api.LookupAccountByID(c, appAddr, params)
			require.NoError(t, err, msg)
			require.Equal(t, http.StatusOK, rec.Code, fmt.Sprintf("msg: %s. unexpected return code, body: %s", msg, rec.Body.String()))

			var resp generated.AccountResponse
			data := rec.Body.Bytes()
			err = json.Decode(data, &resp)

			require.NoError(t, err, msg)
			require.Equal(t, uint64(totalBoxes), resp.Account.TotalBoxes, msg)
			require.Equal(t, uint64(totalBoxBytes), resp.Account.TotalBoxBytes, msg)

			// sanity check of the account summary query vs. the direct box search query results:
			require.Equal(t, uint64(len(boxes)), resp.Account.TotalBoxes, msg)
		}

		sumOfBoxes += totalBoxes
		sumOfBoxBytes += totalBoxBytes
		caseNum++
	}

	fmt.Printf("compareAppBoxesAgainstHandler succeeded with %d requests, %d boxes and %d boxBytes\n", numRequests, sumOfBoxes, sumOfBoxBytes)
}

// test runner copy/pastad/tweaked in handlers_e2e_test.go and postgres_integration_test.go
func runBoxCreateMutateDelete(t *testing.T, comparator boxTestComparator) {
	start := time.Now()

	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	appid := sdk.AppIndex(1)

	// ---- ROUND 1: create and fund the box app  ---- //
	currentRound := sdk.Round(1)
	//
	//createTxn, err := test.MakeComplexCreateAppTxn(test.AccountA, test.BoxApprovalProgram, test.BoxClearProgram, 8)
	//require.NoError(t, err)
	//
	//payNewAppTxn := test.MakePaymentTxn(1000, 500000, 0, 0, 0, 0, test.AccountA, appid.Address(), basics.Address{},
	//	basics.Address{})

	vb1, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/BoxCreateMutateDelete1.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb1)
	require.NoError(t, err)

	opts := idb.ApplicationQuery{ApplicationID: uint64(appid)}

	rowsCh, round := db.Applications(context.Background(), opts)
	require.Equal(t, uint64(currentRound), round)

	row, ok := <-rowsCh
	require.True(t, ok)
	require.NoError(t, row.Error)
	require.NotNil(t, row.Application.CreatedAtRound)
	require.Equal(t, uint64(currentRound), *row.Application.CreatedAtRound)

	// ---- ROUND 2: create 8 boxes for appid == 1  ---- //
	currentRound = sdk.Round(2)

	boxNames := []string{
		"a great box",
		"another great box",
		"not so great box",
		"disappointing box",
		"don't box me in this way",
		"I will be assimilated",
		"I'm destined for deletion",
		"box #8",
	}

	expectedAppBoxes := map[sdk.AppIndex]map[string]string{}

	//expectedAppBoxes[appid] = map[string]string{}
	//newBoxValue := "\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"
	//boxTxns := make([]*transactions.SignedTxnWithAD, 0)
	//for _, boxName := range boxNames {
	//	expectedAppBoxes[appid][apps.MakeBoxKey(uint64(appid), boxName)] = newBoxValue
	//
	//	args := []string{"create", boxName}
	//	boxTxn := test.MakeAppCallTxnWithBoxes(uint64(appid), test.AccountA, args, []string{boxName})
	//	boxTxns = append(boxTxns, &boxTxn)
	//}

	expectedAppBoxes[appid] = map[string]string{}
	newBoxValue := "\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"
	for _, boxName := range boxNames {
		expectedAppBoxes[appid][apps.MakeBoxKey(uint64(appid), boxName)] = newBoxValue
	}

	vb2, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/BoxCreateMutateDelete2.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb2)
	require.NoError(t, err)
	_, round = db.Applications(context.Background(), opts)
	require.Equal(t, uint64(currentRound), round)

	comparator(t, db, expectedAppBoxes, nil, true)

	// ---- ROUND 3: populate the boxes appropriately  ---- //
	currentRound = sdk.Round(3)

	appBoxesToSet := map[string]string{
		"a great box":               "it's a wonderful box",
		"another great box":         "I'm wonderful too",
		"not so great box":          "bummer",
		"disappointing box":         "RUG PULL!!!!",
		"don't box me in this way":  "non box-conforming",
		"I will be assimilated":     "THE BORG",
		"I'm destined for deletion": "I'm still alive!!!",
		"box #8":                    "eight is beautiful",
	}

	//boxTxns = make([]*transactions.SignedTxnWithAD, 0)
	//expectedAppBoxes[appid] = make(map[string]string)
	//for boxName, valPrefix := range appBoxesToSet {
	//	args := []string{"set", boxName, valPrefix}
	//	boxTxn := test.MakeAppCallTxnWithBoxes(uint64(appid), test.AccountA, args, []string{boxName})
	//	boxTxns = append(boxTxns, &boxTxn)
	//
	//	key := apps.MakeBoxKey(uint64(appid), boxName)
	//	expectedAppBoxes[appid][key] = valPrefix + newBoxValue[len(valPrefix):]
	//}

	expectedAppBoxes[appid] = make(map[string]string)
	for boxName, valPrefix := range appBoxesToSet {
		key := apps.MakeBoxKey(uint64(appid), boxName)
		expectedAppBoxes[appid][key] = valPrefix + newBoxValue[len(valPrefix):]
	}
	vb3, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/BoxCreateMutateDelete3.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb3)
	require.NoError(t, err)

	_, round = db.Applications(context.Background(), opts)
	require.Equal(t, uint64(currentRound), round)

	comparator(t, db, expectedAppBoxes, nil, true)

	// ---- ROUND 4: delete the unhappy boxes  ---- //
	currentRound = sdk.Round(4)

	appBoxesToDelete := []string{
		"not so great box",
		"disappointing box",
		"I'm destined for deletion",
	}

	//boxTxns = make([]*transactions.SignedTxnWithAD, 0)
	//for _, boxName := range appBoxesToDelete {
	//	args := []string{"delete", boxName}
	//	boxTxn := test.MakeAppCallTxnWithBoxes(uint64(appid), test.AccountA, args, []string{boxName})
	//	boxTxns = append(boxTxns, &boxTxn)
	//
	//	key := apps.MakeBoxKey(uint64(appid), boxName)
	//	delete(expectedAppBoxes[appid], key)
	//}
	for _, boxName := range appBoxesToDelete {
		key := apps.MakeBoxKey(uint64(appid), boxName)
		delete(expectedAppBoxes[appid], key)
	}
	vb4, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/BoxCreateMutateDelete4.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb4)
	require.NoError(t, err)

	_, round = db.Applications(context.Background(), opts)
	require.Equal(t, uint64(currentRound), round)

	deletedBoxes := make(map[sdk.AppIndex]map[string]bool)
	deletedBoxes[appid] = make(map[string]bool)
	for _, deletedBox := range appBoxesToDelete {
		deletedBoxes[appid][deletedBox] = true
	}
	comparator(t, db, expectedAppBoxes, deletedBoxes, true)

	// ---- ROUND 5: create 3 new boxes, overwriting one of the former boxes  ---- //
	currentRound = sdk.Round(5)

	//boxTxns = make([]*transactions.SignedTxnWithAD, 0)
	//for _, boxName := range appBoxesToCreate {
	//	args := []string{"create", boxName}
	//	boxTxn := test.MakeAppCallTxnWithBoxes(uint64(appid), test.AccountA, args, []string{boxName})
	//	boxTxns = append(boxTxns, &boxTxn)
	//
	//	key := apps.MakeBoxKey(uint64(appid), boxName)
	//	expectedAppBoxes[appid][key] = newBoxValue
	//}
	appBoxesToCreate := []string{
		"fantabulous",
		"disappointing box", // overwriting here
		"AVM is the new EVM",
	}
	for _, boxName := range appBoxesToCreate {
		key := apps.MakeBoxKey(uint64(appid), boxName)
		expectedAppBoxes[appid][key] = newBoxValue
	}
	vb5, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/BoxCreateMutateDelete5.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb5)
	require.NoError(t, err)

	_, round = db.Applications(context.Background(), opts)
	require.Equal(t, uint64(currentRound), round)

	comparator(t, db, expectedAppBoxes, nil, true)

	// ---- ROUND 6: populate the 3 new boxes  ---- //
	currentRound = sdk.Round(6)

	//boxTxns = make([]*transactions.SignedTxnWithAD, 0)
	//for boxName, valPrefix := range appBoxesToSet {
	//	args := []string{"set", boxName, valPrefix}
	//	boxTxn := test.MakeAppCallTxnWithBoxes(uint64(appid), test.AccountA, args, []string{boxName})
	//	boxTxns = append(boxTxns, &boxTxn)
	//
	//	key := apps.MakeBoxKey(uint64(appid), boxName)
	//	expectedAppBoxes[appid][key] = valPrefix + newBoxValue[len(valPrefix):]
	//}

	appBoxesToSet = map[string]string{
		"fantabulous":        "Italian food's the best!", // max char's
		"disappointing box":  "you made it!",
		"AVM is the new EVM": "yes we can!",
	}
	for boxName, valPrefix := range appBoxesToSet {
		key := apps.MakeBoxKey(uint64(appid), boxName)
		expectedAppBoxes[appid][key] = valPrefix + newBoxValue[len(valPrefix):]
	}
	vb6, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/BoxCreateMutateDelete6.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb6)
	require.NoError(t, err)
	_, round = db.Applications(context.Background(), opts)
	require.Equal(t, uint64(currentRound), round)

	comparator(t, db, expectedAppBoxes, nil, true)

	fmt.Printf("runBoxCreateMutateDelete total time: %s\n", time.Since(start))
}

// Test that box evolution is ingested as expected across rounds using API to compare
func TestBoxCreateMutateDeleteAgainstHandler(t *testing.T) {
	runBoxCreateMutateDelete(t, compareAppBoxesAgainstHandler)
}

func makeRequest(t *testing.T, listenAddr string, path string, includeDeleted bool) (*http.Response, []byte) {
	// make a real HTTP request
	t.Log("making HTTP request path", path)
	req := fmt.Sprintf("http://%s%s?pretty", listenAddr, path)
	if includeDeleted {
		req = fmt.Sprintf("%s&include-all=true", req)
	}
	resp, err := http.Get(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp, body
}

// recreate tests from  common.sh/create_delete_tests()
func TestAppDelete(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()
	opts := defaultOpts
	listenAddr := "localhost:8890"
	go Serve(serverCtx, listenAddr, db, nil, logrus.New(), opts)

	waitForServer(t, listenAddr)

	///////////
	// Given // A block containing app creators, who also holds and has opted-into those apps, and an deleted app.
	///////////

	//appSchema := basics.StateSchema{
	//	NumUint:      1,
	//	NumByteSlice: 1,
	//}
	//createAppTxn := test.MakeCreateAppTxn(test.AccountA)
	//createAppTxnB := test.MakeCreateAppTxn(test.AccountB)
	//createAppTxnB.Txn.LocalStateSchema = appSchema
	//appOptInTxnA := test.MakeAppOptInTxn(expectedAppIdx, test.AccountA)
	//appOptInTxnB := test.MakeAppOptInTxn(expectedAppIdx2, test.AccountB)
	//deleteAppTxn := test.MakeAppDestroyTxn(expectedAppIdx, test.AccountA)
	//
	//block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &createAppTxn, &createAppTxnB, &appOptInTxnA, &appOptInTxnB, &deleteAppTxn)
	//require.NoError(t, err)

	const expectedAppIdx = 1 // must be 1 since this is the first txn
	const expectedAppIdx2 = 2
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/AppDelete.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	//////////
	// When // We look up a deleted app with a included-all flag on/off
	//////////

	testCases := []struct {
		name           string
		appID          int
		includeDeleted bool
		statusCode     int
	}{
		{name: "app create & delete, included-all=false", appID: expectedAppIdx, includeDeleted: false, statusCode: 404},
		{name: "app create & delete, included-all=true", appID: expectedAppIdx, includeDeleted: true, statusCode: 200},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path := fmt.Sprintf("/v2/applications/%d", tc.appID)
			resp, data := makeRequest(t, listenAddr, path, tc.includeDeleted)
			if resp.StatusCode != http.StatusOK {
				require.Equal(t, tc.statusCode, resp.StatusCode)
				return
			}
			require.Equal(t, http.StatusOK, resp.StatusCode, fmt.Sprintf("unexpected return code, body: %s", string(data)))
			var response generated.ApplicationResponse
			err := json.Decode(data, &response)
			require.NoError(t, err)
			assert.True(t, *response.Application.Deleted)
			assert.Equal(t, uint64(1), *response.Application.CreatedAtRound)
			assert.Equal(t, uint64(1), *response.Application.DeletedAtRound)
			assert.Equal(t, uint64(tc.appID), response.Application.Id)
		})
	}

	//////////
	// And // look up the accounts opted in to the apps
	//////////

	// deleted application excluded
	path := fmt.Sprintf("/v2/accounts/%s", test.AccountA.String())
	resp, data := makeRequest(t, listenAddr, path, false)
	require.Equal(t, http.StatusOK, resp.StatusCode, fmt.Sprintf("unexpected return code, body: %s", string(data)))
	var account generated.AccountResponse
	err = json.Decode(data, &account)
	require.NoError(t, err)
	assert.Nil(t, account.Account.CreatedApps)
	//	deleted application include
	resp, data = makeRequest(t, listenAddr, path, true)
	require.Equal(t, http.StatusOK, resp.StatusCode, fmt.Sprintf("unexpected return code, body: %s", string(data)))
	err = json.Decode(data, &account)
	require.NoError(t, err)
	assert.Equal(t, 1, len(*account.Account.CreatedApps))
	assert.True(t, *(*account.Account.CreatedApps)[0].Deleted)
	assert.Equal(t, uint64(expectedAppIdx), (*account.Account.CreatedApps)[0].Id)

	//	query account with an app
	path = fmt.Sprintf("/v2/accounts/%s", test.AccountB.String())
	resp, data = makeRequest(t, listenAddr, path, false)
	require.Equal(t, http.StatusOK, resp.StatusCode, fmt.Sprintf("unexpected return code, body: %s", string(data)))
	var accountB generated.AccountResponse
	err = json.Decode(data, &accountB)
	require.NoError(t, err)
	assert.Equal(t, 1, len(*accountB.Account.CreatedApps))
	assert.Equal(t, uint64(1), (*accountB.Account.CreatedApps)[0].Params.LocalStateSchema.NumByteSlice)
	assert.Equal(t, uint64(1), (*accountB.Account.CreatedApps)[0].Params.LocalStateSchema.NumUint)
	assert.Equal(t, uint64(expectedAppIdx2), (*accountB.Account.CreatedApps)[0].Id)
}
func TestAssetDelete(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()
	opts := defaultOpts
	listenAddr := "localhost:8891"
	go Serve(serverCtx, listenAddr, db, nil, logrus.New(), opts)

	waitForServer(t, listenAddr)
	///////////
	// Given // A block containing a creator of an asset, who also holds and has opted-into this assset, and asset deleted.
	///////////
	//createAssetTxn := test.MakeAssetConfigTxn(0, 100, 0, false, "UNIT", "Asset1", "http://asset1.com", test.AccountA)
	//assetOptInTxnA := test.MakeAssetOptInTxn(expectedAssetIdx, test.AccountA)
	//deleteAssetTxn := test.MakeAssetDestroyTxn(expectedAssetIdx, test.AccountA)
	//block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &createAssetTxn, &assetOptInTxnA, &deleteAssetTxn)
	//require.NoError(t, err)

	const expectedAssetIdx = 1 // must be 1 since this is the first txn
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/AssetDelete.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	//////////
	// When // We look up a deleted asset with a included-all flag on/off
	//////////
	testCases := []struct {
		name           string
		assetID        int
		includeDeleted bool
		statusCode     int
	}{
		{name: "asset create / destroy, account = test.AccountA, included-all=false", assetID: expectedAssetIdx, includeDeleted: false, statusCode: 404},
		{name: "asset create / destroy, account = test.AccountA, included-all=true", assetID: expectedAssetIdx, includeDeleted: true, statusCode: 200},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path := fmt.Sprintf("/v2/assets/%d", tc.assetID)
			resp, data := makeRequest(t, listenAddr, path, tc.includeDeleted)
			if resp.StatusCode != http.StatusOK {
				require.Equal(t, tc.statusCode, resp.StatusCode)
				return
			}
			require.Equal(t, http.StatusOK, resp.StatusCode, fmt.Sprintf("unexpected return code, body: %s", string(data)))
			var response generated.AssetResponse
			err := json.Decode(data, &response)
			require.NoError(t, err)
			assert.True(t, *response.Asset.Deleted)
			assert.Equal(t, uint64(1), *response.Asset.CreatedAtRound)
			assert.Equal(t, uint64(1), *response.Asset.DestroyedAtRound)
			assert.Equal(t, uint64(0), response.Asset.Params.Total)
		})
	}

	// Look up apars to make sure they're not there
	resp, data := makeRequest(t, listenAddr, fmt.Sprintf("/v2/blocks/%d", vb.Block.Round), false)
	require.Equal(t, http.StatusOK, resp.StatusCode, fmt.Sprintf("unexpected return code for blocks endpoint, body: %s", string(data)))
	var blk generated.BlockResponse
	err = json.Decode(data, &blk)
	require.NoError(t, err)
	assert.NotNil(t, blk.Transactions)
	blkTxns := *blk.Transactions
	assert.Equal(t, 3, len(blkTxns))
	// Create Txn -- non-nil apars
	assert.NotNil(t, blkTxns[0].AssetConfigTransaction)
	blkAssetCfgCreate := *blkTxns[0].AssetConfigTransaction
	assert.NotNil(t, blkAssetCfgCreate.Params)
	// Delete Txn -- nil apars
	assert.NotNil(t, blkTxns[2].AssetConfigTransaction)
	blkAssetCfgDelete := *blkTxns[2].AssetConfigTransaction
	assert.Nil(t, blkAssetCfgDelete.Params)

	//////////
	// And // look up the accounts opted in to the asset
	//////////

	// deleted asset excluded
	path := fmt.Sprintf("/v2/accounts/%s", test.AccountA)
	resp, data = makeRequest(t, listenAddr, path, false)
	require.Equal(t, http.StatusOK, resp.StatusCode, fmt.Sprintf("unexpected return code, body: %s", string(data)))
	var account generated.AccountResponse
	err = json.Decode(data, &account)
	require.NoError(t, err)
	assert.Nil(t, account.Account.Assets)

	// deleted asset include
	resp, data = makeRequest(t, listenAddr, path, true)
	require.Equal(t, http.StatusOK, resp.StatusCode, fmt.Sprintf("unexpected return code, body: %s", string(data)))
	err = json.Decode(data, &account)
	require.NoError(t, err)
	assert.Equal(t, 1, len(*account.Account.Assets))
	assert.Equal(t, uint64(0), (*account.Account.Assets)[0].Amount)
	assert.Equal(t, uint64(expectedAssetIdx), (*account.Account.Assets)[0].AssetId)
	assert.True(t, *(*account.Account.Assets)[0].Deleted)
	assert.False(t, (*account.Account.Assets)[0].IsFrozen)
	assert.Equal(t, uint64(1), *(*account.Account.Assets)[0].OptedInAtRound)
	assert.Equal(t, uint64(1), *(*account.Account.Assets)[0].OptedOutAtRound)
}

func TestApplicationLocal(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()
	opts := defaultOpts
	listenAddr := "localhost:8892"
	go Serve(serverCtx, listenAddr, db, nil, logrus.New(), opts)

	waitForServer(t, listenAddr)

	///////////
	// Given // Blocks containing app create, app optin and app outs txns
	///////////

	/*
		#pragma version 5;int 0;byte "cX";byte "value";app_local_put;int 1;return
	*/
	//b, err := base64.StdEncoding.DecodeString("BYEAgAJjWIAFdmFsdWVmgQFD")
	//require.NoError(t, err)
	//
	//createAppA := test.MakeCreateAppTxn(test.AccountA)
	//createAppA.Txn.LocalStateSchema = basics.StateSchema{
	//	NumByteSlice: 1,
	//}
	//createAppA.Txn.OnCompletion = transactions.OptInOC
	//createAppA.Txn.ApprovalProgram = b
	//createAppB := test.MakeCreateAppTxn(test.AccountB)
	//appOptinTxnB := test.MakeAppOptInTxn(expectedAppIdx2, test.AccountB)
	//appOptinTxnC := test.MakeAppOptInTxn(expectedAppIdx, test.AccountC)
	//appOptoutTxnB := test.MakeAppOptOutTxn(expectedAppIdx2, test.AccountB)
	//destroyAppB := test.MakeAppDestroyTxn(expectedAppIdx2, test.AccountB)
	//appOptoutTxnC := test.MakeAppOptOutTxn(expectedAppIdx, test.AccountC)

	//txns := []transactions.SignedTxnWithAD{createAppA, createAppB, appOptinTxnB, appOptinTxnC, appOptoutTxnB, appOptinTxnB, appOptoutTxnC, destroyAppB, appOptinTxnC}
	//prevHeader := test.MakeGenesisBlock().BlockHeader
	//for _, txn := range txns {
	//	block, err := test.MakeBlockForTxns(prevHeader, &txn)
	//	require.NoError(t, err)
	//	err = proc(&rpcs.EncodedBlockCert{Block: block})
	//	require.NoError(t, err)
	//	prevHeader = block.BlockHeader
	//}

	const expectedAppIdx = 1 // must be 1 since this is the first txn
	const expectedAppIdx2 = 2
	for i := 1; i <= 9; i++ {
		vb, err := test.ReadValidatedBlockFromFile(fmt.Sprintf("test_resources/validated_blocks/ApplicationLocal%d.vb", i))
		require.NoError(t, err)
		err = db.AddBlock(&vb)
		require.NoError(t, err)
	}

	///////////
	// When // Look up AccountA, which has created and opted in an app
	///////////
	path := fmt.Sprintf("/v2/accounts/%s", test.AccountA.String())
	resp, data := makeRequest(t, listenAddr, path, false)
	require.Equal(t, http.StatusOK, resp.StatusCode, fmt.Sprintf("unexpected return code, body: %s", string(data)))
	var accountA generated.AccountResponse
	err := json.Decode(data, &accountA)
	require.NoError(t, err)
	assert.False(t, *(*accountA.Account.AppsLocalState)[0].Deleted)
	assert.Equal(t, uint64(1), *(*accountA.Account.AppsLocalState)[0].OptedInAtRound)
	assert.Equal(t, "Y1g=", (*(*accountA.Account.AppsLocalState)[0].KeyValue)[0].Key)

	///////////
	// When // Look up AccountB, which has optedin/closedout/optedin an app that is also closed out
	///////////
	// deleted app excluded
	path = fmt.Sprintf("/v2/accounts/%s", test.AccountB.String())
	resp, data = makeRequest(t, listenAddr, path, false)
	require.Equal(t, http.StatusOK, resp.StatusCode, fmt.Sprintf("unexpected return code, body: %s", string(data)))
	var accountB generated.AccountResponse
	err = json.Decode(data, &accountB)
	require.NoError(t, err)
	assert.Nil(t, accountB.Account.CreatedAssets)

	// deleted app included
	resp, data = makeRequest(t, listenAddr, path, true)
	require.Equal(t, http.StatusOK, resp.StatusCode, fmt.Sprintf("unexpected return code, body: %s", string(data)))
	var accountBfull generated.AccountResponse
	err = json.Decode(data, &accountBfull)
	require.NoError(t, err)
	assert.True(t, *(*accountBfull.Account.CreatedApps)[0].Deleted)
	assert.Equal(t, uint64(3), *(*accountBfull.Account.AppsLocalState)[0].OptedInAtRound)
	assert.Equal(t, uint64(5), *(*accountBfull.Account.AppsLocalState)[0].ClosedOutAtRound)

	///////////
	// When // Look up AccountC, which has optedin/closedout/optedin an app
	///////////
	path = fmt.Sprintf("/v2/accounts/%s", test.AccountC.String())
	resp, data = makeRequest(t, listenAddr, path, true)
	require.Equal(t, http.StatusOK, resp.StatusCode, fmt.Sprintf("unexpected return code, body: %s", string(data)))
	var accountC generated.AccountResponse
	err = json.Decode(data, &accountC)
	require.NoError(t, err)
	assert.False(t, *(*accountC.Account.AppsLocalState)[0].Deleted)
	assert.Equal(t, uint64(4), *(*accountC.Account.AppsLocalState)[0].OptedInAtRound)
	assert.Equal(t, uint64(7), *(*accountC.Account.AppsLocalState)[0].ClosedOutAtRound)
	assert.Equal(t, uint64(1), (*accountC.Account.AppsLocalState)[0].Schema.NumByteSlice)
}

func TestAccounts(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()
	opts := defaultOpts
	listenAddr := "localhost:8893"
	go Serve(serverCtx, listenAddr, db, nil, logrus.New(), opts)

	waitForServer(t, listenAddr)

	///////////
	// When // Looking up a genesis account
	///////////
	path := fmt.Sprintf("/v2/accounts/%s", test.AccountA.String())
	resp, data := makeRequest(t, listenAddr, path, false)
	require.Equal(t, http.StatusOK, resp.StatusCode, fmt.Sprintf("unexpected return code, body: %s", string(data)))
	var account generated.AccountResponse
	err := json.Decode(data, &account)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), *account.Account.CreatedAtRound)
	assert.False(t, *account.Account.Deleted)

	///////////
	// Given // Blocks containing multiple create account and delete account transactions for AccountE
	///////////

	//createAcct := test.MakePaymentTxn(1, 100, 0, 0, 0, 0, test.AccountA, test.AccountE, basics.Address{}, basics.Address{})
	//closeAcct := test.MakePaymentTxn(1, 0, 100, 0, 0, 0, test.AccountE, test.AccountA, test.AccountA, basics.Address{})
	//
	//// account create close create close
	//txns := []transactions.SignedTxnWithAD{createAcct, closeAcct, createAcct, closeAcct}
	//prevHeader := test.MakeGenesisBlock().BlockHeader
	//for _, txn := range txns {
	//	block, err := test.MakeBlockForTxns(prevHeader, &txn)
	//	require.NoError(t, err)
	//	err = proc(&rpcs.EncodedBlockCert{Block: block})
	//	require.NoError(t, err)
	//	prevHeader = block.BlockHeader
	//}

	for i := 1; i <= 4; i++ {
		vb, err := test.ReadValidatedBlockFromFile(fmt.Sprintf("test_resources/validated_blocks/Accounts%d.vb", i))
		require.NoError(t, err)
		err = db.AddBlock(&vb)
		require.NoError(t, err)
	}

	///////////
	// When // Look up AccountE, we should get a 404 status code
	///////////
	path = fmt.Sprintf("/v2/accounts/%s", test.AccountE.String())
	resp, data = makeRequest(t, listenAddr, path, false)
	require.Equal(t, http.StatusNotFound, resp.StatusCode, fmt.Sprintf("unexpected return code, body: %s", string(data)))

	///////////
	// Given // A block containing a creator of an asset, who also holds and has opted-into this assset.
	///////////
	//createAsset := test.MakeAssetConfigTxn(0, 1000000000000, 0, false, "bogo", "bogocoin", "http://bogocoin.com", test.AccountA)
	//block, err := test.MakeBlockForTxns(prevHeader, &createAsset)
	//require.NoError(t, err)
	//block.BlockHeader.Round = 5
	//err = proc(&rpcs.EncodedBlockCert{Block: block})
	//require.NoError(t, err)
	const expectedAssetIdx = 5
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/Accounts5.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)
	stxn, _, err := util.DecodeSignedTxn(vb.Block.BlockHeader, vb.Block.Payset[0])
	require.NoError(t, err)
	assetID := crypto.TransactionIDString(stxn.Txn)

	///////////
	// When // Look up transaction containing this asset
	///////////
	path = fmt.Sprintf("/v2/transactions/%s", assetID)
	resp, data = makeRequest(t, listenAddr, path, false)
	require.Equal(t, http.StatusOK, resp.StatusCode, fmt.Sprintf("unexpected return code, body: %s", string(data)))
	var txn generated.TransactionResponse
	err = json.Decode(data, &txn)
	require.NoError(t, err)
	///////////
	// Then  // Parameter fields are correct and base64 fields are serialized
	///////////
	assert.Contains(t, string(data), `"name-b64": "Ym9nb2NvaW4="`)
	assert.Contains(t, string(data), `"unit-name-b64": "Ym9nbw=="`)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), txn.Transaction.AssetConfigTransaction.Params.Decimals)
	assert.False(t, *txn.Transaction.AssetConfigTransaction.Params.DefaultFrozen)
	assert.Equal(t, "bogocoin", *txn.Transaction.AssetConfigTransaction.Params.Name)
	assert.Equal(t, test.AccountA.String(), *txn.Transaction.AssetConfigTransaction.Params.Reserve)
	assert.Equal(t, uint64(1000000000000), txn.Transaction.AssetConfigTransaction.Params.Total)
	assert.Equal(t, "bogo", *txn.Transaction.AssetConfigTransaction.Params.UnitName)
	assert.Equal(t, uint64(expectedAssetIdx), *txn.Transaction.CreatedAssetIndex)

	///////////
	// When // Look up the asset
	///////////
	path = fmt.Sprintf("/v2/assets/%d", 0)
	resp, data = makeRequest(t, listenAddr, path, false)
	require.Equal(t, http.StatusOK, resp.StatusCode, fmt.Sprintf("unexpected return code, body: %s", string(data)))
	var asset generated.AssetResponse
	err = json.Decode(data, &asset)
	require.NoError(t, err)
	///////////
	// Then  // Parameter fields are correct and base64 fields are serialized
	///////////
	assert.Contains(t, string(data), `"name-b64": "Ym9nb2NvaW4="`)
	assert.Contains(t, string(data), `"unit-name-b64": "Ym9nbw=="`)
	assert.Equal(t, uint64(0), asset.Asset.Params.Decimals)
	assert.False(t, *asset.Asset.Params.DefaultFrozen)
	assert.Equal(t, "bogocoin", *asset.Asset.Params.Name)
	assert.Equal(t, test.AccountA.String(), *asset.Asset.Params.Reserve)
	assert.Equal(t, uint64(1000000000000), asset.Asset.Params.Total)
	assert.Equal(t, "bogo", *asset.Asset.Params.UnitName)

	///////////
	// When // Look up the account containing this asset
	///////////
	path = fmt.Sprintf("/v2/accounts/%s", test.AccountA.String())
	resp, data = makeRequest(t, listenAddr, path, false)
	require.Equal(t, http.StatusOK, resp.StatusCode, fmt.Sprintf("unexpected return code, body: %s", string(data)))
	var acctA generated.AccountResponse
	err = json.Decode(data, &acctA)
	require.NoError(t, err)
	///////////
	// Then  // Parameter fields are correct and base64 fields are serialized
	///////////
	assert.Contains(t, string(data), `"name-b64": "Ym9nb2NvaW4="`)
	assert.Contains(t, string(data), `"unit-name-b64": "Ym9nbw=="`)
	assert.Equal(t, uint64(0), (*acctA.Account.CreatedAssets)[0].Params.Decimals)
	assert.False(t, *(*acctA.Account.CreatedAssets)[0].Params.DefaultFrozen)
	assert.Equal(t, "bogocoin", *(*acctA.Account.CreatedAssets)[0].Params.Name)
	assert.Equal(t, test.AccountA.String(), *(*acctA.Account.CreatedAssets)[0].Params.Reserve)
	assert.Equal(t, uint64(1000000000000), (*acctA.Account.CreatedAssets)[0].Params.Total)
	assert.Equal(t, "bogo", *(*acctA.Account.CreatedAssets)[0].Params.UnitName)
}
