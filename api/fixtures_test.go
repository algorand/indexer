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
	"github.com/algorand/go-algorand/rpcs"

	"github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/idb/postgres"
	"github.com/algorand/indexer/util/test"
)

/* See the README.md in this directory for more details about Fixtures Tests */

const fixtestListenAddr = "localhost:8999"
const fixtestBaseURL = "http://" + fixtestListenAddr
const fixtestMaxStartup time.Duration = 5 * time.Second
const fixturesDirectory = "test_resources/"

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

func (f *testCase) proverFromEndoint() (string, prover, error) {
	path := f.Request.Path
	if len(path) == 0 || path[0] != '/' {
		return "", nil, fmt.Errorf("invalid endpoint [%s]", path)
	}
	return getProof(path[1:])
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
func setupIdbAndReturnShutdownFunc(t *testing.T) (db *postgres.IndexerDb, proc func(cert *rpcs.EncodedBlockCert) error, l *ledger.Ledger, shutdown func()) {
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
bodyErr=%v
`, resp, string(body), reqErr, bodyErr)
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

		route, prove, err := seedCase.proverFromEndoint()
		require.NoError(t, err, msg)
		require.Positive(t, len(route), msg)

		liveCase.Request.Route = route

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

		route, prove, err := savedCase.proverFromEndoint()
		require.NoError(t, err, msg)
		require.NotNil(t, prove, msg)
		require.Equal(t, savedCase.Request.Route, route, msg)

		savedProof, savedErrStr := prove(savedCase.Response)
		liveProof, liveErrStr := prove(liveCase.Response)
		require.Equal(t, savedProof, liveProof, msg)
		require.Equal(t, savedErrStr, liveErrStr, msg)
		// sanity check:
		require.Equal(t, savedCase.WitnessError, liveCase.WitnessError, msg)
		require.Equal(t, savedCase.WitnessError == nil, savedCase.Witness != nil, msg)
	}
	// and the saved fixture should be frozen as well before release:
	require.True(t, saved.Frozen, "Please ensure that the saved fixture is frozen before merging.")
}

// When the provided seed has `seed.Frozen == false` assertions will be skipped.
// On the other hand, when `seed.Frozen == false` assertions are made:
// * ownerVariable == saved.Owner == live.Owner
// * saved.File == live.File
// * len(saved.Cases) == len(live.Cases)
// * for each savedCase:
//   - savedCase.Name == liveCase.Name
//   - savedCase.Request == liveCase.Request
//   - recalculated savedCase.Witness == recalculated liveCase.Witness
//
// Regardless of `seed.Frozen`, `live` is saved to `fixturesDirectory + "_" + seed.File`
// NOTE: `live.Witness` is always recalculated via `seed.proof(live.Response)`
// NOTE: by design, the function always fails the test in the case that the seed fixture is not frozen
// as a reminder to freeze the test before merging, so that regressions may be detected going forward.
func validateOrGenerateFixtures(t *testing.T, db *postgres.IndexerDb, seed fixture, owner string) {
	require.Equal(t, owner, seed.Owner, "mismatch between purported owners of fixture")

	live := generateLiveFixture(t, seed)

	require.True(t, seed.Frozen, "To guard against regressions, please ensure that the seed is frozen before merging.")
	validateLiveVsSaved(t, &seed, &live)
}
