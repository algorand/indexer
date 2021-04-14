package postgres

import (
	"testing"

	_ "github.com/lib/pq"

	"github.com/algorand/go-algorand-sdk/types"
	"github.com/algorand/indexer/importer"
	"github.com/algorand/indexer/util/test"
	"github.com/stretchr/testify/assert"
)

func TestMigration_FixFreezeLookupMigration(t *testing.T) {
	db, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	pdb, err := OpenPostgres(connStr, nil, nil)
	assert.NoError(t, err)
	blockImporter := importer.NewDBImporter(pdb)

	var sender types.Address
	var faddr types.Address

	sender[0] = 0x01
	faddr[0] = 0x02

	///////////
	// Given // A block containing an asset freeze txn has been imported.
	///////////
	freeze, _ := test.MakeAssetFreezeOrPanic(test.Round, 1234, true, sender, faddr)
	block := test.MakeBlockForTxns(freeze)
	txnCount, err := blockImporter.ImportDecodedBlock(&block)
	assert.NoError(t, err, "failed to import")
	assert.Equal(t, 1, txnCount)

	//////////
	// When // We truncate the txn_participation table and run our migration
	//////////
	db.Exec("TRUNCATE txn_participation")
	FixFreezeLookupMigration(pdb, &MigrationState{NextMigration: 12})

	//////////
	// Then // The sender is still deleted, but the freeze addr should be back.
	//////////
	senderCount := queryCount(db, "SELECT COUNT(*) FROM txn_participation WHERE addr = $1", sender[:])
	faddrCount := queryCount(db, "SELECT COUNT(*) FROM txn_participation WHERE addr = $1", faddr[:])
	assert.Equal(t, 0, senderCount)
	assert.Equal(t, 1, faddrCount)
}
