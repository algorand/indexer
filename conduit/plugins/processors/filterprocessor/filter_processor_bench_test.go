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
						Txn: sdk.Transaction{
							Header: sdk.Header{
								Group: sdk.Digest{1},
							},
						},
					},
				},
			},
			{
				SignedTxnWithAD: sdk.SignedTxnWithAD{
					SignedTxn: sdk.SignedTxn{
						AuthAddr: addr,
						Txn: sdk.Transaction{
							Header: sdk.Header{
								Group: sdk.Digest{1},
							},
						},
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
		input         int
		omitGroupTxns bool
	}{
		{input: 0, omitGroupTxns: true},
		{input: 10, omitGroupTxns: true},
		{input: 100, omitGroupTxns: true},
		{input: 0, omitGroupTxns: false},
		{input: 10, omitGroupTxns: false},
		{input: 100, omitGroupTxns: false},
	}
	for _, v := range table {
		b.Run(fmt.Sprintf("inner_txn_count_%d_omitGrouptxns_%t", v.input, v.omitGroupTxns), func(b *testing.B) {
			bd, tag := blockData(addr, v.input)
			cfgStr := fmt.Sprintf(`search-inner: true
omit-group-transactions: %t
filters:
  - all:
    - tag: %s
      expression-type: equal
      expression: "%s"`, v.omitGroupTxns, tag, addr.String())

			fp := &FilterProcessor{}
			err := fp.Init(context.Background(), &conduit.PipelineInitProvider{}, plugins.MakePluginConfig(cfgStr), logrus.New())
			assert.NoError(b, err)

			// sanity test Process
			{
				out, err := fp.Process(bd)
				require.NoError(b, err)
				require.Len(b, out.Payset, 2)
			}

			// Ignore the setup cost above.
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				fp.Process(bd)
			}
		})
	}
}
