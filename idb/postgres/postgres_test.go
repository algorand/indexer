package postgres

import (
	"fmt"
	"github.com/algorand/indexer/idb"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAllMigrations(t *testing.T) {
	for idx, m := range migrations {
		t.Run(fmt.Sprintf("Test migration %d", idx), func(t *testing.T) {
			db := MakeMockDB([]*MockStmt{
				// "state"
				MakeMockStmt(
					1,
					[]string{"v"},
					[][]interface{}{
						{`{"account_round": 9000000}`},
					}),
				// "migration"
				MakeMockStmt(
					1,
					[]string{"v"},
					[][]interface{}{
						{fmt.Sprintf(`{"next": %d}`, idx)},
					}),
			})

			// This automatically runs migraions
			pdb, err := openPostgres(db, &idb.IndexerDbOptions{
				ReadOnly: false,
			})
			require.NoError(t, err)

			h, err := pdb.Health()
			// Health attempts to get num rows...
			require.Error(t, err, "not enough statements loaded into mock driver")

			// There should be an error because I'm not attempting to mock the migration code.
			//require.Contains(t, err.Error(), fmt.Sprintf("error during migration %d", idx))
			str := fmt.Sprintf("error during migration %d (%s)", idx, m.description)
			require.Contains(t, (*h.Data)["migration-error"], str)
			require.Contains(t, (*h.Data)["migration-status"], str)
		})
	}
}

func TestNoMigrationsNeeded(t *testing.T) {
	db := MakeMockDB([]*MockStmt{
		// "state"
		MakeMockStmt(
			1,
			[]string{"v"},
			[][]interface{}{
				{`{"account_round": 9000000}`},
			}),
		// "migration"
		MakeMockStmt(
			1,
			[]string{"v"},
			[][]interface{}{
				{fmt.Sprintf(`{"next": %d}`, len(migrations) + 1)},
			}),
	})

	// This automatically runs migraions
	pdb, err := openPostgres(db, &idb.IndexerDbOptions{
		ReadOnly: false,
	})

	h, err := pdb.Health()
	// Health attempts to get num rows...
	require.Error(t, err, "not enough statements loaded into mock driver")

	require.Equal(t, (*h.Data)["migration-status"], "Migrations Complete")
}
