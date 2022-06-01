package local_ledger_test

import (
	"testing"

	"github.com/algorand/indexer/idb/postgres/internal/encoding"
	ledger "github.com/algorand/indexer/idb/postgres/internal/migrations/local_ledger"
	"github.com/algorand/indexer/idb/postgres/internal/schema"
	pgtest "github.com/algorand/indexer/idb/postgres/internal/testing"
	"github.com/algorand/indexer/idb/postgres/internal/types"
	pgutil "github.com/algorand/indexer/idb/postgres/internal/util"
	"github.com/stretchr/testify/assert"
)

func TestRunMigration(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()
	state := &types.ImportState{
		NextRoundToAccount: 1000,
	}
	err := pgutil.SetMetastate(db, nil, schema.StateMetastateKey, string(encoding.EncodeImportState(state)))
	assert.NoError(t, err)
	ledger.RunMigration(1000, nil)
}
