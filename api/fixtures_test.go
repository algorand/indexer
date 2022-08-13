package api

import (
	"context"
	stdJson "encoding/json"
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
	"github.com/stretchr/testify/require"

	"github.com/algorand/go-algorand-sdk/encoding/json"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/data/transactions/logic"
	"github.com/algorand/go-algorand/ledger"
	"github.com/algorand/go-algorand/rpcs"

	"github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/idb/postgres"
	"github.com/algorand/indexer/processor"
	"github.com/algorand/indexer/util/test"
)

const fixtestListenAddr = "localhost:8989"
const fixtestBaseUrl = "http://" + fixtestListenAddr
const fixtestMaxStartup time.Duration = 5 * time.Second
const fixturesDirectory = "test_resources/"

type fixture struct {
	File         string     `json:"file"`
	Owner        string     `json:"owner"`
	LastModified string     `json:"lastModified"`
	Frozen       bool       `json:"frozen"`
	Cases        []testCase `json:"cases"`
}
type testCase struct {
	Name         string       `json:"name"`
	Request      requestInfo  `json:"request"`
	Response     responseInfo `json:"response"`
	Witness      interface{}  `json:"witness"`
	WitnessError *string      `json:"witnessError"`
}
type requestInfo struct {
	Endpoint string  `json:"endpoint"`
	Params   []param `json:"params"`
}
type param struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type responseInfo struct {
	StatusCode int    `json:"statusCode"`
	Body       string `json:"body"`
	// BodyErr       string `json:"bodyErr"`
}
type prover func(responseInfo) (interface{}, *string)

func (f *testCase) proof() (prover, error) {
	path := f.Request.Endpoint
	if len(path) == 0 || path[0] != '/' {
		return nil, fmt.Errorf("invalid endpoint [%s]", path)
	}
	_, p, e := getProof(path[1:])
	return p, e
}

type proofPath struct {
	parts map[string]proofPath
	proof prover
}

var proverRoutes = proofPath{
	parts: map[string]proofPath{
		"v2": {
			parts: map[string]proofPath{
				"accounts": {
					parts: map[string]proofPath{
						":account-id": {
							proof: nil,
						},
					},
				},
				"applications": {
					parts: map[string]proofPath{
						":application-id": {
							proof: nil,
							parts: map[string]proofPath{
								"box": {
									proof: boxProof,
								},
								"boxes": {
									proof: nil,
								},
							},
						},
					},
				},
			},
		},
	},
}

func getProof(path string) (route string, proof prover, err error) {
	var impl func(string, []string, proofPath) (string, prover, error)
	impl = func(prefix string, suffix []string, node proofPath) (path string, proof prover, err error) {
		if len(suffix) == 0 {
			return prefix, node.proof, nil
		}
		part := suffix[0]
		next, ok := node.parts[part]
		if ok {
			return impl(prefix+"/"+part, suffix[1:], next)
		}
		// look for a wild-card part, e.g. ":application-id"
		for routePart, next := range node.parts {
			if routePart[0] == ':' {
				return impl(prefix+"/"+routePart, suffix[1:], next)
			}
		}
		// no wild-card, so an error
		return prefix, nil, fmt.Errorf("<<<suffix=%+v; node=%+v>>>\nfollowing sub-path (%s) cannot find part [%s]", suffix, node, prefix, part)
	}

	return impl("", strings.Split(path, "/"), proverRoutes)
}

// WARNING: receiver should not call l.Close()
func setupIdbAndReturnShutdownFunc(t *testing.T) (db *postgres.IndexerDb, proc processor.Processor, l *ledger.Ledger, shutdown func()) {
	db, dbShutdown, proc, l := setupIdb(t, test.MakeGenesis())

	shutdown = func() {
		dbShutdown()
		l.Close()
	}

	return
}

func setupLiveServerAndReturnShutdownFunc(t *testing.T, db *postgres.IndexerDb) (shutdown func()) {
	serverCtx, shutdown := context.WithCancel(context.Background())
	go Serve(serverCtx, fixtestListenAddr, db, nil, logrus.New(), fixtestServerOpts)

	serverUp := false
	for maxWait := fixtestMaxStartup; !serverUp && maxWait > 0; maxWait -= 50 * time.Millisecond {
		time.Sleep(50 * time.Millisecond)
		resp, _, reqErr, bodyErr := getRequest(t, "/health", []param{})
		if reqErr != nil || bodyErr != nil {
			t.Log("waiting for server:", reqErr, bodyErr)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			t.Log("waiting for server OK:", resp.StatusCode)
			continue
		}
		serverUp = true
	}
	require.True(t, serverUp, "api.Serve did not start server in time")

	return
}

func readFixture(t *testing.T, path string, live fixture) fixture {
	fileBytes, err := ioutil.ReadFile(path + live.File)
	require.NoError(t, err)

	saved := fixture{}
	err = stdJson.Unmarshal(fileBytes, &saved)
	require.NoError(t, err)

	return saved
}

func writeFixture(t *testing.T, path string, save fixture) {
	fileBytes, err := stdJson.MarshalIndent(save, "", "  ")
	require.NoError(t, err)

	err = ioutil.WriteFile(path+save.File, fileBytes, 0644)
	require.NoError(t, err)
}

var fixtestServerOpts = ExtraOptions{
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

func getRequest(t *testing.T, endpoint string, params []param) (resp *http.Response, body []byte, reqErr, bodyErr error) {
	verbose := true

	path := fixtestBaseUrl + endpoint
	prefix := "?"
	for i, param := range params {
		if i > 0 {
			prefix = "&"
		}
		path += prefix + param.Name + "=" + param.Value
	}

	t.Log("making HTTP request path", path)
	resp, reqErr = http.Get(path)
	if reqErr != nil {
		reqErr = fmt.Errorf("client: error making http request: %w", reqErr)
		return
	}
	require.NoError(t, reqErr)
	defer resp.Body.Close()

	body, bodyErr = ioutil.ReadAll(resp.Body)

	if verbose {
		fmt.Printf(`
resp=%+v
body=%s
reqErr=%v
bodyErr=%v`, resp, string(body), reqErr, bodyErr)
	}

	return
}

func setupLiveBoxes(t *testing.T, proc processor.Processor, l *ledger.Ledger) {
	deleted := "DELETED"

	appid := basics.AppIndex(1)

	/**** ROUND 1: create and fund the box app ****/
	currentRound := basics.Round(1)

	createTxn, err := test.MakeComplexCreateAppTxn(test.AccountA, test.BoxApprovalProgram, test.BoxClearProgram, 8)
	require.NoError(t, err)

	payNewAppTxn := test.MakePaymentTxn(1000, 500000, 0, 0, 0, 0, test.AccountA, appid.Address(), basics.Address{},
		basics.Address{})

	block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &createTxn, &payNewAppTxn)
	require.NoError(t, err)

	err = proc.Process(&rpcs.EncodedBlockCert{Block: block})
	require.NoError(t, err)

	// block header handoff: round 1 --> round 2
	blockHdr, err := l.BlockHdr(currentRound)
	require.NoError(t, err)

	/**** ROUND 2: create 8 boxes for appid == 1 ****/
	currentRound = basics.Round(2)

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

	expectedAppBoxes := map[basics.AppIndex]map[string]string{}
	expectedAppBoxes[appid] = map[string]string{}
	newBoxValue := "\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"
	boxTxns := make([]*transactions.SignedTxnWithAD, 0)
	for _, boxName := range boxNames {
		expectedAppBoxes[appid][logic.MakeBoxKey(appid, boxName)] = newBoxValue
		args := []string{"create", boxName}
		boxTxn := test.MakeAppCallTxnWithBoxes(uint64(appid), test.AccountA, args, []string{boxName})
		boxTxns = append(boxTxns, &boxTxn)
	}

	block, err = test.MakeBlockForTxns(blockHdr, boxTxns...)
	require.NoError(t, err)

	err = proc.Process(&rpcs.EncodedBlockCert{Block: block})
	require.NoError(t, err)

	// block header handoff: round 2 --> round 3
	blockHdr, err = l.BlockHdr(currentRound)
	require.NoError(t, err)

	/**** ROUND 3: populate the boxes appropriately ****/
	currentRound = basics.Round(3)

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

	boxTxns = make([]*transactions.SignedTxnWithAD, 0)
	expectedAppBoxes[appid] = make(map[string]string)
	for boxName, valPrefix := range appBoxesToSet {
		args := []string{"set", boxName, valPrefix}
		boxTxn := test.MakeAppCallTxnWithBoxes(uint64(appid), test.AccountA, args, []string{boxName})
		boxTxns = append(boxTxns, &boxTxn)

		key := logic.MakeBoxKey(appid, boxName)
		expectedAppBoxes[appid][key] = valPrefix + newBoxValue[len(valPrefix):]
	}
	block, err = test.MakeBlockForTxns(blockHdr, boxTxns...)
	require.NoError(t, err)

	err = proc.Process(&rpcs.EncodedBlockCert{Block: block})
	require.NoError(t, err)

	// block header handoff: round 3 --> round 4
	blockHdr, err = l.BlockHdr(currentRound)
	require.NoError(t, err)

	/**** ROUND 4: delete the unhappy boxes ****/
	currentRound = basics.Round(4)

	appBoxesToDelete := []string{
		"not so great box",
		"disappointing box",
		"I'm destined for deletion",
	}

	boxTxns = make([]*transactions.SignedTxnWithAD, 0)
	for _, boxName := range appBoxesToDelete {
		args := []string{"delete", boxName}
		boxTxn := test.MakeAppCallTxnWithBoxes(uint64(appid), test.AccountA, args, []string{boxName})
		boxTxns = append(boxTxns, &boxTxn)

		key := logic.MakeBoxKey(appid, boxName)
		expectedAppBoxes[appid][key] = deleted
	}
	block, err = test.MakeBlockForTxns(blockHdr, boxTxns...)
	require.NoError(t, err)

	err = proc.Process(&rpcs.EncodedBlockCert{Block: block})
	require.NoError(t, err)

	// block header handoff: round 4 --> round 5
	blockHdr, err = l.BlockHdr(currentRound)
	require.NoError(t, err)

	/**** ROUND 5: create 3 new boxes, overwriting one of the former boxes ****/
	currentRound = basics.Round(5)

	appBoxesToCreate := []string{
		"fantabulous",
		"disappointing box", // overwriting here
		"AVM is the new EVM",
	}
	boxTxns = make([]*transactions.SignedTxnWithAD, 0)
	for _, boxName := range appBoxesToCreate {
		args := []string{"create", boxName}
		boxTxn := test.MakeAppCallTxnWithBoxes(uint64(appid), test.AccountA, args, []string{boxName})
		boxTxns = append(boxTxns, &boxTxn)

		key := logic.MakeBoxKey(appid, boxName)
		expectedAppBoxes[appid][key] = newBoxValue
	}
	block, err = test.MakeBlockForTxns(blockHdr, boxTxns...)
	require.NoError(t, err)

	err = proc.Process(&rpcs.EncodedBlockCert{Block: block})
	require.NoError(t, err)

	// block header handoff: round 5 --> round 6
	blockHdr, err = l.BlockHdr(currentRound)
	require.NoError(t, err)

	/**** ROUND 6: populate the 3 new boxes ****/
	currentRound = basics.Round(6)

	appBoxesToSet = map[string]string{
		"fantabulous":        "Italian food's the best!", // max char's
		"disappointing box":  "you made it!",
		"AVM is the new EVM": "yes we can!",
	}
	boxTxns = make([]*transactions.SignedTxnWithAD, 0)
	for boxName, valPrefix := range appBoxesToSet {
		args := []string{"set", boxName, valPrefix}
		boxTxn := test.MakeAppCallTxnWithBoxes(uint64(appid), test.AccountA, args, []string{boxName})
		boxTxns = append(boxTxns, &boxTxn)

		key := logic.MakeBoxKey(appid, boxName)
		expectedAppBoxes[appid][key] = valPrefix + newBoxValue[len(valPrefix):]
	}
	block, err = test.MakeBlockForTxns(blockHdr, boxTxns...)
	require.NoError(t, err)

	err = proc.Process(&rpcs.EncodedBlockCert{Block: block})
	require.NoError(t, err)

	totals := map[basics.AppIndex]map[string]int{}
	for appIndex, appBoxes := range expectedAppBoxes {
		totals[appIndex] = map[string]int{
			"tBoxes":    0,
			"tBoxBytes": 0,
		}
		for k, v := range appBoxes {
			if v != deleted {
				totals[appIndex]["tBoxes"]++
				totals[appIndex]["tBoxBytes"] += len(k) + len(v) - 11
			}
		}
	}
	fmt.Printf("expectedAppBoxes=%+v\n", expectedAppBoxes)
	fmt.Printf("expected totals=%+v\n", totals)
}

// When the provided seed has `seed.Frozen == false` assertions will be skipped.
// On the other hand, when `seed.Frozen == false` assertions are made:
// * ownerVariable == seed.Owner == saved.Owner == live.Owner sanity checks
// * seed.File ( == saved.File == live.File sanity checks)
// * len(seed.Cases) == len(saved.Cases) (== len(live.Cases) sanity check)
// * for each seedCase in seed.Cases:
//   * seedCase.Name == savedCase.Name (== liveCase.Name sanity check)
//   * seedCase.Request == savedCase.Request (== liveCase.Request sanity check)
//   * seedCase.Witness == savedCase.Witness (== liveCase.Witness sanity check)
//   * savedCase.Response == liveCase.Response
//   * seedCase.Proof(liveCase.Response) == savedCase.Witness
// Regardless of `seed.Frozen`, saved fixture `live` to `fixturesDirectory + "_" + seed.File`
// NOTE: `live.Witness` is always recalculated via `seed.proof(live.Response)`
func validateOrGenerateFixtures(t *testing.T, db *postgres.IndexerDb, seed fixture, owner string) {
	require.Equal(t, owner, seed.Owner, "mismatch between purported owners of fixture")

	live := fixture{
		File:   seed.File,
		Owner:  seed.Owner,
		Frozen: seed.Frozen,
	}
	for i, seedCase := range seed.Cases {
		msg := fmt.Sprintf("Case %d. seedCase=%+v.", i, seedCase)
		liveCase := testCase{
			Name:    seedCase.Name,
			Request: seedCase.Request,
		}

		resp, body, reqErr, bodyErr := getRequest(t, seedCase.Request.Endpoint, seedCase.Request.Params)
		require.NoError(t, reqErr, msg)
		require.NoError(t, bodyErr, msg) // not sure about this one!!!

		liveCase.Response = responseInfo{
			StatusCode: resp.StatusCode,
			Body:       string(body),
		}
		msg += fmt.Sprintf(" newResponse=%+v", liveCase.Response)

		prove, err := seedCase.proof()
		require.NoError(t, err, msg)

		if prove != nil {
			witness, errStr := prove(liveCase.Response)
			liveCase.Witness = witness
			liveCase.WitnessError = errStr
		}
		live.Cases = append(live.Cases, liveCase)
	}

	live.LastModified = time.Now().String()
	writeFixture(t, fixturesDirectory+"_", live)

	saved := readFixture(t, fixturesDirectory, seed)
	if seed.Frozen {
		// sanity check:
		require.True(t, live.Frozen, "should be frozen for assertions")

		require.Equal(t, seed.Owner, saved.Owner, "unexpected discrepancy in Owner")
		// sanity check:
		require.Equal(t, saved.Owner, live.Owner, "unexpected discrepancy in Owner")

		require.Equal(t, seed.File, saved.File, "unexpected discrepancy in File")
		// sanity check:
		require.Equal(t, saved.File, live.File, "unexpected discrepancy in File")

		numSeedCases, numSavedCases, numLiveCases := len(seed.Cases), len(saved.Cases), len(live.Cases)
		require.Equal(t, numSeedCases, numSavedCases, "numSeedCases=%d but numSavedCases=%d", numSeedCases, numSavedCases)
		// sanity check:
		require.Equal(t, numSavedCases, numLiveCases, "numSavedCases=%d but numLiveCases=%d", numSavedCases, numLiveCases)

		for i, seedCase := range seed.Cases {
			savedCase, liveCase := saved.Cases[i], live.Cases[i]
			msg := fmt.Sprintf("(%d)[%s]. discrepency in seed=\n%+v\nsaved=\n%+v\nlive=\n%+v\n", i, seedCase.Name, seedCase, savedCase, liveCase)

			require.Equal(t, seedCase.Name, savedCase.Name, msg)
			// sanity check:
			require.Equal(t, savedCase.Name, liveCase.Name, msg)

			require.Equal(t, seedCase.Request, savedCase.Request, msg)
			// sanity check:
			require.Equal(t, savedCase.Request, liveCase.Request, msg)

			require.Equal(t, seedCase.Witness, savedCase.Witness, msg)
			// sanity check:
			require.Equal(t, savedCase.Witness, liveCase.Witness, msg)

			require.Equal(t, savedCase.Response, liveCase.Response, msg)

			require.NotNil(t, seedCase.proof, msg)
			prove, err := seedCase.proof()
			require.NoError(t, err, msg)
			require.NotNil(t, prove, msg)

			proof, errStr := prove(liveCase.Response)
			require.Equal(t, seedCase.Witness, proof, msg)
			if seedCase.WitnessError == nil {
				require.Nil(t, errStr)
			} else {
				require.Equal(t, *seedCase.WitnessError, *errStr)
			}
		}
	}
}

func tempName(t *testing.T, db *postgres.IndexerDb, appBoxes map[basics.AppIndex]map[string]string, deletedBoxes map[basics.AppIndex]map[string]bool, verifyTotals bool) {
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

	remainingBoxes := map[basics.AppIndex]map[string]string{}
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
			expectedAppIdx, boxName, err := logic.SplitBoxKey(key)
			require.NoError(t, err, msg)
			require.Equal(t, appIdx, expectedAppIdx, msg)
			numRequests++

			boxDeleted := false
			if deletedBoxes != nil {
				if _, ok := deletedBoxes[appIdx][key]; ok {
					boxDeleted = true
				}
			}

			c, api, rec := setupRequest("/v2/applications/:appidx/box/", "appidx", strconv.Itoa(int(appIdx)))
			// TODO for prefix in
			//  []string{"str", "string", "int", "integer", "addr", "address",
			//		    "b32", "base32", "byte base32",
			//			"b64", "base64", "byte base64",
			//			"abi"
			//			}
			// ... do the right thing
			prefixedName := fmt.Sprintf("str:%s", boxName)
			params := generated.LookupApplicationBoxByIDandNameParams{Name: prefixedName}
			err = api.LookupApplicationBoxByIDandName(c, uint64(appIdx), params)
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
		// TODO: should we add Order to the search params?
		params := generated.SearchForApplicationBoxesParams{}

		// TODO: also test non-nil Limit, Next
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

			appAddr := appIdx.Address().String()
			c, api, rec = setupRequest("/v2/accounts/:addr", "addr", appAddr)
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

func boxProof(resp responseInfo) (wit interface{}, errStr *string) {
	if resp.StatusCode >= 300 {
		s := fmt.Sprintf("%d error", resp.StatusCode)
		errStr = &s
		return
	}
	box := generated.BoxResponse{}
	err := stdJson.Unmarshal([]byte(resp.Body), &box)
	if err != nil {
		s := fmt.Sprintf("unmarshal err: %s", err)
		errStr = &s
		return
	}
	wit = box
	return
}

/*
expectedAppBoxes=map[1:map[AVM is the new EVM:yes we can! I will be assimilated:THE BORG I'm destined for deletion:DELETED a great box:it's a wonderful box another great box:I'm wonderful too box #8:eight is beautiful disappointing box:you made it! don't box me in this way:non box-conforming fantabulous:Italian food's the best! not so great box:DELETED]]
expected totals=map[1:map[tBoxBytes:317 tBoxes:8]]
appIdx=1
appAddr=WCS6TVPJRBSARHLN2326LRU5BYVJZUKI2VJ53CAWKYYHDE455ZGKANWMGM
path=/v2/accounts/WCS6TVPJRBSARHLN2326LRU5BYVJZUKI2VJ53CAWKYYHDE455ZGKANWMGM
*/
var boxSeedFixture = fixture{
	File:   "boxes.json",
	Owner:  "TestBoxes",
	Frozen: false,
	Cases: []testCase{
		{
			Name: "Lookup non-existing app 1337",
			Request: requestInfo{
				Endpoint: "/v2/applications/1337",
				Params:   []param{},
			},
		},
		{
			Name: "Boxes of a non-existing app 1337",
			Request: requestInfo{
				Endpoint: "/v2/applications/1337/boxes",
				Params:   []param{},
			},
		},
		{
			Name: "A box attempt for a non-existing app 1337",
			Request: requestInfo{
				Endpoint: "/v2/applications/1337/box",
				Params: []param{
					{"name", "string:non-existing"},
				},
			},
		},
		{
			Name: "Lookup existing app 1",
			Request: requestInfo{
				Endpoint: "/v2/applications/1",
				Params:   []param{},
			},
		},
		{
			Name: "App 1 boxes: no params",
			Request: requestInfo{
				Endpoint: "/v2/applications/1/boxes",
				Params:   []param{},
			},
		},
		{
			Name: "App 1 box(non-existing)",
			Request: requestInfo{
				Endpoint: "/v2/applications/1/box",
				Params: []param{
					{"name", "string:non-existing"},
				},
			},
		},
		{
			Name: "App 1 box(a great box) - no params",
			Request: requestInfo{
				Endpoint: "/v2/applications/1/box",
				Params: []param{
					{"name", "string:a great box"},
				},
			},
		},
		{
			Name: "App 1 (as address) totals - no params",
			Request: requestInfo{
				Endpoint: "/v2/accounts/WCS6TVPJRBSARHLN2326LRU5BYVJZUKI2VJ53CAWKYYHDE455ZGKANWMGM",
				Params:   []param{},
			},
		},
	},
}

func TestBoxes(t *testing.T) {
	db, proc, l, dbShutdown := setupIdbAndReturnShutdownFunc(t)
	defer dbShutdown()

	setupLiveBoxes(t, proc, l)

	serverShutdown := setupLiveServerAndReturnShutdownFunc(t, db)
	defer serverShutdown()

	validateOrGenerateFixtures(t, db, boxSeedFixture, "TestBoxes")
}
