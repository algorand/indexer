package postgres

import (
	"testing"

	_ "github.com/lib/pq"
)

// TestMaxRoundOnUninitializedDB makes sure we return 0 when getting the max round on a new DB.
func TestM12(t *testing.T) {
	//_, connStr, shutdownFunc := setupPostgres(t)
	//defer shutdownFunc()
}
