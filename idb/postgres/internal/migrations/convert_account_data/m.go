package convertaccountdata

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v4"

	"github.com/algorand/indexer/v3/idb/postgres/internal/encoding"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
)

type aad struct {
	address            sdk.Address
	trimmedAccountData sdk.AccountData
}

func getAccounts(tx pgx.Tx, batchSize uint, lastAddress *sdk.Address) ([]aad, error) {
	var rows pgx.Rows
	var err error
	if lastAddress == nil {
		query :=
			`SELECT addr, account_data FROM account WHERE NOT deleted ORDER BY addr LIMIT $1`
		rows, err = tx.Query(context.Background(), query, batchSize)
	} else {
		query := `SELECT addr, account_data FROM account WHERE NOT deleted AND addr > $1
			ORDER BY addr LIMIT $2`
		rows, err = tx.Query(context.Background(), query, (*lastAddress)[:], batchSize)
	}
	if err != nil {
		return nil, fmt.Errorf("getAccounts() query err: %w", err)
	}

	res := make([]aad, 0, batchSize)
	for rows.Next() {
		var addr []byte
		var accountData []byte
		err = rows.Scan(&addr, &accountData)
		if err != nil {
			return nil, fmt.Errorf("getAccounts() scan err: %w", err)
		}

		res = append(res, aad{})
		e := &res[len(res)-1]
		copy(e.address[:], addr)
		e.trimmedAccountData, err = encoding.DecodeTrimmedAccountData(accountData)
		if err != nil {
			return nil, fmt.Errorf("getAccounts() decode err: %w", err)
		}
	}
	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("getAccounts() rows error err: %w", err)
	}

	return res, nil
}

func computeLcAccountData(tx pgx.Tx, accounts []aad) ([]sdk.AccountData, error) {
	res := make([]sdk.AccountData, 0, len(accounts))
	for i := range accounts {
		res = append(res, accounts[i].trimmedAccountData)
	}

	var batch pgx.Batch
	for i := range accounts {
		batch.Queue(
			"SELECT COUNT(*) FROM account_asset WHERE NOT deleted AND addr = $1",
			accounts[i].address[:])
	}
	for i := range accounts {
		batch.Queue(
			"SELECT COUNT(*) FROM asset WHERE NOT deleted AND creator_addr = $1",
			accounts[i].address[:])
	}
	for i := range accounts {
		batch.Queue(
			"SELECT COUNT(*) FROM app WHERE NOT deleted AND creator = $1",
			accounts[i].address[:])
	}
	for i := range accounts {
		batch.Queue(
			"SELECT COUNT(*) FROM account_app WHERE NOT deleted AND addr = $1",
			accounts[i].address[:])
	}

	results := tx.SendBatch(context.Background(), &batch)
	defer results.Close()

	for i := range accounts {
		err := results.QueryRow().Scan(&res[i].TotalAssets)
		if err != nil {
			return nil, fmt.Errorf("computeLcAccountData() scan total assets err: %w", err)
		}
	}
	for i := range accounts {
		err := results.QueryRow().Scan(&res[i].TotalAssetParams)
		if err != nil {
			return nil, fmt.Errorf("computeLcAccountData() scan total asset params err: %w", err)
		}
	}
	for i := range accounts {
		err := results.QueryRow().Scan(&res[i].TotalAppParams)
		if err != nil {
			return nil, fmt.Errorf("computeLcAccountData() scan total app params err: %w", err)
		}
	}
	for i := range accounts {
		err := results.QueryRow().Scan(&res[i].TotalAppLocalStates)
		if err != nil {
			return nil, fmt.Errorf("computeLcAccountData() scan total app local states err: %w", err)
		}
	}

	err := results.Close()
	if err != nil {
		return nil, fmt.Errorf("computeLcAccountData() close results err: %w", err)
	}

	return res, nil
}

func writeLcAccountData(tx pgx.Tx, accounts []aad, sdkAccountData []sdk.AccountData) error {
	var batch pgx.Batch
	for i := range accounts {
		query := "UPDATE account SET account_data = $1 WHERE addr = $2"
		batch.Queue(
			query, encoding.EncodeTrimmedLcAccountData(sdkAccountData[i]),
			accounts[i].address[:])
	}

	results := tx.SendBatch(context.Background(), &batch)
	// Clean the results off the connection's queue. Without this, weird things happen.
	for i := 0; i < batch.Len(); i++ {
		_, err := results.Exec()
		if err != nil {
			results.Close()
			return fmt.Errorf("writeLcAccountData() exec err: %w", err)
		}
	}
	err := results.Close()
	if err != nil {
		return fmt.Errorf("writeLcAccountData() close results err: %w", err)
	}

	return nil
}

func processAccounts(tx pgx.Tx, accounts []aad) error {
	lcAccountData, err := computeLcAccountData(tx, accounts)
	if err != nil {
		return fmt.Errorf("processAccounts() err: %w", err)
	}

	err = writeLcAccountData(tx, accounts, lcAccountData)
	if err != nil {
		return fmt.Errorf("processAccounts() err: %w", err)
	}

	return nil
}

// RunMigration executes the migration core functionality.
func RunMigration(tx pgx.Tx, batchSize uint) error {
	accounts, err := getAccounts(tx, batchSize, nil)
	if err != nil {
		return fmt.Errorf("RunMigration() err: %w", err)
	}
	err = processAccounts(tx, accounts)
	if err != nil {
		return fmt.Errorf("RunMigration() err: %w", err)
	}

	for uint(len(accounts)) >= batchSize {
		accounts, err = getAccounts(tx, batchSize, &accounts[len(accounts)-1].address)
		if err != nil {
			return fmt.Errorf("RunMigration() err: %w", err)
		}
		err = processAccounts(tx, accounts)
		if err != nil {
			return fmt.Errorf("RunMigration() err: %w", err)
		}
	}

	return nil
}
