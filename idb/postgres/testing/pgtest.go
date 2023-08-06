package testing

import (
	pgtest "github.com/algorand/indexer/v3/idb/postgres/internal/testing"
)

// SetupPostgres allows setting up postgres instance in integration or e2e tests
var SetupPostgres = pgtest.SetupPostgres
