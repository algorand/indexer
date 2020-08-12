SRCPATH		:= $(shell pwd)
VERSION		:= $(shell $(SRCPATH)/mule/scripts/compute_build_number.sh)
OS_TYPE		:= $(shell $(SRCPATH)/mule/scripts/ostype.sh)
ARCH		:= $(shell $(SRCPATH)/mule/scripts/archtype.sh)
PKG_DIR		= $(SRCPATH)/tmp/node_pkgs/$(OS_TYPE)/$(ARCH)/$(VERSION)

GOLDFLAGS += -X github.com/algorand/indexer/version.Hash=$(shell git log -n 1 --pretty="%H")
GOLDFLAGS += -X github.com/algorand/indexer/version.Dirty=$(if $(filter $(strip $(shell git status --porcelain|wc -c)), "0"),,true)
GOLDFLAGS += -X github.com/algorand/indexer/version.CompileTime=$(shell date -u +%Y-%m-%dT%H:%M:%S%z)
GOLDFLAGS += -X github.com/algorand/indexer/version.GitDecorateBase64=$(shell git log -n 1 --pretty="%D"|base64)

# This is the default target, build the indexer:
cmd/algorand-indexer/algorand-indexer:	idb/setup_postgres_sql.go types/protocols_json.go .PHONY
	cd cmd/algorand-indexer && CGO_ENABLED=0 go build -ldflags="${GOLDFLAGS}"

idb/setup_postgres_sql.go:	idb/setup_postgres.sql
	cd idb && go generate

types/protocols_json.go:	types/protocols.json types/consensus.go
	cd types && go generate

idb/mocks/IndexerDb.go:	idb/dummy.go
	go get github.com/vektra/mockery/.../
	cd idb && mockery -name=IndexerDb

deploy:
	mule/deploy.sh

package: clean setup
	misc/release.py --outdir $(PKG_DIR)

setup:
	mkdir -p $(PKG_DIR)

sign:
	mule/sign.sh

test: idb/mocks/IndexerDb.go
	go test ./...

test-package:
	mule/e2e.sh

clean:
	rm -rf $(PKG_DIR)

.PHONY:
