SRCPATH		:= $(shell pwd)
VERSION		:= $(shell $(SRCPATH)/scripts/compute_build_number.sh)
PKG_DIR		= $(SRCPATH)/packages/$(VERSION)

# This is the default target, build the indexer:
cmd/algorand-indexer/algorand-indexer:	idb/setup_postgres_sql.go importer/protocols_json.go .PHONY
	cd cmd/algorand-indexer && CGO_ENABLED=0 go build

idb/setup_postgres_sql.go:	idb/setup_postgres.sql
	cd idb && go generate

importer/protocols_json.go:	importer/protocols.json
	cd importer && go generate

mocks:	idb/dummy.go
	cd idb && mockery -name=IndexerDb

generate-releases-page:
	build/generate_releases_page.sh

package: clean setup
	misc/release.py --outdir $(PKG_DIR)

setup:
	mkdir -p $(PKG_DIR)

sign:
	build/sign.sh

stage-packages:
	aws s3 sync ./packages/$(VERSION) s3://algorand-staging/indexer/$(VERSION)

test: mocks
	go test ./...

test-package:
	build/e2e.sh

clean:
	rm -rf $(PKG_DIR)

.PHONY:

