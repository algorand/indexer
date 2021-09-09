package util

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgerrcode"
	log "github.com/sirupsen/logrus"

	"github.com/algorand/indexer/idb"
)

// TxWithRetry is a helper function that retries the function `f` in case the database
// transaction in it fails due to a serialization error. `f` is provided
// a transaction created using `opts`. `f` takes ownership of the
// transaction and must either call sql.Tx.Rollback() or sql.Tx.Commit(). In the second
// case, `f` must return an error which contains the error returned by sql.Tx.Commit().
// The easiest way is to just return the result of sql.Tx.Commit().
func TxWithRetry(db *sql.DB, opts sql.TxOptions, f func(*sql.Tx) error, log *log.Logger) error {
	count := 0
	for {
		tx, err := db.BeginTx(context.Background(), &opts)
		if err != nil {
			return err
		}

		err = f(tx)

		// If not serialization error.
		var pgerr *pgconn.PgError
		if !errors.As(err, &pgerr) || (pgerr.Code != pgerrcode.SerializationFailure) {
			if (count > 0) && (log != nil) {
				log.Printf("transaction was retried %d times", count)
			}
			return err
		}

		count++
		if log != nil {
			log.Printf("retrying transaction, count: %d", count)
		}
	}
}

// GetMetastate returns `idb.ErrorNotInitialized` if uninitialized.
// If `tx` is nil, it uses a normal query.
func GetMetastate(ctx context.Context, db *sql.DB, tx *sql.Tx, key string) (string, error) {
	query := `SELECT v FROM metastate WHERE k = $1`

	var row *sql.Row
	if tx == nil {
		row = db.QueryRowContext(ctx, query, key)
	} else {
		row = tx.QueryRowContext(ctx, query, key)
	}

	var value string
	err := row.Scan(&value)
	if err == sql.ErrNoRows {
		return "", idb.ErrorNotInitialized
	}
	if err != nil {
		return "", fmt.Errorf("getMetastate() err: %w", err)
	}

	return value, nil
}
