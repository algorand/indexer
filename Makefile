SRCPATH		:= $(shell pwd)
OS_TYPE		:= $(shell ./scripts/ostype.sh)
   ARCH		:= $(shell ./scripts/archtype.sh)
PKG_DIR		= $(SRCPATH)/tmp/node_pkgs/$(OS_TYPE)/$(ARCH)

clean:
	rm -rf $(PKG_DIR)

# This is the default target, build the indexer:
cmd/algorand-indexer/algorand-indexer:	idb/setup_postgres_sql.go importer/protocols_json.go .PHONY
	cd cmd/algorand-indexer && CGO_ENABLED=0 go build

idb/setup_postgres_sql.go:	idb/setup_postgres.sql
	cd idb && go generate

importer/protocols_json.go:	importer/protocols.json
	cd importer && go generate

mocks:	idb/dummy.go
	cd idb && mockery -name=IndexerDb

package: clean setup
	misc/release.py --outdir $(PKG_DIR)

setup:
	mkdir -p $(PKG_DIR)

test:	mocks
	go test ./...

.PHONY:

###### TARGETS FOR CICD PROCESS ######
include ./mule/Makefile.mule

