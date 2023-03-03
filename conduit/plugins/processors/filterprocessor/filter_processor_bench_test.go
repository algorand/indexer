package filterprocessor

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/algorand/indexer/conduit"
	"github.com/algorand/indexer/conduit/data"
	"github.com/algorand/indexer/conduit/plugins"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func blockData(addr sdk.Address, numInner int) (block data.BlockData, searchTag string) {
	searchTag = "sgnr"
	block = data.BlockData{
		BlockHeader: sdk.BlockHeader{},
		Payset: []sdk.SignedTxnInBlock{
			{
				SignedTxnWithAD: sdk.SignedTxnWithAD{
					SignedTxn: sdk.SignedTxn{
						AuthAddr: addr,
					},
				},
			},
		},
		Delta:       nil,
		Certificate: nil,
	}

	for i := 0; i < numInner; i++ {
		block.Payset[0].AuthAddr = sdk.ZeroAddress

		innerTxn := sdk.SignedTxnWithAD{
			SignedTxn: sdk.SignedTxn{},
		}
		// match the final inner txn
		if i == numInner-1 {
			innerTxn.SignedTxn.AuthAddr = addr
		}
		block.Payset[0].EvalDelta.InnerTxns = append(block.Payset[0].EvalDelta.InnerTxns, innerTxn)
	}
	return
}

func BenchmarkProcess(b *testing.B) {
	var addr sdk.Address
	addr[0] = 0x01

	var table = []struct {
		input int
	}{
		{input: 0},
		{input: 10},
		{input: 100},
	}
	for _, v := range table {
		b.Run(fmt.Sprintf("inner_txn_count_%d", v.input), func(b *testing.B) {
			bd, tag := blockData(addr, v.input)
			cfgStr := fmt.Sprintf(`filters:
  - all:
    - tag: %s
      search-inner: true
      expression-type: equal
      expression: "%s"`, tag, addr.String())

			fp := &FilterProcessor{}
			err := fp.Init(context.Background(), &conduit.PipelineInitProvider{}, plugins.MakePluginConfig(cfgStr), logrus.New())
			assert.NoError(b, err)

			// sanity test Process
			{
				out, err := fp.Process(bd)
				require.NoError(b, err)
				require.Len(b, out.Payset, 1)
			}

			// Ignore the setup cost above.
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				fp.Process(bd)
			}
		})
	}
}
