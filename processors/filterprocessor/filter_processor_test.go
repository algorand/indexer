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

// TestFilterProcessor_Init_Multi tests initialization of the filter processor with the "all" and "any" filter types
func TestFilterProcessor_Init_Multi(t *testing.T) {

	sampleAddr1 := basics.Address{1}
	sampleAddr2 := basics.Address{2}
	sampleAddr3 := basics.Address{3}

	sampleCfgStr := `---
filters:
  - any: 
    - tag: SignedTxnWithAD.SignedTxn.AuthAddr
      expression-type: exact
      expression: "` + sampleAddr1.String() + `"
    - tag: SignedTxnWithAD.SignedTxn.Txn.AssetTransferTxnFields.AssetSender
      expression-type: regex
      expression: "` + sampleAddr3.String() + `"
  - all:
    - tag: SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Receiver
      expression-type: regex 
      expression: "` + sampleAddr2.String() + `"
    - tag: SignedTxnWithAD.SignedTxn.Txn.Header.Sender
      expression-type: exact
      expression: "` + sampleAddr2.String() + `"
  - any: 
    - tag: SignedTxnWithAD.SignedTxn.Txn.AssetTransferTxnFields.AssetCloseTo
      expression-type: exact
      expression: "` + sampleAddr2.String() + `"
    - tag: SignedTxnWithAD.SignedTxn.Txn.AssetTransferTxnFields.AssetReceiver
      expression-type: regex
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
					AuthAddr: sampleAddr1,
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
						Header: transactions.Header{
							Sender: sampleAddr2,
						},
						AssetTransferTxnFields: transactions.AssetTransferTxnFields{
							AssetCloseTo: sampleAddr2,
						},
					},
				},
			},
		},
		transactions.SignedTxnInBlock{
			SignedTxnWithAD: transactions.SignedTxnWithAD{
				SignedTxn: transactions.SignedTxn{
					Txn: transactions.Transaction{
						AssetTransferTxnFields: transactions.AssetTransferTxnFields{
							AssetSender: sampleAddr3,
						},
						PaymentTxnFields: transactions.PaymentTxnFields{
							Receiver: sampleAddr3,
						},
					},
				},
			},
		},
		// The one transaction that will be allowed through
		transactions.SignedTxnInBlock{
			SignedTxnWithAD: transactions.SignedTxnWithAD{
				SignedTxn: transactions.SignedTxn{
					Txn: transactions.Transaction{
						PaymentTxnFields: transactions.PaymentTxnFields{
							Receiver: sampleAddr2,
						},
						Header: transactions.Header{
							Sender: sampleAddr2,
						},
						AssetTransferTxnFields: transactions.AssetTransferTxnFields{
							AssetSender:   sampleAddr3,
							AssetCloseTo:  sampleAddr2,
							AssetReceiver: sampleAddr2,
						},
					},
				},
			},
		},
	)

	output, err := fp.Process(bd)
	assert.NoError(t, err)
	assert.Equal(t, len(output.Payset), 1)
	assert.Equal(t, output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Receiver, sampleAddr2)
	assert.Equal(t, output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.Header.Sender, sampleAddr2)
	assert.Equal(t, output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.AssetTransferTxnFields.AssetSender, sampleAddr3)
	assert.Equal(t, output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.AssetTransferTxnFields.AssetCloseTo, sampleAddr2)
	assert.Equal(t, output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.AssetTransferTxnFields.AssetReceiver, sampleAddr2)

}

// TestFilterProcessor_Init_All tests initialization of the filter processor with the "all" filter type
func TestFilterProcessor_Init_All(t *testing.T) {

	sampleAddr1 := basics.Address{1}
	sampleAddr2 := basics.Address{2}
	sampleAddr3 := basics.Address{3}

	sampleCfgStr := `---
filters:
  - all:
    - tag: SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Receiver
      expression-type: regex 
      expression: "` + sampleAddr2.String() + `"
    - tag: SignedTxnWithAD.SignedTxn.Txn.Header.Sender
      expression-type: exact
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
						Header: transactions.Header{
							Sender: sampleAddr2,
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
	assert.Equal(t, len(output.Payset), 1)
	assert.Equal(t, output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Receiver, sampleAddr2)
	assert.Equal(t, output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.Header.Sender, sampleAddr2)
}

// TestFilterProcessor_Init_Some tests initialization of the filter processor with the "any" filter type
func TestFilterProcessor_Init(t *testing.T) {

	sampleAddr1 := basics.Address{1}
	sampleAddr2 := basics.Address{2}
	sampleAddr3 := basics.Address{3}

	sampleCfgStr := `---
filters:
  - any:
    - tag: SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Receiver
      expression-type: regex 
      expression: "` + sampleAddr1.String() + `"
    - tag: SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Receiver
      expression-type: exact
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
	assert.Equal(t, output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Receiver, sampleAddr1)
	assert.Equal(t, output.Payset[1].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Receiver, sampleAddr2)
}
