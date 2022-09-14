package filterprocessor

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/indexer/conduit"
	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/plugins"
	"github.com/algorand/indexer/processors"
)

// TestFilterProcessor_Init tests initialization of the filter processor
func TestFilterProcessor_Init(t *testing.T) {

	sampleAddr1 := basics.Address{1}
	sampleAddr2 := basics.Address{2}
	sampleAddr3 := basics.Address{3}

	sampleCfgStr := `---
filters:
  - some:
    - tag: SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Receiver
      expression-type: regex 
      expression: "` + sampleAddr1.String() + `"
    - tag: SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Receiver
      expression-type: const
      expression: "` + sampleAddr2.String() + `"
`

	fpBuilder, err := processors.ProcessorBuilderByName(implementationName)
	assert.NoError(t, err)

	fp := fpBuilder.New()
	err = fp.Init(context.Background(), &conduit.PipelineInitProvider{}, plugins.PluginConfig(sampleCfgStr), logrus.New())
	assert.NoError(t, err)

	bd := data.BlockData{}
	bd.Payset = append(bd.Payset,

		transactions.SignedTxnInBlock{
			SignedTxnWithAD: transactions.SignedTxnWithAD{
				SignedTxn: transactions.SignedTxn{
					Txn: transactions.Transaction{
						PaymentTxnFields: transactions.PaymentTxnFields{
							Receiver: sampleAddr1,
						},
					},
				},
			},
		},
		transactions.SignedTxnInBlock{
			SignedTxnWithAD: transactions.SignedTxnWithAD{
				SignedTxn: transactions.SignedTxn{
					Txn: transactions.Transaction{
						PaymentTxnFields: transactions.PaymentTxnFields{
							Receiver: sampleAddr2,
						},
					},
				},
			},
		},
		transactions.SignedTxnInBlock{
			SignedTxnWithAD: transactions.SignedTxnWithAD{
				SignedTxn: transactions.SignedTxn{
					Txn: transactions.Transaction{
						PaymentTxnFields: transactions.PaymentTxnFields{
							Receiver: sampleAddr3,
						},
					},
				},
			},
		},
	)

	output, err := fp.Process(bd)
	assert.NoError(t, err)
	assert.Equal(t, len(output.Payset), 2)

}
