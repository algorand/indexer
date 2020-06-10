SRCPATH		:= $(shell pwd)
VERSION		:= $(shell $(SRCPATH)/mule/scripts/compute_build_number.sh)
OS_TYPE		:= $(shell $(SRCPATH)/mule/scripts/ostype.sh)
ARCH		:= $(shell $(SRCPATH)/mule/scripts/archtype.sh)
PKG_DIR		= $(SRCPATH)/tmp/node_pkgs/$(OS_TYPE)/$(ARCH)/$(VERSION)

# This is the default target, build the indexer:
cmd/algorand-indexer/algorand-indexer:	idb/setup_postgres_sql.go importer/protocols_json.go .PHONY
	cd cmd/algorand-indexer && CGO_ENABLED=0 go build

idb/setup_postgres_sql.go:	idb/setup_postgres.sql
	cd idb && go generate

importer/protocols_json.go:	importer/protocols.json
	cd importer && go generate

mocks:	idb/dummy.go
	cd idb && mockery -name=IndexerDb

deploy:
	mule/deploy.sh

generate-releases-page:
	mule/generate_releases_page.sh

package: clean setup
	misc/release.py --outdir $(PKG_DIR)

setup:
	mkdir -p $(PKG_DIR)

sign:
	mule/sign.sh

test: mocks
	go test ./...

test-package:
	mule/e2e.sh

generate-releases-page:
	mule/generate_releases_page.sh

clean:
	rm -rf $(PKG_DIR)

.PHONY:

