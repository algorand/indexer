package postgres

import (
	"fmt"
	"testing"
	"time"

	"github.com/algorand/indexer/idb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllMigrations(t *testing.T) {
	for idx, m := range migrations {
		t.Run(fmt.Sprintf("Test migration %d", idx), func(t *testing.T) {
			db := MakeMockDB([]*MockStmt{
				// INFORMATION_SCHEMA.TABLES
				MakeMockStmt(
					0,
					[]string{"columnname"},
					[][]interface{}{
						{0},
					}),
				// migration state
				MakeMockStmt(
					1,
					[]string{"v"},
					[][]interface{}{
						{fmt.Sprintf(`{"next": %d}`, idx)},
					}),
			})

			// This automatically runs migrations
			pdb, _, err := openPostgres(db, idb.IndexerDbOptions{
				ReadOnly: false,
			}, nil)
			require.NoError(t, err)

			// Just need a moment for the go routine to get started
			time.Sleep(100 * time.Millisecond)

			h, err := pdb.Health()
			fmt.Printf("%v\n", h)
			// Health attempts to get num rows...
			require.Error(t, err, "not enough statements loaded into mock driver")

			// There should be an error because I'm not attempting to mock the migration code.
			//require.Contains(t, err.Error(), fmt.Sprintf("error during migration %d", idx))
			str := fmt.Sprintf("error during migration %d (%s)", idx, m.description)
			require.Contains(t, h.Error, str)
			require.Contains(t, (*h.Data)["migration-status"], str)
		})
	}
}

func TestNoMigrationsNeeded(t *testing.T) {
	db := MakeMockDB([]*MockStmt{
		// INFORMATION_SCHEMA.TABLES
		MakeMockStmt(
			0,
			[]string{"columnname"},
			[][]interface{}{
				{0},
			}),
		// migration state
		MakeMockStmt(
			1,
			[]string{"v"},
			[][]interface{}{
				{fmt.Sprintf(`{"next": %d}`, len(migrations))},
			}),
	})

	// This automatically runs migraions
	pdb, _, err := openPostgres(db, idb.IndexerDbOptions{
		ReadOnly: false,
	}, nil)
	assert.NoError(t, err)

	// Just need a moment for the go routine to get started
	time.Sleep(100 * time.Millisecond)

	h, err := pdb.Health()
	// Health attempts to get num rows...
	require.Error(t, err, "not enough statements loaded into mock driver")

	require.Equal(t, (*h.Data)["migration-status"], "Migrations Complete")
}

func TestTealKeyValue(t *testing.T) {
	a := require.New(t)

	k1 := []byte("key1")
	k2 := []byte("key2")

	var tkv TealKeyValue
	_, ok := tkv.get(k1)
	a.False(ok)

	tkv.put(k1, TealValue{})
	_, ok = tkv.get(k1)
	a.True(ok)

	tkv.put(k2, TealValue{})
	_, ok = tkv.get(k2)
	a.True(ok)

	tkv.delete(k1)
	_, ok = tkv.get(k1)
	a.False(ok)

	tkv.delete(k2)
	_, ok = tkv.get(k2)
	a.False(ok)
}
