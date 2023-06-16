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

	"github.com/algorand/indexer/v3/idb"
)

func isSerializationError(err error) bool {
	var pgerr *pgconn.PgError
	return errors.As(err, &pgerr) && (pgerr.Code == pgerrcode.SerializationFailure)
}

func attemptTx(tx pgx.Tx, f func(pgx.Tx) error) error {
	defer tx.Rollback(context.Background())

	err := f(tx)
	if err != nil {
		return fmt.Errorf("attemptTx() err: %w", err)
	}

	err = tx.Commit(context.Background())
	if err != nil {
		return fmt.Errorf("attemptTx() commit err: %w", err)
	}

	return nil
}

// TxWithRetry is a helper function that retries the function `f` in case the database
// transaction in it fails due to a serialization error. `f` is provided
// a transaction created using `opts`. If `f` experiences a database error, this error
// must be included in `f`'s return error's chain, so that a serialization error can be
// detected.
func TxWithRetry(db *pgxpool.Pool, opts pgx.TxOptions, f func(pgx.Tx) error, log *log.Logger) error {
	count := 0

	for {
		tx, err := db.BeginTx(context.Background(), opts)
		if err != nil {
			return fmt.Errorf("TxWithRetry() begin tx err: %w", err)
		}

		err = attemptTx(tx, f)
		if !isSerializationError(err) {
			if (count > 0) && (log != nil) {
				log.Printf("transaction was retried %d times", count)
			}
			if err != nil {
				return fmt.Errorf("TxWithRetry() err: %w", err)
			}
			return nil
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
		return "", fmt.Errorf("GetMetastate() err: %w", err)
	}

	return value, nil
}

// SetMetastate sets metastate. If `tx` is nil, it uses a normal query.
func SetMetastate(db *pgxpool.Pool, tx pgx.Tx, key, jsonStrValue string) error {
	const setMetastateUpsert = `INSERT INTO metastate (k, v) VALUES ($1, $2)
		ON CONFLICT (k) DO UPDATE SET v = EXCLUDED.v`

	var err error
	if tx == nil {
		_, err = db.Exec(context.Background(), setMetastateUpsert, key, jsonStrValue)
	} else {
		_, err = tx.Exec(context.Background(), setMetastateUpsert, key, jsonStrValue)
	}
	if err != nil {
		return fmt.Errorf("SetMetastate() err: %w", err)
	}

	return nil
}
