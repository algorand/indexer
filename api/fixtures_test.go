package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/algorand/go-algorand/ledger"

	"github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/idb/postgres"
	"github.com/algorand/indexer/processor"
	"github.com/algorand/indexer/util/test"
)

/* **************************************
This Fixtures Test simulates calling indexer GET endpoints and in so doing validates:
* Hand crafted URL's are:
	* routed to the correct endpoint
	* handled as expected
* HTTP Responses are:
	* well formatted and can be parsed to produce results of the expected type
	* have the state that is expected for a "real" blockchain

This is achieved as follows:
A. Set up the state of the blockchain
B. Iterate through a hard-coded `xyzSeedFixture` to generate an `xyzLiveFixture` as follows. For each seed object in `xyzSeedFixture`:
	1. Query indexer via HTTP for the state of the blockchain using the `seed`'s `Request.Path` + `Request.Params`
	2. Query the test-internal `proverRoutes` struct for a route-compatible `prover`
	3. Employ the `prover` to parse the indexer's response into a route appropriate `witness` object
C. Save `xyzLiveFixture` into a non-git-committed fixture `./test_resources/_FIXTURE_NAME.json` (NOTICE THE `_` PREFIX)
D. Assert that the non-git-commited fixture equals the git-committed version `./test_resources/FIXTURE_NAME.json`

It is the responsibility of the test writer to check that the generated fixture represent the expected results.

----
TODO: SHOULD this be used to auto-generate unit test cases in the SDK's?
In particular, one could craft a generic cucumber test that looks something like:

* ("fixture parsing")
	Given an indexer fixture file <file-name>.
* ("fixture mock setup")
	When a mock indexer server has been set up using the urls and responses provided by the indexer fixture,
* ("fixture validation")
	Then iterate through all of the fixture's test cases: query the mock indexer server, parse the response into an SDK object, and validate that it comports with the witness.

The third step ("fixture validation") is unusually high-level for our cucumber tests, but it would allow for a more streamlined unit tests bootstrapping.
Implementoing the "fixture validation" step would logically be broken down as follows:

For each `testCase`` in the parsed fixture:
	1. Use the `witness` type together with the `request` to find an appropriate calling SDK `method`.
	2. Call the SDK `method` against the mock indexer server.
	3. Validate that the resulting SDK object comports with the `witness`.
************************************** */

const fixtestListenAddr = "localhost:8999"
const fixtestBaseURL = "http://" + fixtestListenAddr
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
	Path   string  `json:"path"`
	Params []param `json:"params"`
	URL    string  `json:"url"`
	Route  string  `json:"route"` // `Route` stores the simulated route found in `proverRoutes`
}
type param struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type responseInfo struct {
	StatusCode int    `json:"statusCode"`
	Body       string `json:"body"`
}
type prover func(responseInfo) (interface{}, *string)

// ---- BEGIN provers / witness generators ---- //

func accountsProof(resp responseInfo) (wit interface{}, errStr *string) {
	accounts := generated.AccountsResponse{}
	errStr = parseForProver(resp, &accounts)
	if errStr != nil {
		return
	}
	wit = struct {
		Type     string                     `json:"goType"`
		Accounts generated.AccountsResponse `json:"accounts"`
	}{
		Type:     fmt.Sprintf("%T", accounts),
		Accounts: accounts,
	}
	return
}
func accountInfoProof(resp responseInfo) (wit interface{}, errStr *string) {
	account := generated.AccountResponse{}
	errStr = parseForProver(resp, &account)
	if errStr != nil {
		return
	}
	wit = struct {
		Type    string                    `json:"goType"`
		Account generated.AccountResponse `json:"account"`
	}{
		Type:    fmt.Sprintf("%T", account),
		Account: account,
	}
	return
}

func appsProof(resp responseInfo) (wit interface{}, errStr *string) {
	apps := generated.ApplicationsResponse{}
	errStr = parseForProver(resp, &apps)
	if errStr != nil {
		return
	}
	wit = struct {
		Type string                         `json:"goType"`
		Apps generated.ApplicationsResponse `json:"apps"`
	}{
		Type: fmt.Sprintf("%T", apps),
		Apps: apps,
	}
	return
}

func appInfoProof(resp responseInfo) (wit interface{}, errStr *string) {
	app := generated.ApplicationResponse{}
	errStr = parseForProver(resp, &app)
	if errStr != nil {
		return
	}
	wit = struct {
		Type string                        `json:"goType"`
		App  generated.ApplicationResponse `json:"app"`
	}{
		Type: fmt.Sprintf("%T", app),
		App:  app,
	}
	return
}

func boxProof(resp responseInfo) (wit interface{}, errStr *string) {
	box := generated.BoxResponse{}
	errStr = parseForProver(resp, &box)
	if errStr != nil {
		return
	}
	wit = struct {
		Type string                `json:"goType"`
		Box  generated.BoxResponse `json:"box"`
	}{
		Type: fmt.Sprintf("%T", box),
		Box:  box,
	}
	return
}

func boxesProof(resp responseInfo) (wit interface{}, errStr *string) {
	boxes := generated.BoxesResponse{}
	errStr = parseForProver(resp, &boxes)
	if errStr != nil {
		return
	}
	wit = struct {
		Type  string                  `json:"goType"`
		Boxes generated.BoxesResponse `json:"boxes"`
	}{
		Type:  fmt.Sprintf("%T", boxes),
		Boxes: boxes,
	}
	return
}

func parseForProver(resp responseInfo, reconstructed interface{}) (errStr *string) {
	if resp.StatusCode >= 300 {
		s := fmt.Sprintf("%d error", resp.StatusCode)
		errStr = &s
		return
	}
	err := json.Unmarshal([]byte(resp.Body), reconstructed)
	if err != nil {
		s := fmt.Sprintf("unmarshal err: %s", err)
		errStr = &s
		return
	}
	return nil
}

// ---- END provers / witness generators ---- //

func (f *testCase) proverFromEndoint() (prover, error) {
	path := f.Request.Path
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
					proof: accountsProof,
					parts: map[string]proofPath{
						":account-id": {
							proof: accountInfoProof,
						},
					},
				},
				"applications": {
					proof: appsProof,
					parts: map[string]proofPath{
						":application-id": {
							proof: appInfoProof,
							parts: map[string]proofPath{
								"box": {
									proof: boxProof,
								},
								"boxes": {
									proof: boxesProof,
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
		_, resp, _, reqErr, bodyErr := getRequest(t, "/health", []param{})
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

func readFixture(t *testing.T, path string, seed *fixture) fixture {
	fileBytes, err := ioutil.ReadFile(path + seed.File)
	require.NoError(t, err)

	saved := fixture{}
	err = json.Unmarshal(fileBytes, &saved)
	require.NoError(t, err)

	return saved
}

func writeFixture(t *testing.T, path string, save fixture) {
	fileBytes, err := json.MarshalIndent(save, "", "  ")
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

func getRequest(t *testing.T, endpoint string, params []param) (path string, resp *http.Response, body []byte, reqErr, bodyErr error) {
	verbose := true

	path = fixtestBaseURL + endpoint

	if len(params) > 0 {
		urlValues := url.Values{}
		for _, param := range params {
			urlValues.Add(param.Name, param.Value)
		}
		path += "?" + urlValues.Encode()
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

func generateLiveFixture(t *testing.T, seed fixture) (live fixture) {
	live = fixture{
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

		path, resp, body, reqErr, bodyErr := getRequest(t, seedCase.Request.Path, seedCase.Request.Params)
		require.NoError(t, reqErr, msg)

		// not sure about this one!!!
		require.NoError(t, bodyErr, msg)
		liveCase.Request.URL = path

		liveCase.Response = responseInfo{
			StatusCode: resp.StatusCode,
			Body:       string(body),
		}
		msg += fmt.Sprintf(" newResponse=%+v", liveCase.Response)

		prove, err := seedCase.proverFromEndoint()
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

	return
}

func validateLiveVsSaved(t *testing.T, seed *fixture, live *fixture) {
	require.True(t, live.Frozen, "should be frozen for assertions")
	saved := readFixture(t, fixturesDirectory, seed)

	require.Equal(t, saved.Owner, live.Owner, "unexpected discrepancy in Owner")
	// sanity check:
	require.Equal(t, seed.Owner, saved.Owner, "unexpected discrepancy in Owner")

	require.Equal(t, saved.File, live.File, "unexpected discrepancy in File")
	// sanity check:
	require.Equal(t, seed.File, saved.File, "unexpected discrepancy in File")

	numSeedCases, numSavedCases, numLiveCases := len(seed.Cases), len(saved.Cases), len(live.Cases)
	require.Equal(t, numSavedCases, numLiveCases, "numSavedCases=%d but numLiveCases=%d", numSavedCases, numLiveCases)
	// sanity check:
	require.Equal(t, numSeedCases, numSavedCases, "numSeedCases=%d but numSavedCases=%d", numSeedCases, numSavedCases)

	for i, seedCase := range seed.Cases {
		savedCase, liveCase := saved.Cases[i], live.Cases[i]
		msg := fmt.Sprintf("(%d)[%s]. discrepency in seed=\n%+v\nsaved=\n%+v\nlive=\n%+v\n", i, seedCase.Name, seedCase, savedCase, liveCase)

		require.Equal(t, savedCase.Name, liveCase.Name, msg)
		// sanity check:
		require.Equal(t, seedCase.Name, savedCase.Name, msg)

		// only saved vs live:
		require.Equal(t, savedCase.Request, liveCase.Request, msg)
		require.Equal(t, savedCase.Response, liveCase.Response, msg)

		/* EXPERIMENTAL - PROBLY CAN'T GET RIGHT
		// need to convert witnesses first to same type before comparing
		liveWitness := map[string]interface{}{}
		savedWitness := map[string]interface{}{}
		// err = mapstructure.Decode(liveCase.Witness, &liveWitness)

		liveConfig := &mapstructure.DecoderConfig{
			TagName: "json",
			Result:  &liveWitness,
		}
		decoder, err := mapstructure.NewDecoder(liveConfig)
		require.NoError(t, err, msg)

		err = decoder.Decode(liveCase.Witness)
		require.NoError(t, err, msg)

		err = mapstructure.Decode(savedCase.Witness, &savedWitness)
		require.NoError(t, err, msg)

		dequal := reflect.DeepEqual(savedWitness, liveWitness)
		require.True(t, dequal, msg)
		require.Equal(t, savedWitness, liveWitness, msg)
		// require.Equal(t, savedCase.Witness, liveWitness, msg)
		END OF EXPERIMENTAL */

		prove, err := savedCase.proverFromEndoint()
		require.NoError(t, err, msg)
		require.NotNil(t, prove, msg)

		savedProof, savedErrStr := prove(savedCase.Response)
		liveProof, liveErrStr := prove(liveCase.Response)
		require.Equal(t, savedProof, liveProof, msg)
		require.Equal(t, savedErrStr, liveErrStr, msg)
		// sanity check:
		require.Equal(t, savedCase.WitnessError, liveCase.WitnessError, msg)
		require.Equal(t, savedCase.WitnessError == nil, savedCase.Witness != nil, msg)
	}
}

// When the provided seed has `seed.Frozen == false` assertions will be skipped.
// On the other hand, when `seed.Frozen == false` assertions are made:
// * ownerVariable == saved.Owner == live.Owner
// * saved.File == live.File
// * len(saved.Cases) == len(live.Cases)
// * for each savedCase:
//   * savedCase.Name == liveCase.Name
//   * savedCase.Request == liveCase.Request
//   * recalculated savedCase.Witness == recalculated liveCase.Witness
// Regardless of `seed.Frozen`, `live` is saved to `fixturesDirectory + "_" + seed.File`
// NOTE: `live.Witness` is always recalculated via `seed.proof(live.Response)`
func validateOrGenerateFixtures(t *testing.T, db *postgres.IndexerDb, seed fixture, owner string) {
	require.Equal(t, owner, seed.Owner, "mismatch between purported owners of fixture")

	live := generateLiveFixture(t, seed)

	if seed.Frozen {
		validateLiveVsSaved(t, &seed, &live)
	}
}
