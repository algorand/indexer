# API Design

We are using a documentation driven process.

The API is defined using [OpenAPI v2](https://swagger.io/specification/v2/) in **indexer.oas2.yml**.

## Updating REST API

The Makefile will install our fork of **oapi-codegen**, use `make oapi-codegen` to install it directly.

1. Document your changes by editing **indexer.oas2.yml**
2. Regenerate the endpoints by running **generate.sh**. The sources at **generated/** will be updated.
3. Update the implementation in **handlers.go**. It is sometimes useful to consult **generated/routes.go** to make sure the handler properly implements **ServerInterface**.

## What codegen tool is used?

We found that [oapi-codegen](https://github.com/deepmap/oapi-codegen) produced the cleanest code, and had an easy to work with codebase. 
There is an algorand fork of this project which contains a couple modifications that were needed to properly support our needs.

Specifically, `uint64` types aren't strictly supported by OpenAPI. So we added a type-mapping feature to oapi-codegen.

## Why do we have indexer.oas2.yml and indexer.oas3.yml?

We chose to maintain V2 and V3 versions of the spec because OpenAPI v3 doesn't seem to be widely supported. Some tools worked better with V3 and others with V2, so having both available has been useful.
To reduce developer burdon, the v2 specfile is automatically converted v3 using [converter.swagger.io](http://converter.swagger.io/).

## Fixtures Test

### What is a **Fixtures Test**?

Currently (September 2022) [fixtures_test.go](./fixtures_test.go) is a library that allows testing Indexer's router to verify that endpoints accept parameters and respond as expected,
and guard against future regressions. [app_boxes_fixtures_test.go](./app_boxes_fixtures_test.go) is an example _fixtures test_ and is the _creator_ of the fixture [boxes.json](./test_resources/boxes.json).

A fixtures test

1. is defined by a go-slice called a _Seed Fixture_  e.g. [var boxSeedFixture](https://github.com/algorand/indexer/blob/b5025ad640fabac0d778b4cac60d558a698ed560/api/app_boxes_fixtures_test.go#L302-L692)
which contains request information for making HTTP requests against an Indexer server
2. iterates through the slice, making each of the defined requests and generating a _Live Fixture_
3. reads a _Saved Fixture_ from a json file e.g. [boxes.json](./test_resources/boxes.json)
4. persists the _Live Fixture_ to a json file not in source control
5. asserts that the _Saved Fixture_ is equal to the _Live Fixture_

In reality, because we always want to save the _Live Fixture_ before making assertions that could fail the test and pre-empt saving, steps (3) and (4) happen in the opposite order.

### What's the purpose of a Fixtures Test?

A fixtures test should allow one to quickly stand up an end-to-end test to validate that Indexer endpoints are working as expected. After Indexer's state is programmatically set up, 
it's easy to add new requests and verify that the responses look exactly as expected. Once you're satisfied that the responses are correct, it's easy to _freeze_ the test and guard against future regressions.

### What does a **Fixtures Test Function** Look Like?

[func TestBoxes](https://github.com/algorand/indexer/blob/b5025ad640fabac0d778b4cac60d558a698ed560/api/app_boxes_fixtures_test.go#L694_L704) shows the basic structure of a fixtures test.

1. `setupIdbAndReturnShutdownFunc()` is called to set up the Indexer database

   * this isn't expected to require modification

2. `setupLiveBoxes()` is used to prepare the local ledger and process blocks in order to bring Indexer into a particular state
   * this will always depend on what the test is trying to achieve
   * in this case, an app was used to create and modify a set of boxes which are then queried against
   * it is conceivable that instead of bringing Indexer into a particular state, the responses from the DB or even the handler may be mocked, so we could have had `setupLiveBoxesMocker()` instead of `setupLiveBoxes()`

3. `setupLiveServerAndReturnShutdownFunc()` is used to bring up an instance of a real Indexer.

   * this shouldn't need to be modified; however, if running in parallel and making assertions that conflict with other tests,
   you may need to localize the variable `fixtestListenAddr` and run on a separate port
   * if running a mock server instead, a different setup function would be needed

4. `validateLiveVsSaved()` runs steps (1) through (5) defined in the previous section
  
   * this is designed to be generic and ought not require much modification going forward

### Which Endpoints are Currently _Testable_ in a Fixtures Test?

Endpoints defined in [proverRoutes](https://github.com/algorand/indexer/blob/b955a31b10d8dce7177383895ed8e57206d69f67/api/fixtures_test.go#L232-L263) are testable.

Currently (September 2022) these are:

* `/v2/accounts`
* `/v2/applications`
* `/v2/applications/:application-id`
* `/v2/applications/:application-id/box`
* `/v2/applications/:application-id/boxes`

### How to Introduce a New Fixtures Test for an _Already Testable_ Endpoint?

To set up a new test for endpoints defined above one needs to:

#### 1. Define a new _Seed Fixture_

For example, consider

```go
var boxSeedFixture = fixture{
	File:   "boxes.json",
	Owner:  "TestBoxes",
	Frozen: true,
	Cases: []testCase{
		// /v2/accounts - 1 case
		{
			Name: "What are all the accounts?",
			Request: requestInfo{
				Path:   "/v2/accounts",
				Params: []param{},
			},
		},
        ...
```

A seed fixture is a `struct` with fields

* `File` (_required_) - the name in [test_resources](./test_resources/) where the fixture is read from (and written to with an `_` prefix)
* `Owner` (_recommended_) - a name to define which test "owns" the seed
* `Frozen` (_required_) - set _true_ when you need to run assertions of the _Live Fixture_ vs. the _Saved Fixture_. For tests to pass, it needs to be set _true_.
* `Cases` - the slice of `testCase`s. Each of these has the fields:

  * `Name` (_required_) - an identifier for the test case
  * `Request` (_required_) - a `requestInfo` struct specifying:
    * `Path` (_required_) - the path to be queried
    * `Params` (_required but may be empty_) - the slice of parameters (strings `name` and `value`) to be appended to the path

#### 2. Define a new _Indexer State_ Setup Function

There are many examples of setting up state that can be emulated. For example:

* [setupLiveBoxes()](https://github.com/algorand/indexer/blob/b5025ad640fabac0d778b4cac60d558a698ed560/api/app_boxes_fixtures_test.go#L43) for application boxes
* [TestApplicationHandlers()](https://github.com/algorand/indexer/blob/3a9095c2b5ee25093708f980445611a03f2cf4e2/api/handlers_e2e_test.go#L93) for applications
* [TestBlockWithTransactions()](https://github.com/algorand/indexer/blob/800cb135a0c6da0109e7282acf85cbe1961930c6/idb/postgres/postgres_integration_test.go#L339)
setup state consisting of a set of basic transactions

### How to Make a _New Endpoint_ Testable by Fixtures Tests?

There are 2 steps:

1. Implement a new function _witness generator_ aka [prover function](https://github.com/algorand/indexer/blob/b955a31b10d8dce7177383895ed8e57206d69f67/api/fixtures_test.go#L103) of
type `func(responseInfo) (interface{}, *string)` as examplified in [this section](https://github.com/algorand/indexer/blob/b955a31b10d8dce7177383895ed8e57206d69f67/api/fixtures_test.go#L107-L200).
Such a function is supposed to parse an Indexer response's body into a generated model. Currently, all provers are boilerplate, and with generics, it's expected that this step will no longer be necessary
(this [POC](https://github.com/tzaffi/indexer/blob/generic-boxes/api/fixtures_test.go#L119-L155) shows how it would be done with generics).
2. Define a new route in the [proverRoutes struct](https://github.com/algorand/indexer/blob/b955a31b10d8dce7177383895ed8e57206d69f67/api/fixtures_test.go#L232_L263).
This is a tree structure which is traversed by splitting a path using `/` and eventually reaching a leaf which consists of a `prover` as defined in #1. 

For example, to enable the endpoint `GET /v2/applications/{application-id}/logs` for fixtures test, one need only define a `logsProof` witness generator and have it mapped in `proverRoutes` under:

```go
proverRoutes.parts["v2"].parts["applications"].parts[":application-id"].parts["logs"] = logsProof
```

### How to Fix a Fixtures Test?

Supposing that there was a breaking change upstream, and a fixtures test is now failing. The following approach should work most of the time:

1. Run the broken test generating a temporary fixture file in the `fixturesDirectory` (currently [test_resources](./test_resources/)) with a name the same as the json fixture except begining with `_`
(e.g. `_boxes.json` vs. `boxes.json`).
2. Observe the diff between the temporary fixture and the saved fixture. If the diff is acceptable, then simply copy the temporary fixture over the saved fixture.
3. If the diff isn't acceptable, then make any necessary changes to the setup and seed and repeat steps 1 and 2.
