package util_test

import (
	"fmt"
	"testing"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pgtest "github.com/algorand/indexer/v3/idb/postgres/internal/testing"
	"github.com/algorand/indexer/v3/idb/postgres/internal/util"
)

func TestTxWithRetry(t *testing.T) {
	count := 3
	f := func(pgx.Tx) error {
		if count == 0 {
			return nil
		}

		count--

		pgerr := pgconn.PgError{
			Code: pgerrcode.SerializationFailure,
		}
		return fmt.Errorf("database error: %w", &pgerr)
	}

	db, _, shutdownFunc := pgtest.SetupPostgres(t)
	defer shutdownFunc()

	err := util.TxWithRetry(db, pgx.TxOptions{}, f, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}
