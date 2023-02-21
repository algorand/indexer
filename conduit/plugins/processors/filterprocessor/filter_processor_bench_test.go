package filterprocessor

import (
	"context"
	"testing"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/algorand/indexer/conduit"
	"github.com/algorand/indexer/conduit/plugins"
	"github.com/algorand/indexer/data"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func BenchmarkProcess(b *testing.B) {
	var addr sdk.Address
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

	// Ignore the setup cost above.
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fp.Process(bd)
	}
}
