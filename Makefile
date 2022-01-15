SRCPATH		:= $(shell pwd)
VERSION		:= $(shell $(SRCPATH)/mule/scripts/compute_build_number.sh)
OS_TYPE		?= $(shell $(SRCPATH)/mule/scripts/ostype.sh)
ARCH			?= $(shell $(SRCPATH)/mule/scripts/archtype.sh)
PKG_DIR		= $(SRCPATH)/tmp/node_pkgs/$(OS_TYPE)/$(ARCH)/$(VERSION)

# TODO: ensure any additions here are mirrored in misc/release.py
GOLDFLAGS += -X github.com/algorand/indexer/version.Hash=$(shell git log -n 1 --pretty="%H")
GOLDFLAGS += -X github.com/algorand/indexer/version.Dirty=$(if $(filter $(strip $(shell git status --porcelain|wc -c)), "0"),,true)
GOLDFLAGS += -X github.com/algorand/indexer/version.CompileTime=$(shell date -u +%Y-%m-%dT%H:%M:%S%z)
GOLDFLAGS += -X github.com/algorand/indexer/version.GitDecorateBase64=$(shell git log -n 1 --pretty="%D"|base64|tr -d ' \n')
GOLDFLAGS += -X github.com/algorand/indexer/version.ReleaseVersion=$(shell cat .version)

# Used for e2e test
export GO_IMAGE = golang:$(shell go version | cut -d ' ' -f 3 | tail -c +3 )

# This is the default target, build the indexer:
cmd/algorand-indexer/algorand-indexer: idb/postgres/internal/schema/setup_postgres_sql.go go-algorand
	cd cmd/algorand-indexer && go build -ldflags="${GOLDFLAGS}"

# TODO: allow specifying the branch
go-algorand-submodule:
	git submodule update --init --force --remote

go-algorand-build:
	cd third_party/go-algorand && \
		make crypto/libs/`scripts/ostype.sh`/`scripts/archtype.sh`/lib/libsodium.a

go-algorand: go-algorand-submodule go-algorand-build

idb/postgres/internal/schema/setup_postgres_sql.go:	idb/postgres/internal/schema/setup_postgres.sql
	cd idb/postgres/internal/schema && go generate

idb/mocks/IndexerDb.go:	idb/idb.go
	go get github.com/vektra/mockery/.../
	cd idb && mockery -name=IndexerDb

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
	go test ./... -coverprofile=coverage.txt -covermode=atomic

lint: go-algorand
	golint -set_exit_status ./...
	go vet -mod=mod ./...

fmt:
	go fmt ./...

integration: cmd/algorand-indexer/algorand-indexer
	mkdir -p test/blockdata
	curl -s https://algorand-testdata.s3.amazonaws.com/indexer/test_blockdata/create_destroy.tar.bz2 -o test/blockdata/create_destroy.tar.bz2
	test/postgres_integration_test.sh

e2e: cmd/algorand-indexer/algorand-indexer
	cd misc && docker-compose build --build-arg GO_IMAGE=${GO_IMAGE} && docker-compose up --exit-code-from e2e

deploy:
	mule/deploy.sh

sign:
	mule/sign.sh

test-package:
	mule/e2e.sh

.PHONY: test e2e integration fmt lint deploy sign test-package package fakepackage cmd/algorand-indexer/algorand-indexer idb/mocks/IndexerDb.go go-algorand
