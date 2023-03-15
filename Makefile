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
export GOPATH := $(shell go env GOPATH)
GOPATH1 := $(firstword $(subst :, ,$(GOPATH)))

# TODO: ensure any additions here are mirrored in misc/release.py
GOLDFLAGS += -X github.com/algorand/indexer/version.Hash=$(shell git log -n 1 --pretty="%H")
GOLDFLAGS += -X github.com/algorand/indexer/version.Dirty=$(if $(filter $(strip $(shell git status --porcelain|wc -c)), "0"),,true)
GOLDFLAGS += -X github.com/algorand/indexer/version.CompileTime=$(shell date -u +%Y-%m-%dT%H:%M:%S%z)
GOLDFLAGS += -X github.com/algorand/indexer/version.GitDecorateBase64=$(shell git log -n 1 --pretty="%D"|base64|tr -d ' \n')
GOLDFLAGS += -X github.com/algorand/indexer/version.ReleaseVersion=$(shell cat .version)

COVERPKG := $(shell go list ./...  | grep -v '/cmd/' | egrep -v '(testing|test|mocks)$$' |  paste -s -d, - )

# Used for e2e test
export GO_IMAGE = golang:$(shell go version | cut -d ' ' -f 3 | tail -c +3 )

# This is the default target, build everything:
all: cmd/algorand-indexer/algorand-indexer idb/postgres/internal/schema/setup_postgres_sql.go idb/mocks/IndexerDb.go


cmd/algorand-indexer/algorand-indexer: idb/postgres/internal/schema/setup_postgres_sql.go
	cd cmd/algorand-indexer && go build -ldflags="${GOLDFLAGS}"

idb/postgres/internal/schema/setup_postgres_sql.go:	idb/postgres/internal/schema/setup_postgres.sql
	cd idb/postgres/internal/schema && go generate

idb/mocks/IndexerDb.go:	idb/idb.go
	go install github.com/vektra/mockery/v2@v2.12.1
	cd idb && mockery --name=IndexerDb

# check that all packages (except tests) compile
check:
	go build ./...

package:
	rm -rf $(PKG_DIR)
	mkdir -p $(PKG_DIR)
	misc/release.py --host-only --outdir $(PKG_DIR)

# used in travis test builds; doesn't verify that tag and .version match
fakepackage:
	rm -rf $(PKG_DIR)
	mkdir -p $(PKG_DIR)
	misc/release.py --host-only --outdir $(PKG_DIR) --fake-release

test: idb/mocks/IndexerDb.go cmd/algorand-indexer/algorand-indexer
	go test -coverpkg=$(COVERPKG) ./... -coverprofile=coverage.txt -covermode=atomic ${TEST_FLAG}

lint:
	golangci-lint run -c .golangci.yml
	go vet ./...

fmt:
	go fmt ./...

# note: when running e2e tests manually be sure to set the e2e filename:
# 	'export CI_E2E_FILENAME=rel-nightly'
# To keep the container running at exit set 'export EXTRA="--keep-alive"',
# once the container is paused use 'docker exec <id> bash' to inspect temp
# files in `/tmp/*/'
e2e: cmd/algorand-indexer/algorand-indexer
	cd e2e_tests/docker/indexer/ && docker-compose build --build-arg GO_IMAGE=${GO_IMAGE} && docker-compose up --exit-code-from e2e

e2e-filter-test: cmd/algorand-indexer/algorand-indexer conduit
	cd e2e_tests/docker/indexer-filtered/ && docker-compose build --build-arg GO_IMAGE=${GO_IMAGE} && docker-compose up --exit-code-from e2e-read

e2e-filter-test-nightly: cmd/algorand-indexer/algorand-indexer conduit
	cd e2e_tests/docker/indexer-filtered/ && docker-compose build --build-arg GO_IMAGE=${GO_IMAGE} --build-arg CHANNEL=nightly && docker-compose up --exit-code-from e2e-read

e2e-nightly: cmd/algorand-indexer/algorand-indexer
	cd e2e_tests/docker/indexer/ && docker-compose build --build-arg GO_IMAGE=${GO_IMAGE} --build-arg CHANNEL=nightly && docker-compose up --exit-code-from e2e

deploy:
	mule/deploy.sh

sign:
	mule/sign.sh

test-package:
	mule/e2e.sh

test-generate:
	test/test_generate.py

indexer-v-algod:
	pytest -sv misc/parity

.PHONY: all test e2e integration fmt lint deploy sign test-package package fakepackage cmd/algorand-indexer/algorand-indexer idb/mocks/IndexerDb.go indexer-v-algod
