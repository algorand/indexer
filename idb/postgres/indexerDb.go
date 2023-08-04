package postgres

import (
	"fmt"
	"sync"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	log "github.com/sirupsen/logrus"

	"github.com/algorand/indexer/v3/idb/migration"
)

// const useExperimentalTxnInsertion = false
// const useExperimentalWithIntraBugfix = true

var serializable = pgx.TxOptions{IsoLevel: pgx.Serializable} // be a real ACID database

// in actuality, for postgres the following is no weaker than ReadCommitted:
// https://www.postgresql.org/docs/current/transaction-iso.html
// TODO: change this to pgs.ReadCommitted
var uncommitted = pgx.TxOptions{IsoLevel: pgx.ReadUncommitted}
var readonlyRepeatableRead = pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}

var experimentalCommitLevel = uncommitted // serializable // uncommitted

// IndexerDb is an idb.IndexerDB implementation
type IndexerDb struct {
	readonly bool
	log      *log.Logger

	db             *pgxpool.Pool
	migration      *migration.Migration
	accountingLock sync.Mutex

	TuningParams
}

type TuningParams struct {
	PgxOpts   pgx.TxOptions
	BatchSize uint32
}

func shortName(isoLevel pgx.TxIsoLevel) string {
	switch isoLevel {
	case pgx.Serializable:
		return "S"
	case pgx.RepeatableRead:
		return "RR"
	case pgx.ReadCommitted:
		return "RC"
	case pgx.ReadUncommitted:
		return "RU"
	default:
		return fmt.Sprintf("unknown_%s", isoLevel)
	}
}
