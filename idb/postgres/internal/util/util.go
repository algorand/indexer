package util

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	log "github.com/sirupsen/logrus"

	"github.com/algorand/indexer/idb"
)

// TxWithRetry is a helper function that retries the function `f` in case the database
// transaction in it fails due to a serialization error. `f` is provided
// a transaction created using `opts`. `f` should either return an error in which case
// the transaction is rolled back and `TxWithRetry` terminates, or nil in which case
// the transaction attempts to be committed.
func TxWithRetry(db *pgxpool.Pool, opts pgx.TxOptions, f func(pgx.Tx) error, log *log.Logger) error {
	count := 0
	for {
		tx, err := db.BeginTx(context.Background(), opts)
		if err != nil {
			return fmt.Errorf("TxWithRetry() begin tx err: %w", err)
		}

		err = f(tx)
		if err != nil {
			tx.Rollback(context.Background())
			return fmt.Errorf("TxWithRetry() err: %w", err)
		}

		err = tx.Commit(context.Background())
		if err == nil {
			return nil
		}
		// If not serialization error.
		var pgerr *pgconn.PgError
		if !errors.As(err, &pgerr) || (pgerr.Code != pgerrcode.SerializationFailure) {
			if (count > 0) && (log != nil) {
				log.Printf("transaction was retried %d times", count)
			}
			return fmt.Errorf("TxWithRetry() commit tx err: %w", err)
		}

		count++
		if log != nil {
			log.Printf("retrying transaction, count: %d", count)
		}
	}
}

// GetMetastate returns `idb.ErrorNotInitialized` if uninitialized.
// If `tx` is nil, it uses a normal query.
func GetMetastate(ctx context.Context, db *pgxpool.Pool, tx pgx.Tx, key string) (string, error) {
	query := `SELECT v FROM metastate WHERE k = $1`

	var row pgx.Row
	if tx == nil {
		row = db.QueryRow(ctx, query, key)
	} else {
		row = tx.QueryRow(ctx, query, key)
	}

	var value string
	err := row.Scan(&value)
	if err == pgx.ErrNoRows {
		return "", idb.ErrorNotInitialized
	}
	if err != nil {
		return "", fmt.Errorf("getMetastate() err: %w", err)
	}

	return value, nil
}
