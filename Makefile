SRCPATH		:= $(shell pwd)
VERSION		:= $(shell $(SRCPATH)/mule/scripts/compute_build_number.sh)
OS_TYPE		?= $(shell $(SRCPATH)/mule/scripts/ostype.sh)
ARCH		?= $(shell $(SRCPATH)/mule/scripts/archtype.sh)
PKG_DIR		= $(SRCPATH)/tmp/node_pkgs/$(OS_TYPE)/$(ARCH)/$(VERSION)
ifeq ($(OS_TYPE), darwin)
ifeq ($(ARCH), arm64)
export CPATH=/opt/homebrew/include
export LIBRARY_PATH=/opt/homebrew/lib
endif
endif

# TODO: ensure any additions here are mirrored in misc/release.py
GOLDFLAGS += -X github.com/algorand/indexer/version.Hash=$(shell git log -n 1 --pretty="%H")
GOLDFLAGS += -X github.com/algorand/indexer/version.Dirty=$(if $(filter $(strip $(shell git status --porcelain|wc -c)), "0"),,true)
GOLDFLAGS += -X github.com/algorand/indexer/version.CompileTime=$(shell date -u +%Y-%m-%dT%H:%M:%S%z)
GOLDFLAGS += -X github.com/algorand/indexer/version.GitDecorateBase64=$(shell git log -n 1 --pretty="%D"|base64|tr -d ' \n')
GOLDFLAGS += -X github.com/algorand/indexer/version.ReleaseVersion=$(shell cat .version)

COVERPKG := $(shell go list ./...  | grep -v '/cmd/' | egrep -v '(testing|test|mocks)$$' |  paste -s -d, - )

# Used for e2e test
export GO_IMAGE = golang:$(shell go version | cut -d ' ' -f 3 | tail -c +3 )

# This is the default target, build the indexer:
cmd/algorand-indexer/algorand-indexer: idb/postgres/internal/schema/setup_postgres_sql.go go-algorand
	cd cmd/algorand-indexer && go build -ldflags="${GOLDFLAGS}"

go-algorand:
	git submodule update --init && cd third_party/go-algorand && \
		make crypto/libs/`scripts/ostype.sh`/`scripts/archtype.sh`/lib/libsodium.a

idb/postgres/internal/schema/setup_postgres_sql.go:	idb/postgres/internal/schema/setup_postgres.sql
	cd idb/postgres/internal/schema && go generate

idb/mocks/IndexerDb.go:	idb/idb.go
	go install github.com/vektra/mockery/v2@v2.12.1
	cd idb && mockery --name=IndexerDb

# check that all packages (except tests) compile
check: go-algorand
	go build ./...

package: go-algorand
	rm -rf $(PKG_DIR)
	mkdir -p $(PKG_DIR)
	misc/release.py --host-only --outdir $(PKG_DIR)

# used in travis test builds; doesn't verify that tag and .version match
fakepackage: go-algorand
	rm -rf $(PKG_DIR)
	mkdir -p $(PKG_DIR)
	misc/release.py --host-only --outdir $(PKG_DIR) --fake-release

test: idb/mocks/IndexerDb.go cmd/algorand-indexer/algorand-indexer
	go test -coverpkg=$(COVERPKG) ./... -coverprofile=coverage.txt -covermode=atomic ${TEST_FLAG}

lint: go-algorand
	golangci-lint run -c .golangci.yml
	go vet ./...

fmt:
	go fmt ./...

# These are completely broken after the go-algoroand change which starts the
# TxnCounter at 1000. Because these tests have already been deprecated and
# replaced in the develop branch, we have opted to disable them instead of
# fixing them.
#integration: cmd/algorand-indexer/algorand-indexer
#	mkdir -p test/blockdata
#	curl -s https://algorand-testdata.s3.amazonaws.com/indexer/test_blockdata/create_destroy.tar.bz2 -o test/blockdata/create_destroy.tar.bz2
#	test/postgres_integration_test.sh

e2e: cmd/algorand-indexer/algorand-indexer
	cd misc && docker-compose build --build-arg GO_IMAGE=${GO_IMAGE} && docker-compose up --exit-code-from e2e

deploy:
	mule/deploy.sh

sign:
	mule/sign.sh

test-package:
	mule/e2e.sh

test-generate:
	test/test_generate.py

nightly-setup:
	cd third_party/go-algorand && git fetch && git reset --hard origin/master

nightly-teardown:
	git submodule update

indexer-v-algod-swagger:
	pytest -sv misc/parity

indexer-v-algod: nightly-setup indexer-v-algod-swagger nightly-teardown

# fetch and update submodule. it's default to latest rel/nightly branch.
# to use a different branch, update the branch in .gitmodules for CI build,
# and for local testing, you may checkout a specific branch in the submodule.
# after submodule is updated, CI_E2E_FILE in circleci/config.yml should also
# be updated to use a newer artifact. path copied from s3 bucket, s3://algorand-testdata/indexer/e2e4/
update-submodule:
	git submodule update --remote

.PHONY: test e2e integration fmt lint deploy sign test-package package fakepackage cmd/algorand-indexer/algorand-indexer idb/mocks/IndexerDb.go go-algorand indexer-v-algod
