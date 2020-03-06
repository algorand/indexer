cmd/indexer/indexer:	idb/setup_postgres_sql.go .PHONY
	cd cmd/indexer && go build

idb/setup_postgres_sql.go:	idb/setup_postgres.sql
	cd idb && go generate

mocks:	idb/dummy.go
	cd idb && mockery -name=IndexerDb

test:	mocks
	go test ./...

.PHONY:
