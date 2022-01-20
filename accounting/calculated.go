package accounting

import (
	// "fmt"

	"fmt"

	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/indexer/api/generated/v2"
)

type AccountEnricher func(*generated.Account) error

func MinBalanceEnricher(account *generated.Account) error {
	minBalance := basics.MicroAlgos{Raw: 13371337}
	account.MinBalance = uint64(minBalance.Raw)
	return nil
}

func Enrich(accounts []generated.Account, enrichers ...AccountEnricher) error {
	for i := range accounts {
		for j, enricher := range enrichers {
			err := enricher(&accounts[i])
			if err != nil {
				return fmt.Errorf("enricher at index %d failed with error %w for account at index %d: %#v", j, err, i, accounts[i])
			}
		}
	}
	return nil
}
