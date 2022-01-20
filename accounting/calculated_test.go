package accounting

import (
	"testing"

	"github.com/algorand/indexer/api/generated/v2"
	"github.com/stretchr/testify/assert"
)

func TestMinBalance(t *testing.T) {
	accounts := make([]generated.Account, 1)
	err := Enrich(accounts, MinBalanceEnricher)
	assert.Equal(t, uint64(13371337), accounts[0].MinBalance)
	assert.NoError(t, err)
}
