cmd/indexer/indexer:	idb/setup_postgres_sql.go importer/protocols_json.go .PHONY
	cd cmd/indexer && go build

idb/setup_postgres_sql.go:	idb/setup_postgres.sql
	cd idb && go generate

importer/protocols_json.go:	importer/protocols.json
	cd importer && go generate

mocks:	idb/dummy.go
	cd idb && mockery -name=IndexerDb

test:	mocks
	go test ./...

.PHONY:
