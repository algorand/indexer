// You can build without postgres by `go build --tags nopostgres` but it's on by default
//go:build !nopostgres
// +build !nopostgres

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/ledger"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/protocol"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	log "github.com/sirupsen/logrus"

	"github.com/algorand/indexer/accounting"
	models "github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/migration"
	"github.com/algorand/indexer/idb/postgres/internal/encoding"
	ledger_for_evaluator "github.com/algorand/indexer/idb/postgres/internal/ledger_for_evaluator"
	"github.com/algorand/indexer/idb/postgres/internal/schema"
	"github.com/algorand/indexer/idb/postgres/internal/types"
	pgutil "github.com/algorand/indexer/idb/postgres/internal/util"
	"github.com/algorand/indexer/idb/postgres/internal/writer"
	"github.com/algorand/indexer/util"
	"github.com/algorand/indexer/util/metrics"
)

var serializable = pgx.TxOptions{IsoLevel: pgx.Serializable} // be a real ACID database
var readonlyRepeatableRead = pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}

// OpenPostgres is available for creating test instances of postgres.IndexerDb
// Returns an error object and a channel that gets closed when blocking migrations
// finish running successfully.
func OpenPostgres(connection string, opts idb.IndexerDbOptions, log *log.Logger) (*IndexerDb, chan struct{}, error) {

	postgresConfig, err := pgxpool.ParseConfig(connection)
	if err != nil {
		return nil, nil, fmt.Errorf("Couldn't parse config: %v", err)
	}

	if opts.MaxConn != 0 {
		postgresConfig.MaxConns = int32(opts.MaxConn)
	}

	db, err := pgxpool.ConnectConfig(context.Background(), postgresConfig)

	if err != nil {
		return nil, nil, fmt.Errorf("connecting to postgres: %v", err)
	}

	if strings.Contains(connection, "readonly") {
		opts.ReadOnly = true
	}

	return openPostgres(db, opts, log)
}

// Allow tests to inject a DB
func openPostgres(db *pgxpool.Pool, opts idb.IndexerDbOptions, logger *log.Logger) (*IndexerDb, chan struct{}, error) {
	idb := &IndexerDb{
		readonly: opts.ReadOnly,
		log:      logger,
		db:       db,
	}

	if idb.log == nil {
		idb.log = log.New()
		idb.log.SetFormatter(&log.JSONFormatter{})
		idb.log.SetOutput(os.Stdout)
		idb.log.SetLevel(log.TraceLevel)
	}

	var ch chan struct{}
	// e.g. a user named "readonly" is in the connection string
	if opts.ReadOnly {
		migrationState, err := idb.getMigrationState(context.Background(), nil)
		if err != nil {
			return nil, nil, fmt.Errorf("openPostgres() err: %w", err)
		}

		ch = make(chan struct{})
		if !migrationStateBlocked(migrationState) {
			close(ch)
		}
	} else {
		var err error
		ch, err = idb.init(opts)
		if err != nil {
			return nil, nil, fmt.Errorf("initializing postgres: %v", err)
		}
	}

	return idb, ch, nil
}

// IndexerDb is an idb.IndexerDB implementation
type IndexerDb struct {
	readonly bool
	log      *log.Logger

	db             *pgxpool.Pool
	migration      *migration.Migration
	accountingLock sync.Mutex
}

// Close is part of idb.IndexerDb.
func (db *IndexerDb) Close() {
	db.db.Close()
}

// txWithRetry is a helper function that retries the function `f` in case the database
// transaction in it fails due to a serialization error. `f` is provided
// a transaction created using `opts`. If `f` experiences a database error, this error
// must be included in `f`'s return error's chain, so that a serialization error can be
// detected.
func (db *IndexerDb) txWithRetry(opts pgx.TxOptions, f func(pgx.Tx) error) error {
	return pgutil.TxWithRetry(db.db, opts, f, db.log)
}

func (db *IndexerDb) isSetup() (bool, error) {
	query := `SELECT 0 FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_NAME = 'metastate'`
	row := db.db.QueryRow(context.Background(), query)

	var tmp int
	err := row.Scan(&tmp)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("isSetup() err: %w", err)
	}
	return true, nil
}

// Returns an error object and a channel that gets closed when blocking migrations
// finish running successfully.
func (db *IndexerDb) init(opts idb.IndexerDbOptions) (chan struct{}, error) {
	setup, err := db.isSetup()
	if err != nil {
		return nil, fmt.Errorf("init() err: %w", err)
	}

	if !setup {
		// new database, run setup
		_, err = db.db.Exec(context.Background(), schema.SetupPostgresSql)
		if err != nil {
			return nil, fmt.Errorf("unable to setup postgres: %v", err)
		}

		err = db.markMigrationsAsDone()
		if err != nil {
			return nil, fmt.Errorf("unable to confirm migration: %v", err)
		}

		ch := make(chan struct{})
		close(ch)
		return ch, nil
	}

	// see postgres_migrations.go
	return db.runAvailableMigrations()
}

// Returns all addresses referenced in `block`.
func getBlockAddresses(block *bookkeeping.Block) map[basics.Address]struct{} {
	// Reserve a reasonable memory size for the map.
	res := make(map[basics.Address]struct{}, len(block.Payset)+2)

	res[block.FeeSink] = struct{}{}
	res[block.RewardsPool] = struct{}{}
	for _, stib := range block.Payset {
		addFunc := func(address basics.Address) {
			res[address] = struct{}{}
		}
		accounting.GetTransactionParticipants(&stib.SignedTxnWithAD, true, addFunc)
	}

	return res
}

func prepareEvalResources(l *ledger_for_evaluator.LedgerForEvaluator, block *bookkeeping.Block) (ledger.EvalForIndexerResources, error) {
	addresses := getBlockAddresses(block)
	assets := make(map[basics.AssetIndex]struct{})
	apps := make(map[basics.AppIndex]struct{})

	for _, stib := range block.Payset {
		switch stib.Txn.Type {
		case protocol.AssetConfigTx:
			if stib.Txn.ConfigAsset != 0 {
				assets[stib.Txn.ConfigAsset] = struct{}{}
			}
		case protocol.AssetTransferTx:
			if stib.Txn.XferAsset != 0 {
				assets[stib.Txn.XferAsset] = struct{}{}
			}
		case protocol.AssetFreezeTx:
			if stib.Txn.FreezeAsset != 0 {
				assets[stib.Txn.FreezeAsset] = struct{}{}
			}
		case protocol.ApplicationCallTx:
			if stib.Txn.ApplicationID != 0 {
				apps[stib.Txn.ApplicationID] = struct{}{}
			}
		}
	}

	res := ledger.EvalForIndexerResources{
		Accounts: nil,
		Creators: make(map[ledger.Creatable]ledger.FoundAddress),
	}

	assetCreators, err := l.GetAssetCreator(assets)
	if err != nil {
		return ledger.EvalForIndexerResources{},
			fmt.Errorf("prepareEvalResources() err: %w", err)
	}
	for index, foundAddress := range assetCreators {
		creatable := ledger.Creatable{
			Index: basics.CreatableIndex(index),
			Type:  basics.AssetCreatable,
		}
		res.Creators[creatable] = foundAddress

		if foundAddress.Exists {
			addresses[foundAddress.Address] = struct{}{}
		}
	}

	appCreators, err := l.GetAppCreator(apps)
	if err != nil {
		return ledger.EvalForIndexerResources{},
			fmt.Errorf("prepareEvalResources() err: %w", err)
	}
	for index, foundAddress := range appCreators {
		creatable := ledger.Creatable{
			Index: basics.CreatableIndex(index),
			Type:  basics.AppCreatable,
		}
		res.Creators[creatable] = foundAddress

		if foundAddress.Exists {
			addresses[foundAddress.Address] = struct{}{}
		}
	}

	res.Accounts, err = l.LookupWithoutRewards(addresses)
	if err != nil {
		return ledger.EvalForIndexerResources{},
			fmt.Errorf("prepareEvalResources() err: %w", err)
	}

	return res, nil
}

// AddBlock is part of idb.IndexerDb.
func (db *IndexerDb) AddBlock(block *bookkeeping.Block) error {
	db.log.Printf("adding block %d", block.Round())

	db.accountingLock.Lock()
	defer db.accountingLock.Unlock()

	f := func(tx pgx.Tx) error {
		// Check and increment next round counter.
		importstate, err := db.getImportState(context.Background(), tx)
		if err != nil {
			return fmt.Errorf("AddBlock() err: %w", err)
		}
		if block.Round() != basics.Round(importstate.NextRoundToAccount) {
			return fmt.Errorf(
				"AddBlock() adding block round %d but next round to account is %d",
				block.Round(), importstate.NextRoundToAccount)
		}
		importstate.NextRoundToAccount++
		err = db.setImportState(tx, &importstate)
		if err != nil {
			return fmt.Errorf("AddBlock() err: %w", err)
		}

		w, err := writer.MakeWriter(tx)
		if err != nil {
			return fmt.Errorf("AddBlock() err: %w", err)
		}
		defer w.Close()

		if block.Round() == basics.Round(0) {
			// Block 0 is special, we cannot run the evaluator on it.
			err := w.AddBlock0(block)
			if err != nil {
				return fmt.Errorf("AddBlock() err: %w", err)
			}
		} else {
			proto, ok := config.Consensus[block.BlockHeader.CurrentProtocol]
			if !ok {
				return fmt.Errorf(
					"AddBlock() cannot find proto version %s", block.BlockHeader.CurrentProtocol)
			}
			protoChanged := !proto.EnableAssetCloseAmount
			proto.EnableAssetCloseAmount = true

			var wg sync.WaitGroup
			defer wg.Wait()

			// Write transaction participation and possibly transactions in a parallel db
			// transaction. If `proto.EnableAssetCloseAmount` is already true, we can start
			// writing transactions contained in the block early.
			var err0 error
			wg.Add(1)
			go func() {
				defer wg.Done()

				f := func(tx pgx.Tx) error {
					if !protoChanged {
						err := writer.AddTransactions(block, block.Payset, tx)
						if err != nil {
							return err
						}
					}
					return writer.AddTransactionParticipation(block, tx)
				}
				err0 = db.txWithRetry(serializable, f)
			}()

			ledgerForEval, err :=
				ledger_for_evaluator.MakeLedgerForEvaluator(tx, block.Round()-1)
			if err != nil {
				return fmt.Errorf("AddBlock() err: %w", err)
			}
			defer ledgerForEval.Close()

			resources, err := prepareEvalResources(&ledgerForEval, block)
			if err != nil {
				return fmt.Errorf("AddBlock() eval err: %w", err)
			}

			start := time.Now()
			delta, modifiedTxns, err :=
				ledger.EvalForIndexer(ledgerForEval, block, proto, resources)
			if err != nil {
				return fmt.Errorf("AddBlock() eval err: %w", err)
			}
			metrics.PostgresEvalTimeSeconds.Observe(time.Since(start).Seconds())

			var err1 error
			// Skip if transaction writing has already started.
			if protoChanged {
				// Write transactions in a parallel db transaction.
				wg.Add(1)
				go func() {
					defer wg.Done()

					f := func(tx pgx.Tx) error {
						return writer.AddTransactions(block, modifiedTxns, tx)
					}
					err1 = db.txWithRetry(serializable, f)
				}()
			}

			err = w.AddBlock(block, modifiedTxns, delta)
			if err != nil {
				return fmt.Errorf("AddBlock() err: %w", err)
			}

			// Wait for goroutines to finish and check for errors. If there is an error, we
			// return our own error so that the main transaction does not commit. Hence,
			// `txn` and `txn_participation` tables can only be ahead but not behind
			// the other state.
			wg.Wait()
			isUniqueViolationFunc := func(err error) bool {
				var pgerr *pgconn.PgError
				return errors.As(err, &pgerr) && (pgerr.Code == pgerrcode.UniqueViolation)
			}
			if (err0 != nil) && !isUniqueViolationFunc(err0) {
				return fmt.Errorf("AddBlock() err0: %w", err0)
			}
			if (err1 != nil) && !isUniqueViolationFunc(err1) {
				return fmt.Errorf("AddBlock() err1: %w", err1)
			}
		}

		return nil
	}
	err := db.txWithRetry(serializable, f)
	if err != nil {
		return fmt.Errorf("AddBlock() err: %w", err)
	}

	return nil
}

// LoadGenesis is part of idb.IndexerDB
func (db *IndexerDb) LoadGenesis(genesis bookkeeping.Genesis) error {
	f := func(tx pgx.Tx) error {
		// check genesis hash
		network, err := db.getNetworkState(context.Background(), tx)
		if err == idb.ErrorNotInitialized {
			networkState := types.NetworkState{
				GenesisHash: crypto.HashObj(genesis),
			}
			err = db.setNetworkState(tx, &networkState)
			if err != nil {
				return fmt.Errorf("LoadGenesis() err: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("LoadGenesis() err: %w", err)
		} else {
			if network.GenesisHash != crypto.HashObj(genesis) {
				return fmt.Errorf("LoadGenesis() genesis hash not matching")
			}
		}
		setAccountStatementName := "set_account"
		query := `INSERT INTO account (addr, microalgos, rewardsbase, account_data, rewards_total, created_at, deleted) VALUES ($1, $2, 0, $3, $4, 0, false)`
		_, err = tx.Prepare(context.Background(), setAccountStatementName, query)
		if err != nil {
			return fmt.Errorf("LoadGenesis() prepare tx err: %w", err)
		}
		defer tx.Conn().Deallocate(context.Background(), setAccountStatementName)

		proto, ok := config.Consensus[genesis.Proto]
		if !ok {
			return fmt.Errorf("LoadGenesis() consensus version %s not found", genesis.Proto)
		}
		var ot basics.OverflowTracker
		var totals ledgercore.AccountTotals
		for ai, alloc := range genesis.Allocation {
			addr, err := basics.UnmarshalChecksumAddress(alloc.Address)
			if err != nil {
				return fmt.Errorf("LoadGenesis() decode address err: %w", err)
			}
			if len(alloc.State.AssetParams) > 0 || len(alloc.State.Assets) > 0 {
				return fmt.Errorf("LoadGenesis() genesis account[%d] has unhandled asset", ai)
			}
			_, err = tx.Exec(
				context.Background(), setAccountStatementName,
				addr[:], alloc.State.MicroAlgos.Raw,
				encoding.EncodeTrimmedAccountData(encoding.TrimAccountData(alloc.State)), 0)
			if err != nil {
				return fmt.Errorf("LoadGenesis() error setting genesis account[%d], %w", ai, err)
			}

			totals.AddAccount(proto, alloc.State, &ot)
		}

		err = db.setMetastate(
			tx, schema.AccountTotals, string(encoding.EncodeAccountTotals(&totals)))
		if err != nil {
			return fmt.Errorf("LoadGenesis() err: %w", err)
		}

		importstate := types.ImportState{
			NextRoundToAccount: 0,
		}
		err = db.setImportState(tx, &importstate)
		if err != nil {
			return fmt.Errorf("LoadGenesis() err: %w", err)
		}

		return nil
	}
	err := db.txWithRetry(serializable, f)
	if err != nil {
		return fmt.Errorf("LoadGenesis() err: %w", err)
	}

	return nil
}

// Returns `idb.ErrorNotInitialized` if uninitialized.
// If `tx` is nil, use a normal query.
func (db *IndexerDb) getMetastate(ctx context.Context, tx pgx.Tx, key string) (string, error) {
	return pgutil.GetMetastate(ctx, db.db, tx, key)
}

// If `tx` is nil, use a normal query.
func (db *IndexerDb) setMetastate(tx pgx.Tx, key, jsonStrValue string) (err error) {
	return pgutil.SetMetastate(db.db, tx, key, jsonStrValue)
}

// Returns idb.ErrorNotInitialized if uninitialized.
// If `tx` is nil, use a normal query.
func (db *IndexerDb) getImportState(ctx context.Context, tx pgx.Tx) (types.ImportState, error) {
	importStateJSON, err := db.getMetastate(ctx, tx, schema.StateMetastateKey)
	if err == idb.ErrorNotInitialized {
		return types.ImportState{}, idb.ErrorNotInitialized
	}
	if err != nil {
		return types.ImportState{}, fmt.Errorf("unable to get import state err: %w", err)
	}

	state, err := encoding.DecodeImportState([]byte(importStateJSON))
	if err != nil {
		return types.ImportState{},
			fmt.Errorf("unable to parse import state v: \"%s\" err: %w", importStateJSON, err)
	}

	return state, nil
}

// If `tx` is nil, use a normal query.
func (db *IndexerDb) setImportState(tx pgx.Tx, state *types.ImportState) error {
	return db.setMetastate(
		tx, schema.StateMetastateKey, string(encoding.EncodeImportState(state)))
}

// Returns idb.ErrorNotInitialized if uninitialized.
// If `tx` is nil, use a normal query.
func (db *IndexerDb) getNetworkState(ctx context.Context, tx pgx.Tx) (types.NetworkState, error) {
	networkStateJSON, err := db.getMetastate(ctx, tx, schema.NetworkMetaStateKey)
	if err == idb.ErrorNotInitialized {
		return types.NetworkState{}, idb.ErrorNotInitialized
	}
	if err != nil {
		return types.NetworkState{}, fmt.Errorf("unable to get network state err: %w", err)
	}

	state, err := encoding.DecodeNetworkState([]byte(networkStateJSON))
	if err != nil {
		return types.NetworkState{},
			fmt.Errorf("unable to parse network state v: \"%s\" err: %w", networkStateJSON, err)
	}

	return state, nil
}

// If `tx` is nil, use a normal query.
func (db *IndexerDb) setNetworkState(tx pgx.Tx, state *types.NetworkState) error {
	return db.setMetastate(
		tx, schema.NetworkMetaStateKey, string(encoding.EncodeNetworkState(state)))
}

// Returns ErrorNotInitialized if genesis is not loaded.
// If `tx` is nil, use a normal query.
func (db *IndexerDb) getNextRoundToAccount(ctx context.Context, tx pgx.Tx) (uint64, error) {
	state, err := db.getImportState(ctx, tx)
	if err == idb.ErrorNotInitialized {
		return 0, err
	}
	if err != nil {
		return 0, fmt.Errorf("getNextRoundToAccount() err: %w", err)
	}

	return state.NextRoundToAccount, nil
}

// GetNextRoundToAccount is part of idb.IndexerDB
// Returns ErrorNotInitialized if genesis is not loaded.
func (db *IndexerDb) GetNextRoundToAccount() (uint64, error) {
	return db.getNextRoundToAccount(context.Background(), nil)
}

// Returns ErrorNotInitialized if genesis is not loaded.
// If `tx` is nil, use a normal query.
func (db *IndexerDb) getMaxRoundAccounted(ctx context.Context, tx pgx.Tx) (uint64, error) {
	round, err := db.getNextRoundToAccount(ctx, tx)
	if err != nil {
		return 0, err
	}

	if round > 0 {
		round--
	}
	return round, nil
}

// GetBlock is part of idb.IndexerDB
func (db *IndexerDb) GetBlock(ctx context.Context, round uint64, options idb.GetBlockOptions) (blockHeader bookkeeping.BlockHeader, transactions []idb.TxnRow, err error) {
	tx, err := db.db.BeginTx(ctx, readonlyRepeatableRead)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)
	row := tx.QueryRow(ctx, `SELECT header FROM block_header WHERE round = $1`, round)
	var blockheaderjson []byte
	err = row.Scan(&blockheaderjson)
	if err == pgx.ErrNoRows {
		err = idb.ErrorBlockNotFound
		return
	}
	if err != nil {
		return
	}
	blockHeader, err = encoding.DecodeBlockHeader(blockheaderjson)
	if err != nil {
		return
	}

	if options.Transactions {
		out := make(chan idb.TxnRow, 1)
		query, whereArgs, err := buildTransactionQuery(idb.TransactionFilter{Round: &round})
		if err != nil {
			err = fmt.Errorf("txn query err %v", err)
			out <- idb.TxnRow{Error: err}
			close(out)
			return bookkeeping.BlockHeader{}, nil, err
		}
		rows, err := tx.Query(ctx, query, whereArgs...)
		if err != nil {
			err = fmt.Errorf("txn query %#v err %v", query, err)
			return bookkeeping.BlockHeader{}, nil, err
		}

		go func() {
			db.yieldTxnsThreadSimple(rows, out, nil, nil)
			close(out)
		}()

		results := make([]idb.TxnRow, 0)
		for txrow := range out {
			results = append(results, txrow)
		}
		transactions = results
	}

	return blockHeader, transactions, nil
}

func buildTransactionQuery(tf idb.TransactionFilter) (query string, whereArgs []interface{}, err error) {
	// TODO? There are some combinations of tf params that will
	// yield no results and we could catch that before asking the
	// database. A hopefully rare optimization.
	const maxWhereParts = 30
	whereParts := make([]string, 0, maxWhereParts)
	whereArgs = make([]interface{}, 0, maxWhereParts)
	joinParticipation := false
	partNumber := 1
	if tf.Address != nil {
		whereParts = append(whereParts, fmt.Sprintf("p.addr = $%d", partNumber))
		whereArgs = append(whereArgs, tf.Address)
		partNumber++
		if tf.AddressRole != 0 {
			addrBase64 := encoding.Base64(tf.Address)
			roleparts := make([]string, 0, 8)
			if tf.AddressRole&idb.AddressRoleSender != 0 {
				roleparts = append(roleparts, fmt.Sprintf("t.txn -> 'txn' ->> 'snd' = $%d", partNumber))
				whereArgs = append(whereArgs, addrBase64)
				partNumber++
			}
			if tf.AddressRole&idb.AddressRoleReceiver != 0 {
				roleparts = append(roleparts, fmt.Sprintf("t.txn -> 'txn' ->> 'rcv' = $%d", partNumber))
				whereArgs = append(whereArgs, addrBase64)
				partNumber++
			}
			if tf.AddressRole&idb.AddressRoleCloseRemainderTo != 0 {
				roleparts = append(roleparts, fmt.Sprintf("t.txn -> 'txn' ->> 'close' = $%d", partNumber))
				whereArgs = append(whereArgs, addrBase64)
				partNumber++
			}
			if tf.AddressRole&idb.AddressRoleAssetSender != 0 {
				roleparts = append(roleparts, fmt.Sprintf("t.txn -> 'txn' ->> 'asnd' = $%d", partNumber))
				whereArgs = append(whereArgs, addrBase64)
				partNumber++
			}
			if tf.AddressRole&idb.AddressRoleAssetReceiver != 0 {
				roleparts = append(roleparts, fmt.Sprintf("t.txn -> 'txn' ->> 'arcv' = $%d", partNumber))
				whereArgs = append(whereArgs, addrBase64)
				partNumber++
			}
			if tf.AddressRole&idb.AddressRoleAssetCloseTo != 0 {
				roleparts = append(roleparts, fmt.Sprintf("t.txn -> 'txn' ->> 'aclose' = $%d", partNumber))
				whereArgs = append(whereArgs, addrBase64)
				partNumber++
			}
			if tf.AddressRole&idb.AddressRoleFreeze != 0 {
				roleparts = append(roleparts, fmt.Sprintf("t.txn -> 'txn' ->> 'fadd' = $%d", partNumber))
				whereArgs = append(whereArgs, addrBase64)
				partNumber++
			}
			rolepart := strings.Join(roleparts, " OR ")
			whereParts = append(whereParts, "("+rolepart+")")
		}
		joinParticipation = true
	}
	if tf.MinRound != 0 {
		whereParts = append(whereParts, fmt.Sprintf("t.round >= $%d", partNumber))
		whereArgs = append(whereArgs, tf.MinRound)
		partNumber++
	}
	if tf.MaxRound != 0 {
		whereParts = append(whereParts, fmt.Sprintf("t.round <= $%d", partNumber))
		whereArgs = append(whereArgs, tf.MaxRound)
		partNumber++
	}
	if !tf.BeforeTime.IsZero() {
		whereParts = append(whereParts, fmt.Sprintf("h.realtime < $%d", partNumber))
		whereArgs = append(whereArgs, tf.BeforeTime)
		partNumber++
	}
	if !tf.AfterTime.IsZero() {
		whereParts = append(whereParts, fmt.Sprintf("h.realtime > $%d", partNumber))
		whereArgs = append(whereArgs, tf.AfterTime)
		partNumber++
	}
	if tf.AssetID != 0 || tf.ApplicationID != 0 {
		var creatableID uint64
		if tf.AssetID != 0 {
			creatableID = tf.AssetID
			if tf.ApplicationID != 0 {
				if tf.AssetID == tf.ApplicationID {
					// this is nonsense, but I'll allow it
				} else {
					return "", nil, fmt.Errorf("cannot search both assetid and appid")
				}
			}
		} else {
			creatableID = tf.ApplicationID
		}
		whereParts = append(whereParts, fmt.Sprintf("t.asset = $%d", partNumber))
		whereArgs = append(whereArgs, creatableID)
		partNumber++
	}
	if tf.AssetAmountGT != nil {
		whereParts = append(whereParts, fmt.Sprintf("(t.txn -> 'txn' -> 'aamt')::bigint > $%d", partNumber))
		whereArgs = append(whereArgs, *tf.AssetAmountGT)
		partNumber++
	}
	if tf.AssetAmountLT != nil {
		whereParts = append(whereParts, fmt.Sprintf("(t.txn -> 'txn' -> 'aamt')::bigint < $%d", partNumber))
		whereArgs = append(whereArgs, *tf.AssetAmountLT)
		partNumber++
	}
	if tf.TypeEnum != 0 {
		whereParts = append(whereParts, fmt.Sprintf("t.typeenum = $%d", partNumber))
		whereArgs = append(whereArgs, tf.TypeEnum)
		partNumber++
	}
	if len(tf.Txid) != 0 {
		whereParts = append(whereParts, fmt.Sprintf("t.txid = $%d", partNumber))
		whereArgs = append(whereArgs, tf.Txid)
		partNumber++
	}
	if tf.Round != nil {
		whereParts = append(whereParts, fmt.Sprintf("t.round = $%d", partNumber))
		whereArgs = append(whereArgs, *tf.Round)
		partNumber++
	}
	if tf.Offset != nil {
		whereParts = append(whereParts, fmt.Sprintf("t.intra = $%d", partNumber))
		whereArgs = append(whereArgs, *tf.Offset)
		partNumber++
	}
	if tf.OffsetLT != nil {
		whereParts = append(whereParts, fmt.Sprintf("t.intra < $%d", partNumber))
		whereArgs = append(whereArgs, *tf.OffsetLT)
		partNumber++
	}
	if tf.OffsetGT != nil {
		whereParts = append(whereParts, fmt.Sprintf("t.intra > $%d", partNumber))
		whereArgs = append(whereArgs, *tf.OffsetGT)
		partNumber++
	}
	if len(tf.SigType) != 0 {
		whereParts = append(whereParts, fmt.Sprintf("t.txn -> $%d IS NOT NULL", partNumber))
		whereArgs = append(whereArgs, tf.SigType)
		partNumber++
	}
	if len(tf.NotePrefix) > 0 {
		whereParts = append(whereParts, fmt.Sprintf("substring(decode(t.txn -> 'txn' ->> 'note', 'base64') from 1 for %d) = $%d", len(tf.NotePrefix), partNumber))
		whereArgs = append(whereArgs, tf.NotePrefix)
		partNumber++
	}
	if tf.AlgosGT != nil {
		whereParts = append(whereParts, fmt.Sprintf("(t.txn -> 'txn' -> 'amt')::bigint > $%d", partNumber))
		whereArgs = append(whereArgs, *tf.AlgosGT)
		partNumber++
	}
	if tf.AlgosLT != nil {
		whereParts = append(whereParts, fmt.Sprintf("(t.txn -> 'txn' -> 'amt')::bigint < $%d", partNumber))
		whereArgs = append(whereArgs, *tf.AlgosLT)
		partNumber++
	}
	if tf.EffectiveAmountGT != nil {
		whereParts = append(whereParts, fmt.Sprintf("((t.txn -> 'ca')::bigint + (t.txn -> 'txn' -> 'amt')::bigint) > $%d", partNumber))
		whereArgs = append(whereArgs, *tf.EffectiveAmountGT)
		partNumber++
	}
	if tf.EffectiveAmountLT != nil {
		whereParts = append(whereParts, fmt.Sprintf("((t.txn -> 'ca')::bigint + (t.txn -> 'txn' -> 'amt')::bigint) < $%d", partNumber))
		whereArgs = append(whereArgs, *tf.EffectiveAmountLT)
		partNumber++
	}
	if tf.RekeyTo != nil && (*tf.RekeyTo) {
		whereParts = append(whereParts, "(t.txn -> 'txn' -> 'rekey') IS NOT NULL")
	}

	// If returnInnerTxnOnly flag is false, then return the root transaction
	if !tf.ReturnInnerTxnOnly {
		query = "SELECT t.round, t.intra, t.txn, root.txn, t.extra, t.asset, h.realtime FROM txn t JOIN block_header h ON t.round = h.round"
	} else {
		query = "SELECT t.round, t.intra, t.txn, NULL, t.extra, t.asset, h.realtime FROM txn t JOIN block_header h ON t.round = h.round"
	}

	if joinParticipation {
		query += " JOIN txn_participation p ON t.round = p.round AND t.intra = p.intra"
	}

	// join in the root transaction if the returnInnerTxnOnly flag is false
	if !tf.ReturnInnerTxnOnly {
		query += " LEFT OUTER JOIN txn root ON t.round = root.round AND t.extra->>'root-intra' = root.intra::text"
	}

	if len(whereParts) > 0 {
		whereStr := strings.Join(whereParts, " AND ")
		query += " WHERE " + whereStr
	}
	if joinParticipation {
		// this should match the index on txn_particpation
		query += " ORDER BY p.addr, p.round DESC, p.intra DESC"
	} else {
		// this should explicitly match the primary key on txn (round,intra)
		query += " ORDER BY t.round, t.intra"
	}
	if tf.Limit != 0 {
		query += fmt.Sprintf(" LIMIT %d", tf.Limit)
	}
	return
}

// This function blocks. `tx` must be non-nil.
func (db *IndexerDb) yieldTxns(ctx context.Context, tx pgx.Tx, tf idb.TransactionFilter, out chan<- idb.TxnRow) {
	if len(tf.NextToken) > 0 {
		db.txnsWithNext(ctx, tx, tf, out)
		return
	}

	query, whereArgs, err := buildTransactionQuery(tf)
	if err != nil {
		err = fmt.Errorf("txn query err %v", err)
		out <- idb.TxnRow{Error: err}
		return
	}

	rows, err := tx.Query(ctx, query, whereArgs...)
	if err != nil {
		err = fmt.Errorf("txn query %#v err %v", query, err)
		out <- idb.TxnRow{Error: err}
		return
	}

	db.yieldTxnsThreadSimple(rows, out, nil, nil)
}

// Transactions is part of idb.IndexerDB
func (db *IndexerDb) Transactions(ctx context.Context, tf idb.TransactionFilter) (<-chan idb.TxnRow, uint64) {
	out := make(chan idb.TxnRow, 1)

	tx, err := db.db.BeginTx(ctx, readonlyRepeatableRead)
	if err != nil {
		out <- idb.TxnRow{Error: err}
		close(out)
		return out, 0
	}

	round, err := db.getMaxRoundAccounted(ctx, tx)
	if err != nil {
		tx.Rollback(ctx)
		out <- idb.TxnRow{Error: err}
		close(out)
		return out, round
	}

	go func() {
		db.yieldTxns(ctx, tx, tf, out)
		tx.Rollback(ctx)
		close(out)
	}()

	return out, round
}

// This function blocks. `tx` must be non-nil.
func (db *IndexerDb) txnsWithNext(ctx context.Context, tx pgx.Tx, tf idb.TransactionFilter, out chan<- idb.TxnRow) {
	// TODO: Use txid to deduplicate next resultset at the query level?

	// Check for remainder of round from previous page.
	nextround, nextintra32, err := idb.DecodeTxnRowNext(tf.NextToken)
	nextintra := uint64(nextintra32)
	if err != nil {
		out <- idb.TxnRow{Error: err}
		return
	}
	origRound := tf.Round
	origOLT := tf.OffsetLT
	origOGT := tf.OffsetGT
	if tf.Address != nil {
		// (round,intra) descending into the past
		if nextround == 0 && nextintra == 0 {
			return
		}
		tf.Round = &nextround
		tf.OffsetLT = &nextintra
	} else {
		// (round,intra) ascending into the future
		tf.Round = &nextround
		tf.OffsetGT = &nextintra
	}
	query, whereArgs, err := buildTransactionQuery(tf)
	if err != nil {
		err = fmt.Errorf("txn query err %v", err)
		out <- idb.TxnRow{Error: err}
		return
	}
	rows, err := tx.Query(ctx, query, whereArgs...)
	if err != nil {
		err = fmt.Errorf("txn query %#v err %v", query, err)
		out <- idb.TxnRow{Error: err}
		return
	}

	count := 0
	db.yieldTxnsThreadSimple(rows, out, &count, &err)
	if err != nil {
		return
	}

	// If we haven't reached the limit, restore the original filter and
	// re-run the original search with new Min/Max round and reduced limit.
	if uint64(count) >= tf.Limit {
		return
	}
	tf.Limit -= uint64(count)
	select {
	case <-ctx.Done():
		return
	default:
	}
	tf.Round = origRound
	if tf.Address != nil {
		// (round,intra) descending into the past
		tf.OffsetLT = origOLT

		if nextround <= 1 {
			// NO second query
			return
		}

		tf.MaxRound = nextround - 1
	} else {
		// (round,intra) ascending into the future
		tf.OffsetGT = origOGT
		tf.MinRound = nextround + 1
	}
	query, whereArgs, err = buildTransactionQuery(tf)
	if err != nil {
		err = fmt.Errorf("txn query err %v", err)
		out <- idb.TxnRow{Error: err}
		return
	}
	rows, err = tx.Query(ctx, query, whereArgs...)
	if err != nil {
		err = fmt.Errorf("txn query %#v err %v", query, err)
		out <- idb.TxnRow{Error: err}
		return
	}
	db.yieldTxnsThreadSimple(rows, out, nil, nil)
}

func (db *IndexerDb) yieldTxnsThreadSimple(rows pgx.Rows, results chan<- idb.TxnRow, countp *int, errp *error) {
	defer rows.Close()

	count := 0
	for rows.Next() {
		var round uint64
		var asset uint64
		var intra int
		var txn []byte
		var roottxn []byte
		var extra []byte
		var roundtime time.Time
		err := rows.Scan(&round, &intra, &txn, &roottxn, &extra, &asset, &roundtime)
		var row idb.TxnRow
		if err != nil {
			row.Error = err
		} else {
			row.Round = round
			row.Intra = intra
			if roottxn != nil {
				// Inner transaction.
				row.RootTxn = new(transactions.SignedTxnWithAD)
				*row.RootTxn, err = encoding.DecodeSignedTxnWithAD(roottxn)
				if err != nil {
					err = fmt.Errorf("error decoding roottxn, err: %w", err)
					row.Error = err
				}
			} else {
				// Root transaction.
				row.Txn = new(transactions.SignedTxnWithAD)
				*row.Txn, err = encoding.DecodeSignedTxnWithAD(txn)
				if err != nil {
					err = fmt.Errorf("error decoding txn, err: %w", err)
					row.Error = err
				}
			}

			row.RoundTime = roundtime
			row.AssetID = asset
			if len(extra) > 0 {
				row.Extra, err = encoding.DecodeTxnExtra(extra)
				if err != nil {
					err = fmt.Errorf("%d:%d decode txn extra, %v", row.Round, row.Intra, err)
					row.Error = err
				}
			}
		}
		results <- row
		if row.Error != nil {
			if errp != nil {
				*errp = err
			}
			goto finish
		}
		count++
	}
	if err := rows.Err(); err != nil {
		results <- idb.TxnRow{Error: err}
		if errp != nil {
			*errp = err
		}
	}
finish:
	if countp != nil {
		*countp = count
	}
}

var statusStrings = []string{"Offline", "Online", "NotParticipating"}

const offlineStatusIdx = 0

func tealValueToModel(tv basics.TealValue) models.TealValue {
	switch tv.Type {
	case basics.TealUintType:
		return models.TealValue{
			Uint: tv.Uint,
			Type: uint64(tv.Type),
		}
	case basics.TealBytesType:
		return models.TealValue{
			Bytes: encoding.Base64([]byte(tv.Bytes)),
			Type:  uint64(tv.Type),
		}
	}
	return models.TealValue{}
}

func tealKeyValueToModel(tkv basics.TealKeyValue) *models.TealKeyValueStore {
	if len(tkv) == 0 {
		return nil
	}
	var out models.TealKeyValueStore = make([]models.TealKeyValue, len(tkv))
	pos := 0
	for key, tv := range tkv {
		out[pos].Key = encoding.Base64([]byte(key))
		out[pos].Value = tealValueToModel(tv)
		pos++
	}
	return &out
}

func (db *IndexerDb) yieldAccountsThread(req *getAccountsRequest) {
	count := uint64(0)
	defer func() {
		req.rows.Close()

		end := time.Now()
		dt := end.Sub(req.start)
		if dt > (1 * time.Second) {
			db.log.Warnf("long query %fs: %s", dt.Seconds(), req.query)
		}
	}()
	for req.rows.Next() {
		var addr []byte
		var microalgos uint64
		var rewardstotal uint64
		var createdat sql.NullInt64
		var closedat sql.NullInt64
		var deleted sql.NullBool
		var rewardsbase uint64
		var keytype *string
		var accountDataJSONStr []byte

		// below are bytes of json serialization

		// holding* are a triplet of lists that should merge together
		var holdingAssetids []byte
		var holdingAmount []byte
		var holdingFrozen []byte
		var holdingCreatedBytes []byte
		var holdingClosedBytes []byte
		var holdingDeletedBytes []byte

		// assetParams* are a pair of lists that should merge together
		var assetParamsIds []byte
		var assetParamsStr []byte
		var assetParamsCreatedBytes []byte
		var assetParamsClosedBytes []byte
		var assetParamsDeletedBytes []byte

		// appParam* are a pair of lists that should merge together
		var appParamIndexes []byte // [appId, ...]
		var appParams []byte       // [{AppParams}, ...]
		var appCreatedBytes []byte
		var appClosedBytes []byte
		var appDeletedBytes []byte

		// localState* are a pair of lists that should merge together
		var localStateAppIds []byte // [appId, ...]
		var localStates []byte      // [{local state}, ...]
		var localStateCreatedBytes []byte
		var localStateClosedBytes []byte
		var localStateDeletedBytes []byte

		var err error

		if req.opts.IncludeAssetHoldings && req.opts.IncludeAssetParams {
			err = req.rows.Scan(
				&addr, &microalgos, &rewardstotal, &createdat, &closedat, &deleted, &rewardsbase, &keytype, &accountDataJSONStr,
				&holdingAssetids, &holdingAmount, &holdingFrozen, &holdingCreatedBytes, &holdingClosedBytes, &holdingDeletedBytes,
				&assetParamsIds, &assetParamsStr, &assetParamsCreatedBytes, &assetParamsClosedBytes, &assetParamsDeletedBytes,
				&appParamIndexes, &appParams, &appCreatedBytes, &appClosedBytes, &appDeletedBytes, &localStateAppIds, &localStates,
				&localStateCreatedBytes, &localStateClosedBytes, &localStateDeletedBytes,
			)
		} else if req.opts.IncludeAssetHoldings {
			err = req.rows.Scan(
				&addr, &microalgos, &rewardstotal, &createdat, &closedat, &deleted, &rewardsbase, &keytype, &accountDataJSONStr,
				&holdingAssetids, &holdingAmount, &holdingFrozen, &holdingCreatedBytes, &holdingClosedBytes, &holdingDeletedBytes,
				&appParamIndexes, &appParams, &appCreatedBytes, &appClosedBytes, &appDeletedBytes, &localStateAppIds, &localStates,
				&localStateCreatedBytes, &localStateClosedBytes, &localStateDeletedBytes,
			)
		} else if req.opts.IncludeAssetParams {
			err = req.rows.Scan(
				&addr, &microalgos, &rewardstotal, &createdat, &closedat, &deleted, &rewardsbase, &keytype, &accountDataJSONStr,
				&assetParamsIds, &assetParamsStr, &assetParamsCreatedBytes, &assetParamsClosedBytes, &assetParamsDeletedBytes,
				&appParamIndexes, &appParams, &appCreatedBytes, &appClosedBytes, &appDeletedBytes, &localStateAppIds, &localStates,
				&localStateCreatedBytes, &localStateClosedBytes, &localStateDeletedBytes,
			)
		} else {
			err = req.rows.Scan(
				&addr, &microalgos, &rewardstotal, &createdat, &closedat, &deleted, &rewardsbase, &keytype, &accountDataJSONStr,
				&appParamIndexes, &appParams, &appCreatedBytes, &appClosedBytes, &appDeletedBytes, &localStateAppIds, &localStates,
				&localStateCreatedBytes, &localStateClosedBytes, &localStateDeletedBytes,
			)
		}
		if err != nil {
			err = fmt.Errorf("account scan err %v", err)
			req.out <- idb.AccountRow{Error: err}
			break
		}

		var account models.Account
		var aaddr basics.Address
		copy(aaddr[:], addr)
		account.Address = aaddr.String()
		account.Round = uint64(req.blockheader.Round)
		account.AmountWithoutPendingRewards = microalgos
		account.Rewards = rewardstotal
		account.CreatedAtRound = nullableInt64Ptr(createdat)
		account.ClosedAtRound = nullableInt64Ptr(closedat)
		account.Deleted = nullableBoolPtr(deleted)
		account.RewardBase = new(uint64)
		*account.RewardBase = rewardsbase
		// default to Offline in there have been no keyreg transactions.
		account.Status = statusStrings[offlineStatusIdx]
		if keytype != nil && *keytype != "" {
			account.SigType = keytype
		}

		{
			var ad basics.AccountData
			ad, err = encoding.DecodeTrimmedAccountData(accountDataJSONStr)
			if err != nil {
				err = fmt.Errorf("account decode err (%s) %v", accountDataJSONStr, err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			account.Status = statusStrings[ad.Status]
			hasSel := !allZero(ad.SelectionID[:])
			hasVote := !allZero(ad.VoteID[:])
			if hasSel || hasVote {
				part := new(models.AccountParticipation)
				if hasSel {
					part.SelectionParticipationKey = ad.SelectionID[:]
				}
				if hasVote {
					part.VoteParticipationKey = ad.VoteID[:]
				}
				part.VoteFirstValid = uint64(ad.VoteFirstValid)
				part.VoteLastValid = uint64(ad.VoteLastValid)
				part.VoteKeyDilution = ad.VoteKeyDilution
				account.Participation = part
			}

			if !ad.AuthAddr.IsZero() {
				var spendingkey basics.Address
				copy(spendingkey[:], ad.AuthAddr[:])
				account.AuthAddr = stringPtr(spendingkey.String())
			}

			{
				totalSchema := models.ApplicationStateSchema{
					NumByteSlice: ad.TotalAppSchema.NumByteSlice,
					NumUint:      ad.TotalAppSchema.NumUint,
				}
				if totalSchema != (models.ApplicationStateSchema{}) {
					account.AppsTotalSchema = &totalSchema
				}
			}
			if ad.TotalExtraAppPages != 0 {
				account.AppsTotalExtraPages = uint64Ptr(uint64(ad.TotalExtraAppPages))
			}
		}

		if account.Status == "NotParticipating" {
			account.PendingRewards = 0
		} else {
			// TODO: pending rewards calculation doesn't belong in database layer (this is just the most covenient place which has all the data)
			proto, ok := config.Consensus[req.blockheader.CurrentProtocol]
			if !ok {
				err = fmt.Errorf("get protocol err (%s)", req.blockheader.CurrentProtocol)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			rewardsUnits := uint64(0)
			if proto.RewardUnit != 0 {
				rewardsUnits = microalgos / proto.RewardUnit
			}
			rewardsDelta := req.blockheader.RewardsLevel - rewardsbase
			account.PendingRewards = rewardsUnits * rewardsDelta
		}
		account.Amount = microalgos + account.PendingRewards
		// not implemented: account.Rewards sum of all rewards ever

		const nullarraystr = "[null]"

		if len(holdingAssetids) > 0 && string(holdingAssetids) != nullarraystr {
			var haids []uint64
			err = encoding.DecodeJSON(holdingAssetids, &haids)
			if err != nil {
				err = fmt.Errorf("parsing json holding asset ids err %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var hamounts []uint64
			err = encoding.DecodeJSON(holdingAmount, &hamounts)
			if err != nil {
				err = fmt.Errorf("parsing json holding amounts err %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var hfrozen []bool
			err = encoding.DecodeJSON(holdingFrozen, &hfrozen)
			if err != nil {
				err = fmt.Errorf("parsing json holding frozen err %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var holdingCreated []*uint64
			err = encoding.DecodeJSON(holdingCreatedBytes, &holdingCreated)
			if err != nil {
				err = fmt.Errorf("parsing json holding created ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var holdingClosed []*uint64
			err = encoding.DecodeJSON(holdingClosedBytes, &holdingClosed)
			if err != nil {
				err = fmt.Errorf("parsing json holding closed ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var holdingDeleted []*bool
			err = encoding.DecodeJSON(holdingDeletedBytes, &holdingDeleted)
			if err != nil {
				err = fmt.Errorf("parsing json holding deleted ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}

			if len(hamounts) != len(haids) || len(hfrozen) != len(haids) || len(holdingCreated) != len(haids) || len(holdingClosed) != len(haids) || len(holdingDeleted) != len(haids) {
				err = fmt.Errorf("account asset holding unpacking, all should be %d:  %d amounts, %d frozen, %d created, %d closed, %d deleted",
					len(haids), len(hamounts), len(hfrozen), len(holdingCreated), len(holdingClosed), len(holdingDeleted))
				req.out <- idb.AccountRow{Error: err}
				break
			}

			av := make([]models.AssetHolding, 0, len(haids))
			for i, assetid := range haids {
				// SQL can result in cross-product duplication when account has both asset holdings and assets created, de-dup here
				dup := false
				for _, xaid := range haids[:i] {
					if assetid == xaid {
						dup = true
						break
					}
				}
				if dup {
					continue
				}
				tah := models.AssetHolding{
					Amount:          hamounts[i],
					IsFrozen:        hfrozen[i],
					AssetId:         assetid,
					OptedOutAtRound: holdingClosed[i],
					OptedInAtRound:  holdingCreated[i],
					Deleted:         holdingDeleted[i],
				} // TODO: set Creator to asset creator addr string
				av = append(av, tah)
			}
			account.Assets = new([]models.AssetHolding)
			*account.Assets = av
		}
		if len(assetParamsIds) > 0 && string(assetParamsIds) != nullarraystr {
			var assetids []uint64
			err = encoding.DecodeJSON(assetParamsIds, &assetids)
			if err != nil {
				err = fmt.Errorf("parsing json asset param ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			assetParams, err := encoding.DecodeAssetParamsArray(assetParamsStr)
			if err != nil {
				err = fmt.Errorf("parsing json asset param string, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var assetCreated []*uint64
			err = encoding.DecodeJSON(assetParamsCreatedBytes, &assetCreated)
			if err != nil {
				err = fmt.Errorf("parsing json asset created ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var assetClosed []*uint64
			err = encoding.DecodeJSON(assetParamsClosedBytes, &assetClosed)
			if err != nil {
				err = fmt.Errorf("parsing json asset closed ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var assetDeleted []*bool
			err = encoding.DecodeJSON(assetParamsDeletedBytes, &assetDeleted)
			if err != nil {
				err = fmt.Errorf("parsing json asset deleted ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}

			if len(assetParams) != len(assetids) || len(assetCreated) != len(assetids) || len(assetClosed) != len(assetids) || len(assetDeleted) != len(assetids) {
				err = fmt.Errorf("account asset unpacking, all should be %d:  %d assetids, %d created, %d closed, %d deleted",
					len(assetParams), len(assetids), len(assetCreated), len(assetClosed), len(assetDeleted))
				req.out <- idb.AccountRow{Error: err}
				break
			}

			cal := make([]models.Asset, 0, len(assetids))
			for i, assetid := range assetids {
				// SQL can result in cross-product duplication when account has both asset holdings and assets created, de-dup here
				dup := false
				for _, xaid := range assetids[:i] {
					if assetid == xaid {
						dup = true
						break
					}
				}
				if dup {
					continue
				}
				ap := assetParams[i]

				tma := models.Asset{
					Index:            assetid,
					CreatedAtRound:   assetCreated[i],
					DestroyedAtRound: assetClosed[i],
					Deleted:          assetDeleted[i],
					Params: models.AssetParams{
						Creator:       account.Address,
						Total:         ap.Total,
						Decimals:      uint64(ap.Decimals),
						DefaultFrozen: boolPtr(ap.DefaultFrozen),
						UnitName:      stringPtr(util.PrintableUTF8OrEmpty(ap.UnitName)),
						UnitNameB64:   byteSlicePtr([]byte(ap.UnitName)),
						Name:          stringPtr(util.PrintableUTF8OrEmpty(ap.AssetName)),
						NameB64:       byteSlicePtr([]byte(ap.AssetName)),
						Url:           stringPtr(util.PrintableUTF8OrEmpty(ap.URL)),
						UrlB64:        byteSlicePtr([]byte(ap.URL)),
						MetadataHash:  byteSliceOmitZeroPtr(ap.MetadataHash[:]),
						Manager:       addrStr(ap.Manager),
						Reserve:       addrStr(ap.Reserve),
						Freeze:        addrStr(ap.Freeze),
						Clawback:      addrStr(ap.Clawback),
					},
				}
				cal = append(cal, tma)
			}
			account.CreatedAssets = new([]models.Asset)
			*account.CreatedAssets = cal
		}

		if len(appParamIndexes) > 0 {
			// apps owned by this account
			var appIds []uint64
			err = encoding.DecodeJSON(appParamIndexes, &appIds)
			if err != nil {
				err = fmt.Errorf("parsing json appids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var appCreated []*uint64
			err = encoding.DecodeJSON(appCreatedBytes, &appCreated)
			if err != nil {
				err = fmt.Errorf("parsing json app created ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var appClosed []*uint64
			err = encoding.DecodeJSON(appClosedBytes, &appClosed)
			if err != nil {
				err = fmt.Errorf("parsing json app closed ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var appDeleted []*bool
			err = encoding.DecodeJSON(appDeletedBytes, &appDeleted)
			if err != nil {
				err = fmt.Errorf("parsing json app deleted flags, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}

			apps, err := encoding.DecodeAppParamsArray(appParams)
			if err != nil {
				err = fmt.Errorf("parsing json appparams, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			if len(appIds) != len(apps) || len(appClosed) != len(apps) || len(appCreated) != len(apps) || len(appDeleted) != len(apps) {
				err = fmt.Errorf("account app unpacking, all should be %d:  %d appids, %d appClosed, %d appCreated, %d appDeleted", len(apps), len(appIds), len(appClosed), len(appCreated), len(appDeleted))
				req.out <- idb.AccountRow{Error: err}
				break
			}

			aout := make([]models.Application, len(appIds))
			outpos := 0
			for i, appid := range appIds {
				aout[outpos].Id = appid
				aout[outpos].CreatedAtRound = appCreated[i]
				aout[outpos].DeletedAtRound = appClosed[i]
				aout[outpos].Deleted = appDeleted[i]
				aout[outpos].Params.Creator = &account.Address

				// If these are both nil the app was probably deleted, leave out params
				// some "required" fields will be left in the results.
				if apps[i].ApprovalProgram != nil || apps[i].ClearStateProgram != nil {
					aout[outpos].Params.ApprovalProgram = apps[i].ApprovalProgram
					aout[outpos].Params.ClearStateProgram = apps[i].ClearStateProgram
					aout[outpos].Params.GlobalState = tealKeyValueToModel(apps[i].GlobalState)
					aout[outpos].Params.GlobalStateSchema = &models.ApplicationStateSchema{
						NumByteSlice: apps[i].GlobalStateSchema.NumByteSlice,
						NumUint:      apps[i].GlobalStateSchema.NumUint,
					}
					aout[outpos].Params.LocalStateSchema = &models.ApplicationStateSchema{
						NumByteSlice: apps[i].LocalStateSchema.NumByteSlice,
						NumUint:      apps[i].LocalStateSchema.NumUint,
					}
					if apps[i].ExtraProgramPages > 0 {
						epp := uint64(apps[i].ExtraProgramPages)
						aout[outpos].Params.ExtraProgramPages = &epp
					}
				}

				outpos++
			}
			if outpos != len(aout) {
				aout = aout[:outpos]
			}
			account.CreatedApps = &aout
		}

		if len(localStateAppIds) > 0 {
			var appIds []uint64
			err = encoding.DecodeJSON(localStateAppIds, &appIds)
			if err != nil {
				err = fmt.Errorf("parsing json local appids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var appCreated []*uint64
			err = encoding.DecodeJSON(localStateCreatedBytes, &appCreated)
			if err != nil {
				err = fmt.Errorf("parsing json ls created ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var appClosed []*uint64
			err = encoding.DecodeJSON(localStateClosedBytes, &appClosed)
			if err != nil {
				err = fmt.Errorf("parsing json ls closed ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var appDeleted []*bool
			err = encoding.DecodeJSON(localStateDeletedBytes, &appDeleted)
			if err != nil {
				err = fmt.Errorf("parsing json ls closed ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			ls, err := encoding.DecodeAppLocalStateArray(localStates)
			if err != nil {
				err = fmt.Errorf("parsing json local states, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			if len(appIds) != len(ls) || len(appClosed) != len(ls) || len(appCreated) != len(ls) || len(appDeleted) != len(ls) {
				err = fmt.Errorf("account app unpacking, all should be %d:  %d appids, %d appClosed, %d appCreated, %d appDeleted", len(ls), len(appIds), len(appClosed), len(appCreated), len(appDeleted))
				req.out <- idb.AccountRow{Error: err}
				break
			}

			aout := make([]models.ApplicationLocalState, len(ls))
			for i, appid := range appIds {
				aout[i].Id = appid
				aout[i].OptedInAtRound = appCreated[i]
				aout[i].ClosedOutAtRound = appClosed[i]
				aout[i].Deleted = appDeleted[i]
				aout[i].Schema = models.ApplicationStateSchema{
					NumByteSlice: ls[i].Schema.NumByteSlice,
					NumUint:      ls[i].Schema.NumUint,
				}
				aout[i].KeyValue = tealKeyValueToModel(ls[i].KeyValue)
			}
			account.AppsLocalState = &aout
		}

		req.out <- idb.AccountRow{Account: account}
		count++
		if req.opts.Limit != 0 && count >= req.opts.Limit {
			return
		}
	}
	if err := req.rows.Err(); err != nil {
		err = fmt.Errorf("error reading rows: %v", err)
		req.out <- idb.AccountRow{Error: err}
	}
}

func nullableInt64Ptr(x sql.NullInt64) *uint64 {
	if !x.Valid {
		return nil
	}
	return uint64Ptr(uint64(x.Int64))
}

func nullableBoolPtr(x sql.NullBool) *bool {
	if !x.Valid {
		return nil
	}
	return &x.Bool
}

func uintOrDefault(x *uint64) uint64 {
	if x != nil {
		return *x
	}
	return 0
}

func uint64Ptr(x uint64) *uint64 {
	out := new(uint64)
	*out = x
	return out
}

func boolPtr(x bool) *bool {
	out := new(bool)
	*out = x
	return out
}

func stringPtr(x string) *string {
	if len(x) == 0 {
		return nil
	}
	out := new(string)
	*out = x
	return out
}

func byteSlicePtr(x []byte) *[]byte {
	if len(x) == 0 {
		return nil
	}

	xx := make([]byte, len(x))
	copy(xx, x)
	return &xx
}

func byteSliceOmitZeroPtr(x []byte) *[]byte {
	allzero := true
	for _, b := range x {
		if b != 0 {
			allzero = false
			break
		}
	}
	if allzero {
		return nil
	}

	xx := make([]byte, len(x))
	copy(xx, x)
	return &xx
}

func allZero(x []byte) bool {
	for _, v := range x {
		if v != 0 {
			return false
		}
	}
	return true
}

func addrStr(addr basics.Address) *string {
	if addr.IsZero() {
		return nil
	}
	out := new(string)
	*out = addr.String()
	return out
}

type getAccountsRequest struct {
	opts        idb.AccountQueryOptions
	blockheader bookkeeping.BlockHeader
	query       string
	rows        pgx.Rows
	out         chan idb.AccountRow
	start       time.Time
}

// GetAccounts is part of idb.IndexerDB
func (db *IndexerDb) GetAccounts(ctx context.Context, opts idb.AccountQueryOptions) (<-chan idb.AccountRow, uint64) {
	out := make(chan idb.AccountRow, 1)

	if opts.HasAssetID != 0 {
		opts.IncludeAssetHoldings = true
	} else if (opts.AssetGT != nil) || (opts.AssetLT != nil) {
		err := fmt.Errorf("AssetGT=%d, AssetLT=%d, but HasAssetID=%d", uintOrDefault(opts.AssetGT), uintOrDefault(opts.AssetLT), opts.HasAssetID)
		out <- idb.AccountRow{Error: err}
		close(out)
		return out, 0
	}

	// Begin transaction so we get everything at one consistent point in time and round of accounting.
	tx, err := db.db.BeginTx(ctx, readonlyRepeatableRead)
	if err != nil {
		err = fmt.Errorf("account tx err %v", err)
		out <- idb.AccountRow{Error: err}
		close(out)
		return out, 0
	}

	// Get round number through which accounting has been updated
	round, err := db.getMaxRoundAccounted(ctx, tx)
	if err != nil {
		err = fmt.Errorf("account round err %v", err)
		out <- idb.AccountRow{Error: err}
		close(out)
		tx.Rollback(ctx)
		return out, round
	}

	// Get block header for that round so we know protocol and rewards info
	row := tx.QueryRow(ctx, `SELECT header FROM block_header WHERE round = $1`, round)
	var headerjson []byte
	err = row.Scan(&headerjson)
	if err != nil {
		err = fmt.Errorf("account round header %d err %v", round, err)
		out <- idb.AccountRow{Error: err}
		close(out)
		tx.Rollback(ctx)
		return out, round
	}
	blockheader, err := encoding.DecodeBlockHeader(headerjson)
	if err != nil {
		err = fmt.Errorf("account round header %d err %v", round, err)
		out <- idb.AccountRow{Error: err}
		close(out)
		tx.Rollback(ctx)
		return out, round
	}

	// Construct query for fetching accounts...
	query, whereArgs := db.buildAccountQuery(opts)
	req := &getAccountsRequest{
		opts:        opts,
		blockheader: blockheader,
		query:       query,
		out:         out,
		start:       time.Now(),
	}
	req.rows, err = tx.Query(ctx, query, whereArgs...)
	if err != nil {
		err = fmt.Errorf("account query %#v err %v", query, err)
		out <- idb.AccountRow{Error: err}
		close(out)
		tx.Rollback(ctx)
		return out, round
	}
	go func() {
		db.yieldAccountsThread(req)
		close(req.out)
		tx.Rollback(ctx)
	}()
	return out, round
}

func (db *IndexerDb) buildAccountQuery(opts idb.AccountQueryOptions) (query string, whereArgs []interface{}) {
	// Construct query for fetching accounts...
	const maxWhereParts = 14
	whereParts := make([]string, 0, maxWhereParts)
	whereArgs = make([]interface{}, 0, maxWhereParts)
	partNumber := 1
	withClauses := make([]string, 0, maxWhereParts)
	// filter by has-asset or has-app
	if opts.HasAssetID != 0 {
		aq := fmt.Sprintf("SELECT addr FROM account_asset WHERE assetid = $%d", partNumber)
		whereArgs = append(whereArgs, opts.HasAssetID)
		partNumber++
		if opts.AssetGT != nil {
			aq += fmt.Sprintf(" AND amount > $%d", partNumber)
			whereArgs = append(whereArgs, *opts.AssetGT)
			partNumber++
		}
		if opts.AssetLT != nil {
			aq += fmt.Sprintf(" AND amount < $%d", partNumber)
			whereArgs = append(whereArgs, *opts.AssetLT)
			partNumber++
		}
		aq = "qasf AS (" + aq + ")"
		withClauses = append(withClauses, aq)
	}
	if opts.HasAppID != 0 {
		withClauses = append(withClauses, fmt.Sprintf("qapf AS (SELECT addr FROM account_app WHERE app = $%d)", partNumber))
		whereArgs = append(whereArgs, opts.HasAppID)
		partNumber++
	}
	// filters against main account table
	if len(opts.GreaterThanAddress) > 0 {
		whereParts = append(whereParts, fmt.Sprintf("a.addr > $%d", partNumber))
		whereArgs = append(whereArgs, opts.GreaterThanAddress)
		partNumber++
	}
	if len(opts.EqualToAddress) > 0 {
		whereParts = append(whereParts, fmt.Sprintf("a.addr = $%d", partNumber))
		whereArgs = append(whereArgs, opts.EqualToAddress)
		partNumber++
	}
	if opts.AlgosGreaterThan != nil {
		whereParts = append(whereParts, fmt.Sprintf("a.microalgos > $%d", partNumber))
		whereArgs = append(whereArgs, *opts.AlgosGreaterThan)
		partNumber++
	}
	if opts.AlgosLessThan != nil {
		whereParts = append(whereParts, fmt.Sprintf("a.microalgos < $%d", partNumber))
		whereArgs = append(whereArgs, *opts.AlgosLessThan)
		partNumber++
	}
	if !opts.IncludeDeleted {
		whereParts = append(whereParts, "coalesce(a.deleted, false) = false")
	}
	if len(opts.EqualToAuthAddr) > 0 {
		whereParts = append(whereParts, fmt.Sprintf("a.account_data ->> 'spend' = $%d", partNumber))
		whereArgs = append(whereArgs, encoding.Base64(opts.EqualToAuthAddr))
		partNumber++
	}
	query = `SELECT a.addr, a.microalgos, a.rewards_total, a.created_at, a.closed_at, a.deleted, a.rewardsbase, a.keytype, a.account_data FROM account a`
	if opts.HasAssetID != 0 {
		// inner join requires match, filtering on presence of asset
		query += " JOIN qasf ON a.addr = qasf.addr"
	}
	if opts.HasAppID != 0 {
		// inner join requires match, filtering on presence of app
		query += " JOIN qapf ON a.addr = qapf.addr"
	}
	if len(whereParts) > 0 {
		whereStr := strings.Join(whereParts, " AND ")
		query += " WHERE " + whereStr
	}
	query += " ORDER BY a.addr ASC"
	if opts.Limit != 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}
	// TODO: asset holdings and asset params are optional, but practically always used. Either make them actually always on, or make app-global and app-local clauses also optional (they are currently always on).
	withClauses = append(withClauses, "qaccounts AS ("+query+")")
	query = "WITH " + strings.Join(withClauses, ", ")
	if opts.IncludeDeleted {
		if opts.IncludeAssetHoldings {
			query += `, qaa AS (SELECT xa.addr, json_agg(aa.assetid) as haid, json_agg(aa.amount) as hamt, json_agg(aa.frozen) as hf, json_agg(aa.created_at) as holding_created_at, json_agg(aa.closed_at) as holding_closed_at, json_agg(coalesce(aa.deleted, false)) as holding_deleted FROM account_asset aa JOIN qaccounts xa ON aa.addr = xa.addr GROUP BY 1)`
		}
		if opts.IncludeAssetParams {
			query += `, qap AS (SELECT ya.addr, json_agg(ap.index) as paid, json_agg(ap.params) as pp, json_agg(ap.created_at) as asset_created_at, json_agg(ap.closed_at) as asset_closed_at, json_agg(ap.deleted) as asset_deleted FROM asset ap JOIN qaccounts ya ON ap.creator_addr = ya.addr GROUP BY 1)`
		}
		// app
		query += `, qapp AS (SELECT app.creator as addr, json_agg(app.index) as papps, json_agg(app.params) as ppa, json_agg(app.created_at) as app_created_at, json_agg(app.closed_at) as app_closed_at, json_agg(app.deleted) as app_deleted FROM app JOIN qaccounts ON qaccounts.addr = app.creator GROUP BY 1)`
		// app localstate
		query += `, qls AS (SELECT la.addr, json_agg(la.app) as lsapps, json_agg(la.localstate) as lsls, json_agg(la.created_at) as ls_created_at, json_agg(la.closed_at) as ls_closed_at, json_agg(la.deleted) as ls_deleted FROM account_app la JOIN qaccounts ON qaccounts.addr = la.addr GROUP BY 1)`
	} else {
		if opts.IncludeAssetHoldings {
			query += `, qaa AS (SELECT xa.addr, json_agg(aa.assetid) as haid, json_agg(aa.amount) as hamt, json_agg(aa.frozen) as hf, json_agg(aa.created_at) as holding_created_at, json_agg(aa.closed_at) as holding_closed_at, json_agg(coalesce(aa.deleted, false)) as holding_deleted FROM account_asset aa JOIN qaccounts xa ON aa.addr = xa.addr WHERE coalesce(aa.deleted, false) = false GROUP BY 1)`
		}
		if opts.IncludeAssetParams {
			query += `, qap AS (SELECT ya.addr, json_agg(ap.index) as paid, json_agg(ap.params) as pp, json_agg(ap.created_at) as asset_created_at, json_agg(ap.closed_at) as asset_closed_at, json_agg(ap.deleted) as asset_deleted FROM asset ap JOIN qaccounts ya ON ap.creator_addr = ya.addr WHERE coalesce(ap.deleted, false) = false GROUP BY 1)`
		}
		// app
		query += `, qapp AS (SELECT app.creator as addr, json_agg(app.index) as papps, json_agg(app.params) as ppa, json_agg(app.created_at) as app_created_at, json_agg(app.closed_at) as app_closed_at, json_agg(app.deleted) as app_deleted FROM app JOIN qaccounts ON qaccounts.addr = app.creator WHERE coalesce(app.deleted, false) = false GROUP BY 1)`
		// app localstate
		query += `, qls AS (SELECT la.addr, json_agg(la.app) as lsapps, json_agg(la.localstate) as lsls, json_agg(la.created_at) as ls_created_at, json_agg(la.closed_at) as ls_closed_at, json_agg(la.deleted) as ls_deleted FROM account_app la JOIN qaccounts ON qaccounts.addr = la.addr WHERE coalesce(la.deleted, false) = false GROUP BY 1)`
	}

	// query results
	query += ` SELECT za.addr, za.microalgos, za.rewards_total, za.created_at, za.closed_at, za.deleted, za.rewardsbase, za.keytype, za.account_data`
	if opts.IncludeAssetHoldings {
		query += `, qaa.haid, qaa.hamt, qaa.hf, qaa.holding_created_at, qaa.holding_closed_at, qaa.holding_deleted`
	}
	if opts.IncludeAssetParams {
		query += `, qap.paid, qap.pp, qap.asset_created_at, qap.asset_closed_at, qap.asset_deleted`
	}
	query += `, qapp.papps, qapp.ppa, qapp.app_created_at, qapp.app_closed_at, qapp.app_deleted, qls.lsapps, qls.lsls, qls.ls_created_at, qls.ls_closed_at, qls.ls_deleted FROM qaccounts za`

	// join everything together
	if opts.IncludeAssetHoldings {
		query += ` LEFT JOIN qaa ON za.addr = qaa.addr`
	}
	if opts.IncludeAssetParams {
		query += ` LEFT JOIN qap ON za.addr = qap.addr`
	}
	query += " LEFT JOIN qapp ON za.addr = qapp.addr LEFT JOIN qls ON qls.addr = za.addr ORDER BY za.addr ASC;"
	return query, whereArgs
}

// Assets is part of idb.IndexerDB
func (db *IndexerDb) Assets(ctx context.Context, filter idb.AssetsQuery) (<-chan idb.AssetRow, uint64) {
	query := `SELECT index, creator_addr, params, created_at, closed_at, deleted FROM asset a`
	const maxWhereParts = 14
	whereParts := make([]string, 0, maxWhereParts)
	whereArgs := make([]interface{}, 0, maxWhereParts)
	partNumber := 1
	if filter.AssetID != 0 {
		whereParts = append(whereParts, fmt.Sprintf("a.index = $%d", partNumber))
		whereArgs = append(whereArgs, filter.AssetID)
		partNumber++
	}
	if filter.AssetIDGreaterThan != 0 {
		whereParts = append(whereParts, fmt.Sprintf("a.index > $%d", partNumber))
		whereArgs = append(whereArgs, filter.AssetIDGreaterThan)
		partNumber++
	}
	if filter.Creator != nil {
		whereParts = append(whereParts, fmt.Sprintf("a.creator_addr = $%d", partNumber))
		whereArgs = append(whereArgs, filter.Creator)
		partNumber++
	}
	if filter.Name != "" {
		whereParts = append(whereParts, fmt.Sprintf("a.params ->> 'an' ILIKE $%d", partNumber))
		whereArgs = append(whereArgs, "%"+filter.Name+"%")
		partNumber++
	}
	if filter.Unit != "" {
		whereParts = append(whereParts, fmt.Sprintf("a.params ->> 'un' ILIKE $%d", partNumber))
		whereArgs = append(whereArgs, "%"+filter.Unit+"%")
		partNumber++
	}
	if filter.Query != "" {
		qs := "%" + filter.Query + "%"
		whereParts = append(whereParts, fmt.Sprintf("(a.params ->> 'un' ILIKE $%d OR a.params ->> 'an' ILIKE $%d)", partNumber, partNumber))
		whereArgs = append(whereArgs, qs)
		partNumber++
	}
	if !filter.IncludeDeleted {
		whereParts = append(whereParts, "coalesce(a.deleted, false) = false")
	}
	if len(whereParts) > 0 {
		whereStr := strings.Join(whereParts, " AND ")
		query += " WHERE " + whereStr
	}
	query += " ORDER BY index ASC"
	if filter.Limit != 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}

	out := make(chan idb.AssetRow, 1)

	tx, err := db.db.BeginTx(ctx, readonlyRepeatableRead)
	if err != nil {
		out <- idb.AssetRow{Error: err}
		close(out)
		return out, 0
	}

	round, err := db.getMaxRoundAccounted(ctx, tx)
	if err != nil {
		out <- idb.AssetRow{Error: err}
		close(out)
		tx.Rollback(ctx)
		return out, round
	}

	rows, err := tx.Query(ctx, query, whereArgs...)
	if err != nil {
		err = fmt.Errorf("asset query %#v err %v", query, err)
		out <- idb.AssetRow{Error: err}
		close(out)
		tx.Rollback(ctx)
		return out, round
	}
	go func() {
		db.yieldAssetsThread(filter, rows, out)
		close(out)
		tx.Rollback(ctx)
	}()
	return out, round
}

func (db *IndexerDb) yieldAssetsThread(filter idb.AssetsQuery, rows pgx.Rows, out chan<- idb.AssetRow) {
	defer rows.Close()

	for rows.Next() {
		var index uint64
		var creatorAddr []byte
		var paramsJSONStr []byte
		var created *uint64
		var closed *uint64
		var deleted *bool
		var err error

		err = rows.Scan(&index, &creatorAddr, &paramsJSONStr, &created, &closed, &deleted)
		if err != nil {
			out <- idb.AssetRow{Error: err}
			break
		}
		params, err := encoding.DecodeAssetParams(paramsJSONStr)
		if err != nil {
			out <- idb.AssetRow{Error: err}
			break
		}
		rec := idb.AssetRow{
			AssetID:      index,
			Creator:      creatorAddr,
			Params:       params,
			CreatedRound: created,
			ClosedRound:  closed,
			Deleted:      deleted,
		}
		out <- rec
	}
	if err := rows.Err(); err != nil {
		out <- idb.AssetRow{Error: err}
	}
}

// AssetBalances is part of idb.IndexerDB
func (db *IndexerDb) AssetBalances(ctx context.Context, abq idb.AssetBalanceQuery) (<-chan idb.AssetBalanceRow, uint64) {
	const maxWhereParts = 14
	whereParts := make([]string, 0, maxWhereParts)
	whereArgs := make([]interface{}, 0, maxWhereParts)
	partNumber := 1
	if abq.AssetID != 0 {
		whereParts = append(whereParts, fmt.Sprintf("aa.assetid = $%d", partNumber))
		whereArgs = append(whereArgs, abq.AssetID)
		partNumber++
	}
	if abq.AmountGT != nil {
		whereParts = append(whereParts, fmt.Sprintf("aa.amount > $%d", partNumber))
		whereArgs = append(whereArgs, *abq.AmountGT)
		partNumber++
	}
	if abq.AmountLT != nil {
		whereParts = append(whereParts, fmt.Sprintf("aa.amount < $%d", partNumber))
		whereArgs = append(whereArgs, *abq.AmountLT)
		partNumber++
	}
	if len(abq.PrevAddress) != 0 {
		whereParts = append(whereParts, fmt.Sprintf("aa.addr > $%d", partNumber))
		whereArgs = append(whereArgs, abq.PrevAddress)
		partNumber++
	}
	if !abq.IncludeDeleted {
		whereParts = append(whereParts, "coalesce(aa.deleted, false) = false")
	}
	query := `SELECT addr, assetid, amount, frozen, created_at, closed_at, deleted FROM account_asset aa`
	if len(whereParts) > 0 {
		query += " WHERE " + strings.Join(whereParts, " AND ")
	}
	query += " ORDER BY addr ASC"
	if abq.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", abq.Limit)
	}

	out := make(chan idb.AssetBalanceRow, 1)

	tx, err := db.db.BeginTx(ctx, readonlyRepeatableRead)
	if err != nil {
		out <- idb.AssetBalanceRow{Error: err}
		close(out)
		return out, 0
	}

	round, err := db.getMaxRoundAccounted(ctx, tx)
	if err != nil {
		out <- idb.AssetBalanceRow{Error: err}
		close(out)
		tx.Rollback(ctx)
		return out, round
	}

	rows, err := tx.Query(ctx, query, whereArgs...)
	if err != nil {
		out <- idb.AssetBalanceRow{Error: err}
		close(out)
		tx.Rollback(ctx)
		return out, round
	}
	go func() {
		db.yieldAssetBalanceThread(rows, out)
		close(out)
		tx.Rollback(ctx)
	}()
	return out, round
}

func (db *IndexerDb) yieldAssetBalanceThread(rows pgx.Rows, out chan<- idb.AssetBalanceRow) {
	defer rows.Close()

	for rows.Next() {
		var addr []byte
		var assetID uint64
		var amount uint64
		var frozen bool
		var created *uint64
		var closed *uint64
		var deleted *bool
		err := rows.Scan(&addr, &assetID, &amount, &frozen, &created, &closed, &deleted)
		if err != nil {
			out <- idb.AssetBalanceRow{Error: err}
			break
		}
		rec := idb.AssetBalanceRow{
			Address:      addr,
			AssetID:      assetID,
			Amount:       amount,
			Frozen:       frozen,
			ClosedRound:  closed,
			CreatedRound: created,
			Deleted:      deleted,
		}
		out <- rec
	}
	if err := rows.Err(); err != nil {
		out <- idb.AssetBalanceRow{Error: err}
	}
}

// Applications is part of idb.IndexerDB
func (db *IndexerDb) Applications(ctx context.Context, filter *models.SearchForApplicationsParams) (<-chan idb.ApplicationRow, uint64) {
	out := make(chan idb.ApplicationRow, 1)
	if filter == nil {
		out <- idb.ApplicationRow{Error: fmt.Errorf("no arguments provided to application search")}
		close(out)
		return out, 0
	}

	query := `SELECT index, creator, params, created_at, closed_at, deleted FROM app `

	const maxWhereParts = 30
	whereParts := make([]string, 0, maxWhereParts)
	whereArgs := make([]interface{}, 0, maxWhereParts)
	partNumber := 1
	if filter.ApplicationId != nil {
		whereParts = append(whereParts, fmt.Sprintf("index = $%d", partNumber))
		whereArgs = append(whereArgs, *filter.ApplicationId)
		partNumber++
	}
	if filter.Next != nil {
		whereParts = append(whereParts, fmt.Sprintf("index > $%d", partNumber))
		whereArgs = append(whereArgs, *filter.Next)
		partNumber++
	}
	if filter.IncludeAll == nil || !(*filter.IncludeAll) {
		whereParts = append(whereParts, "coalesce(deleted, false) = false")
	}
	if len(whereParts) > 0 {
		whereStr := strings.Join(whereParts, " AND ")
		query += " WHERE " + whereStr
	}
	query += " ORDER BY 1"
	if filter.Limit != nil {
		query += fmt.Sprintf(" LIMIT %d", *filter.Limit)
	}

	tx, err := db.db.BeginTx(ctx, readonlyRepeatableRead)
	if err != nil {
		out <- idb.ApplicationRow{Error: err}
		close(out)
		return out, 0
	}

	round, err := db.getMaxRoundAccounted(ctx, tx)
	if err != nil {
		out <- idb.ApplicationRow{Error: err}
		close(out)
		tx.Rollback(ctx)
		return out, round
	}

	rows, err := tx.Query(ctx, query, whereArgs...)
	if err != nil {
		out <- idb.ApplicationRow{Error: err}
		close(out)
		tx.Rollback(ctx)
		return out, round
	}

	go func() {
		db.yieldApplicationsThread(rows, out)
		close(out)
		tx.Rollback(ctx)
	}()
	return out, round
}

func (db *IndexerDb) yieldApplicationsThread(rows pgx.Rows, out chan idb.ApplicationRow) {
	defer rows.Close()

	for rows.Next() {
		var index uint64
		var creator []byte
		var paramsjson []byte
		var created *uint64
		var closed *uint64
		var deleted *bool
		err := rows.Scan(&index, &creator, &paramsjson, &created, &closed, &deleted)
		if err != nil {
			out <- idb.ApplicationRow{Error: err}
			break
		}
		var rec idb.ApplicationRow
		rec.Application.Id = index
		rec.Application.CreatedAtRound = created
		rec.Application.DeletedAtRound = closed
		rec.Application.Deleted = deleted
		ap, err := encoding.DecodeAppParams(paramsjson)
		if err != nil {
			rec.Error = fmt.Errorf("app=%d json err, %v", index, err)
			out <- rec
			break
		}
		rec.Application.Params.ApprovalProgram = ap.ApprovalProgram
		rec.Application.Params.ClearStateProgram = ap.ClearStateProgram
		rec.Application.Params.Creator = new(string)

		var aaddr basics.Address
		copy(aaddr[:], creator)
		rec.Application.Params.Creator = new(string)
		*(rec.Application.Params.Creator) = aaddr.String()
		rec.Application.Params.GlobalState = tealKeyValueToModel(ap.GlobalState)
		rec.Application.Params.GlobalStateSchema = &models.ApplicationStateSchema{
			NumByteSlice: ap.GlobalStateSchema.NumByteSlice,
			NumUint:      ap.GlobalStateSchema.NumUint,
		}
		rec.Application.Params.LocalStateSchema = &models.ApplicationStateSchema{
			NumByteSlice: ap.LocalStateSchema.NumByteSlice,
			NumUint:      ap.LocalStateSchema.NumUint,
		}

		if ap.ExtraProgramPages != 0 {
			rec.Application.Params.ExtraProgramPages = new(uint64)
			*rec.Application.Params.ExtraProgramPages = uint64(ap.ExtraProgramPages)
		}

		out <- rec
	}
	if err := rows.Err(); err != nil {
		out <- idb.ApplicationRow{Error: err}
	}
}

// Health is part of idb.IndexerDB
func (db *IndexerDb) Health(ctx context.Context) (idb.Health, error) {
	migrationRequired := false
	migrating := false
	blocking := false
	errString := ""
	var data = make(map[string]interface{})

	if db.readonly {
		data["read-only-mode"] = true
	}

	if db.migration != nil {
		state := db.migration.GetStatus()

		if state.Err != nil {
			errString = state.Err.Error()
		}
		if state.Status != "" {
			data["migration-status"] = state.Status
		}

		migrationRequired = state.Running
		migrating = state.Running
		blocking = state.Blocking
	} else {
		state, err := db.getMigrationState(ctx, nil)
		if err != nil {
			return idb.Health{}, err
		}

		blocking = migrationStateBlocked(state)
		migrationRequired = needsMigration(state)
	}

	data["migration-required"] = migrationRequired

	round, err := db.getMaxRoundAccounted(ctx, nil)

	// We'll just have to set the round to 0
	if err == idb.ErrorNotInitialized {
		err = nil
		round = 0
	}

	return idb.Health{
		Data:        &data,
		Round:       round,
		IsMigrating: migrating,
		DBAvailable: !blocking,
		Error:       errString,
	}, err
}

// GetSpecialAccounts is part of idb.IndexerDB
func (db *IndexerDb) GetSpecialAccounts(ctx context.Context) (transactions.SpecialAddresses, error) {
	cache, err := db.getMetastate(ctx, nil, schema.SpecialAccountsMetastateKey)
	if err != nil {
		return transactions.SpecialAddresses{}, fmt.Errorf("GetSpecialAccounts() err: %w", err)
	}

	accounts, err := encoding.DecodeSpecialAddresses([]byte(cache))
	if err != nil {
		err = fmt.Errorf(
			"GetSpecialAccounts() problem decoding, cache: '%s' err: %w", cache, err)
		return transactions.SpecialAddresses{}, err
	}

	return accounts, nil
}

// GetAccountData returns account data for the given addresses. For accounts that are
// not found, empty AccountData is returned. This function is only used for debugging.
func (db *IndexerDb) GetAccountData(addresses []basics.Address) (map[basics.Address]basics.AccountData, error) {
	tx, err := db.db.BeginTx(context.Background(), readonlyRepeatableRead)
	if err != nil {
		return nil, fmt.Errorf("GetAccountData() begin tx err: %w", err)
	}
	defer tx.Rollback(context.Background())

	l, err := ledger_for_evaluator.MakeLedgerForEvaluator(tx, basics.Round(0))
	if err != nil {
		return nil, fmt.Errorf("GetAccountData() err: %w", err)
	}
	defer l.Close()

	addressesMap := make(map[basics.Address]struct{}, len(addresses))
	for _, address := range addresses {
		addressesMap[address] = struct{}{}
	}

	accountDataMap, err := l.LookupWithoutRewards(addressesMap)
	if err != nil {
		return nil, fmt.Errorf("GetAccountData() err: %w", err)
	}

	res := make(map[basics.Address]basics.AccountData, len(accountDataMap))
	for address, accountData := range accountDataMap {
		if accountData == nil {
			res[address] = basics.AccountData{}
		} else {
			res[address] = *accountData
		}
	}

	return res, nil
}

// GetNetworkState is part of idb.IndexerDB
func (db *IndexerDb) GetNetworkState() (idb.NetworkState, error) {
	state, err := db.getNetworkState(context.Background(), nil)
	if err != nil {
		return idb.NetworkState{}, fmt.Errorf("GetNetworkState() err: %w", err)
	}
	networkState := idb.NetworkState{
		GenesisHash: state.GenesisHash,
	}
	return networkState, nil
}
