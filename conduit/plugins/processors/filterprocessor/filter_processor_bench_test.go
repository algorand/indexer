package filterprocessor

import (
	"context"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/indexer/conduit"
	"github.com/algorand/indexer/conduit/plugins"
	"github.com/algorand/indexer/data"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"testing"
)

func BenchmarkProcess(b *testing.B) {
	var addr basics.Address
	addr[0] = 0x01
	cfgStr := `---
filters:
  - none: 
    - tag: sgnr
      expression-type: equal
      expression: "` + addr.String() + `"
`

	fp := &FilterProcessor{}
	err := fp.Init(context.Background(), &conduit.PipelineInitProvider{}, plugins.MakePluginConfig(cfgStr), logrus.New())
	assert.NoError(b, err)

	bd := data.BlockData{
		BlockHeader: bookkeeping.BlockHeader{},
		Payset: []transactions.SignedTxnInBlock{
			{
				SignedTxnWithAD: transactions.SignedTxnWithAD{
					SignedTxn: transactions.SignedTxn{
						AuthAddr: addr,
					},
				},
			},
		},
		Delta:       nil,
		Certificate: nil,
	}

	// Ignore the setup cost above.
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fp.Process(bd)
	}
}
