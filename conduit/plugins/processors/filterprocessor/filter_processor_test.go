package filterprocessor

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/transactions"

	"github.com/algorand/indexer/conduit"
	"github.com/algorand/indexer/conduit/plugins"
	"github.com/algorand/indexer/conduit/plugins/processors"
	"github.com/algorand/indexer/data"
)

// TestFilterProcessor_Init_None
func TestFilterProcessor_Init_None(t *testing.T) {

	sampleAddr1 := basics.Address{1}
	sampleAddr2 := basics.Address{2}
	sampleAddr3 := basics.Address{3}

	sampleCfgStr := `---
filters:
  - none: 
    - tag: sgnr
      expression-type: exact
      expression: "` + sampleAddr1.String() + `"
    - tag: txn.asnd
      expression-type: regex
      expression: "` + sampleAddr3.String() + `"
  - all:
    - tag: txn.rcv
      expression-type: regex 
      expression: "` + sampleAddr2.String() + `"
    - tag: txn.snd
      expression-type: exact
      expression: "` + sampleAddr2.String() + `"
  - any: 
    - tag: txn.aclose
      expression-type: exact
      expression: "` + sampleAddr2.String() + `"
    - tag: txn.arcv
      expression-type: regex
      expression: "` + sampleAddr2.String() + `"
`

	fpBuilder, err := processors.ProcessorBuilderByName(implementationName)
	assert.NoError(t, err)

	fp := fpBuilder.New()
	err = fp.Init(context.Background(), &conduit.PipelineInitProvider{}, plugins.MakePluginConfig(sampleCfgStr), logrus.New())
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
					AuthAddr: sampleAddr1,
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
					AuthAddr: sampleAddr1,
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
		transactions.SignedTxnInBlock{
			SignedTxnWithAD: transactions.SignedTxnWithAD{
				SignedTxn: transactions.SignedTxn{
					AuthAddr: sampleAddr1,
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
		// The one transaction that will be allowed through
		transactions.SignedTxnInBlock{
			SignedTxnWithAD: transactions.SignedTxnWithAD{
				SignedTxn: transactions.SignedTxn{
					AuthAddr: sampleAddr2,
					Txn: transactions.Transaction{
						PaymentTxnFields: transactions.PaymentTxnFields{
							Receiver: sampleAddr2,
						},
						Header: transactions.Header{
							Sender: sampleAddr2,
						},
						AssetTransferTxnFields: transactions.AssetTransferTxnFields{
							AssetSender:   sampleAddr1,
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
	assert.Equal(t, output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.AssetTransferTxnFields.AssetSender, sampleAddr1)
	assert.Equal(t, output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.AssetTransferTxnFields.AssetCloseTo, sampleAddr2)
	assert.Equal(t, output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.AssetTransferTxnFields.AssetReceiver, sampleAddr2)
}

// TestFilterProcessor_Illegal tests that numerical operations won't occur on non-supported types
func TestFilterProcessor_Illegal(t *testing.T) {
	tests := []struct {
		name          string
		cfg           string
		errorContains string
	}{
		{
			"illegal 1", `---
filters:
  - any:
    - tag: txn.type
      expression-type: less-than 
      expression: 4
`, "unknown target kind"},

		{
			"illegal 2", `---
filters:
  - any:
    - tag: txn.type
      expression-type: less-than-equal
      expression: 4
`, "unknown target kind"},

		{
			"illegal 3", `---
filters:
  - any:
    - tag: txn.type
      expression-type: greater-than 
      expression: 4
`, "unknown target kind"},

		{
			"illegal 4", `---
filters:
  - any:
    - tag: txn.type
      expression-type: greater-than-equal
      expression: 4
`, "unknown target kind"},

		{
			"illegal 4", `---
filters:
  - any:
    - tag: txn.type
      expression-type: equal
      expression: 4
`, "unknown target kind"},

		{
			"illegal 5", `---
filters:
  - any:
    - tag: txn.type
      expression-type: not-equal
      expression: 4
`, "unknown target kind"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			fpBuilder, err := processors.ProcessorBuilderByName(implementationName)
			assert.NoError(t, err)

			fp := fpBuilder.New()
			err = fp.Init(context.Background(), &conduit.PipelineInitProvider{}, plugins.MakePluginConfig(test.cfg), logrus.New())
			assert.ErrorContains(t, err, test.errorContains)
		})
	}
}

// TestFilterProcessor_Alias tests the various numerical operations on integers that are aliased
func TestFilterProcessor_Alias(t *testing.T) {
	tests := []struct {
		name string
		cfg  string
		fxn  func(t *testing.T, output *data.BlockData)
	}{

		{"alias 1", `---
filters:
  - any:
    - tag: apid 
      expression-type: less-than 
      expression: 4
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, len(output.Payset), 1)
			assert.Equal(t, output.Payset[0].SignedTxnWithAD.ApplicationID, basics.AppIndex(2))
		},
		},
		{"alias 2", `---
filters:
  - any:
    - tag: apid
      expression-type: less-than 
      expression: 5
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, len(output.Payset), 2)
			assert.Equal(t, output.Payset[0].SignedTxnWithAD.ApplicationID, basics.AppIndex(4))
			assert.Equal(t, output.Payset[1].SignedTxnWithAD.ApplicationID, basics.AppIndex(2))
		},
		},

		{"alias 3", `---
filters:
  - any:
    - tag: apid
      expression-type: less-than-equal
      expression: 4
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, len(output.Payset), 2)
			assert.Equal(t, output.Payset[0].SignedTxnWithAD.ApplicationID, basics.AppIndex(4))
			assert.Equal(t, output.Payset[1].SignedTxnWithAD.ApplicationID, basics.AppIndex(2))
		},
		},
		{"alias 4", `---
filters:
  - any:
    - tag: apid
      expression-type: equal
      expression: 11
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, len(output.Payset), 1)
			assert.Equal(t, output.Payset[0].SignedTxnWithAD.ApplicationID, basics.AppIndex(11))
		},
		},

		{"alias 5", `---
filters:
  - any:
    - tag: apid
      expression-type: not-equal
      expression: 11
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, len(output.Payset), 2)
			assert.Equal(t, output.Payset[0].SignedTxnWithAD.ApplicationID, basics.AppIndex(4))
			assert.Equal(t, output.Payset[1].SignedTxnWithAD.ApplicationID, basics.AppIndex(2))
		},
		},

		{"alias 6", `---
filters:
  - any:
    - tag: apid
      expression-type: greater-than 
      expression: 4
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, len(output.Payset), 1)
			assert.Equal(t, output.Payset[0].SignedTxnWithAD.ApplicationID, basics.AppIndex(11))
		},
		},
		{"alias 7", `---
filters:
  - any:
    - tag: apid
      expression-type: greater-than 
      expression: 3
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, len(output.Payset), 2)
			assert.Equal(t, output.Payset[0].SignedTxnWithAD.ApplicationID, basics.AppIndex(4))
			assert.Equal(t, output.Payset[1].SignedTxnWithAD.ApplicationID, basics.AppIndex(11))
		},
		},

		{"alias 8", `---
filters:
  - any:
    - tag: apid
      expression-type: greater-than-equal
      expression: 4
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, len(output.Payset), 2)
			assert.Equal(t, output.Payset[0].SignedTxnWithAD.ApplicationID, basics.AppIndex(4))
			assert.Equal(t, output.Payset[1].SignedTxnWithAD.ApplicationID, basics.AppIndex(11))
		},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			fpBuilder, err := processors.ProcessorBuilderByName(implementationName)
			assert.NoError(t, err)

			fp := fpBuilder.New()
			err = fp.Init(context.Background(), &conduit.PipelineInitProvider{}, plugins.MakePluginConfig(test.cfg), logrus.New())
			assert.NoError(t, err)

			bd := data.BlockData{}
			bd.Payset = append(bd.Payset,

				transactions.SignedTxnInBlock{
					SignedTxnWithAD: transactions.SignedTxnWithAD{
						ApplyData: transactions.ApplyData{
							ApplicationID: 4,
						},
					},
				},
				transactions.SignedTxnInBlock{

					SignedTxnWithAD: transactions.SignedTxnWithAD{
						ApplyData: transactions.ApplyData{
							ApplicationID: 2,
						},
					},
				},
				transactions.SignedTxnInBlock{

					SignedTxnWithAD: transactions.SignedTxnWithAD{
						ApplyData: transactions.ApplyData{
							ApplicationID: 11,
						},
					},
				},
			)

			output, err := fp.Process(bd)
			assert.NoError(t, err)
			test.fxn(t, &output)
		})
	}
}

// TestFilterProcessor_Numerical tests the various numerical operations on integers
func TestFilterProcessor_Numerical(t *testing.T) {
	tests := []struct {
		name string
		cfg  string
		fxn  func(t *testing.T, output *data.BlockData)
	}{

		{"numerical 1", `---
filters:
  - any:
    - tag: aca
      expression-type: less-than 
      expression: 4
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, len(output.Payset), 1)
			assert.Equal(t, output.Payset[0].SignedTxnWithAD.AssetClosingAmount, uint64(2))
		},
		},
		{"numerical 2", `---
filters:
  - any:
    - tag: aca
      expression-type: less-than 
      expression: 5
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, len(output.Payset), 2)
			assert.Equal(t, output.Payset[0].SignedTxnWithAD.AssetClosingAmount, uint64(4))
			assert.Equal(t, output.Payset[1].SignedTxnWithAD.AssetClosingAmount, uint64(2))
		},
		},

		{"numerical 3", `---
filters:
  - any:
    - tag: aca
      expression-type: less-than-equal
      expression: 4
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, len(output.Payset), 2)
			assert.Equal(t, output.Payset[0].SignedTxnWithAD.AssetClosingAmount, uint64(4))
			assert.Equal(t, output.Payset[1].SignedTxnWithAD.AssetClosingAmount, uint64(2))
		},
		},
		{"numerical 4", `---
filters:
  - any:
    - tag: aca
      expression-type: equal
      expression: 11
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, len(output.Payset), 1)
			assert.Equal(t, output.Payset[0].SignedTxnWithAD.AssetClosingAmount, uint64(11))
		},
		},

		{"numerical 5", `---
filters:
  - any:
    - tag: aca
      expression-type: not-equal
      expression: 11
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, len(output.Payset), 2)
			assert.Equal(t, output.Payset[0].SignedTxnWithAD.AssetClosingAmount, uint64(4))
			assert.Equal(t, output.Payset[1].SignedTxnWithAD.AssetClosingAmount, uint64(2))
		},
		},

		{"numerical 6", `---
filters:
  - any:
    - tag: aca
      expression-type: greater-than 
      expression: 4
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, len(output.Payset), 1)
			assert.Equal(t, output.Payset[0].SignedTxnWithAD.AssetClosingAmount, uint64(11))
		},
		},
		{"numerical 7", `---
filters:
  - any:
    - tag: aca
      expression-type: greater-than 
      expression: 3
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, len(output.Payset), 2)
			assert.Equal(t, output.Payset[0].SignedTxnWithAD.AssetClosingAmount, uint64(4))
			assert.Equal(t, output.Payset[1].SignedTxnWithAD.AssetClosingAmount, uint64(11))
		},
		},

		{"numerical 8", `---
filters:
  - any:
    - tag: aca
      expression-type: greater-than-equal
      expression: 4
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, len(output.Payset), 2)
			assert.Equal(t, output.Payset[0].SignedTxnWithAD.AssetClosingAmount, uint64(4))
			assert.Equal(t, output.Payset[1].SignedTxnWithAD.AssetClosingAmount, uint64(11))
		},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			fpBuilder, err := processors.ProcessorBuilderByName(implementationName)
			assert.NoError(t, err)

			fp := fpBuilder.New()
			err = fp.Init(context.Background(), &conduit.PipelineInitProvider{}, plugins.MakePluginConfig(test.cfg), logrus.New())
			assert.NoError(t, err)

			bd := data.BlockData{}
			bd.Payset = append(bd.Payset,

				transactions.SignedTxnInBlock{
					SignedTxnWithAD: transactions.SignedTxnWithAD{
						ApplyData: transactions.ApplyData{
							AssetClosingAmount: 4,
						},
					},
				},
				transactions.SignedTxnInBlock{

					SignedTxnWithAD: transactions.SignedTxnWithAD{
						ApplyData: transactions.ApplyData{
							AssetClosingAmount: 2,
						},
					},
				},
				transactions.SignedTxnInBlock{

					SignedTxnWithAD: transactions.SignedTxnWithAD{
						ApplyData: transactions.ApplyData{
							AssetClosingAmount: 11,
						},
					},
				},
			)

			output, err := fp.Process(bd)
			assert.NoError(t, err)
			test.fxn(t, &output)
		})
	}
}

// TestFilterProcessor_MicroAlgos tests the various numerical operations on microalgos
func TestFilterProcessor_MicroAlgos(t *testing.T) {
	tests := []struct {
		name string
		cfg  string
		fxn  func(t *testing.T, output *data.BlockData)
	}{
		{"micro algo 1", `---
filters:
  - any:
    - tag: txn.amt
      expression-type: less-than 
      expression: 4
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, len(output.Payset), 1)
			assert.Equal(t, output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Amount.Raw, uint64(2))
		},
		},
		{"micro algo 2", `---
filters:
  - any:
    - tag: txn.amt
      expression-type: less-than 
      expression: 5
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, len(output.Payset), 2)
			assert.Equal(t, output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Amount.Raw, uint64(4))
			assert.Equal(t, output.Payset[1].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Amount.Raw, uint64(2))
		},
		},

		{"micro algo 3", `---
filters:
  - any:
    - tag: txn.amt
      expression-type: less-than-equal
      expression: 4
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, len(output.Payset), 2)
			assert.Equal(t, output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Amount.Raw, uint64(4))
			assert.Equal(t, output.Payset[1].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Amount.Raw, uint64(2))
		},
		},
		{"micro algo 4", `---
filters:
  - any:
    - tag: txn.amt
      expression-type: equal
      expression: 11
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, len(output.Payset), 1)
			assert.Equal(t, output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Amount.Raw, uint64(11))
		},
		},

		{"micro algo 5", `---
filters:
  - any:
    - tag: txn.amt
      expression-type: not-equal
      expression: 11
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, len(output.Payset), 2)
			assert.Equal(t, output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Amount.Raw, uint64(4))
			assert.Equal(t, output.Payset[1].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Amount.Raw, uint64(2))
		},
		},

		{"micro algo 6", `---
filters:
  - any:
    - tag: txn.amt
      expression-type: greater-than 
      expression: 4
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, len(output.Payset), 1)
			assert.Equal(t, output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Amount.Raw, uint64(11))
		},
		},
		{"micro algo 7", `---
filters:
  - any:
    - tag: txn.amt
      expression-type: greater-than 
      expression: 3
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, len(output.Payset), 2)
			assert.Equal(t, output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Amount.Raw, uint64(4))
			assert.Equal(t, output.Payset[1].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Amount.Raw, uint64(11))
		},
		},

		{"micro algo 8", `---
filters:
  - any:
    - tag: txn.amt
      expression-type: greater-than-equal
      expression: 4
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, len(output.Payset), 2)
			assert.Equal(t, output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Amount.Raw, uint64(4))
			assert.Equal(t, output.Payset[1].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Amount.Raw, uint64(11))
		},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			fpBuilder, err := processors.ProcessorBuilderByName(implementationName)
			assert.NoError(t, err)

			fp := fpBuilder.New()
			err = fp.Init(context.Background(), &conduit.PipelineInitProvider{}, plugins.MakePluginConfig(test.cfg), logrus.New())
			assert.NoError(t, err)

			bd := data.BlockData{}
			bd.Payset = append(bd.Payset,

				transactions.SignedTxnInBlock{
					SignedTxnWithAD: transactions.SignedTxnWithAD{
						SignedTxn: transactions.SignedTxn{
							Txn: transactions.Transaction{
								PaymentTxnFields: transactions.PaymentTxnFields{
									Amount: basics.MicroAlgos{Raw: 4},
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
									Amount: basics.MicroAlgos{Raw: 2},
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
									Amount: basics.MicroAlgos{Raw: 11},
								},
							},
						},
					},
				},
			)

			output, err := fp.Process(bd)
			assert.NoError(t, err)
			test.fxn(t, &output)
		})
	}
}

// TestFilterProcessor_VariousErrorPathsOnInit tests the various error paths in the filter processor init function
func TestFilterProcessor_VariousErrorPathsOnInit(t *testing.T) {
	tests := []struct {
		name             string
		sampleCfgStr     string
		errorContainsStr string
	}{

		{"MakeExpressionError", `---
filters:
 - any:
   - tag: DoesNot.ExistIn.Struct
     expression-type: exact
     expression: "sample"
`, "unknown tag"},

		{"MakeExpressionError", `---
filters:
 - any:
   - tag: sgnr
     expression-type: wrong-expression-type
     expression: "sample"
`, "could not make expression with string"},

		{"CorrectFilterType", `---
filters:
  - wrong-filter-type: 
    - tag: sgnr
      expression-type: exact
      expression: "sample"

`, "filter key was not a valid value"},

		{"FilterTagFormation", `---
filters:
  - any: 
    - tag: sgnr
      expression-type: exact
      expression: "sample"
    all:
    - tag: sgnr
      expression-type: exact
      expression: "sample"


`, "illegal filter tag formation"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			fpBuilder, err := processors.ProcessorBuilderByName(implementationName)
			assert.NoError(t, err)

			fp := fpBuilder.New()
			err = fp.Init(context.Background(), &conduit.PipelineInitProvider{}, plugins.MakePluginConfig(test.sampleCfgStr), logrus.New())
			assert.ErrorContains(t, err, test.errorContainsStr)
		})
	}
}

// TestFilterProcessor_Init_Multi tests initialization of the filter processor with the "all" and "any" filter types
func TestFilterProcessor_Init_Multi(t *testing.T) {

	sampleAddr1 := basics.Address{1}
	sampleAddr2 := basics.Address{2}
	sampleAddr3 := basics.Address{3}

	sampleCfgStr := `---
filters:
  - any: 
    - tag: sgnr
      expression-type: exact
      expression: "` + sampleAddr1.String() + `"
    - tag: txn.asnd
      expression-type: regex
      expression: "` + sampleAddr3.String() + `"
  - all:
    - tag: txn.rcv
      expression-type: regex 
      expression: "` + sampleAddr2.String() + `"
    - tag: txn.snd
      expression-type: exact
      expression: "` + sampleAddr2.String() + `"
  - any: 
    - tag: txn.aclose
      expression-type: exact
      expression: "` + sampleAddr2.String() + `"
    - tag: txn.arcv
      expression-type: regex
      expression: "` + sampleAddr2.String() + `"
`

	fpBuilder, err := processors.ProcessorBuilderByName(implementationName)
	assert.NoError(t, err)

	fp := fpBuilder.New()
	err = fp.Init(context.Background(), &conduit.PipelineInitProvider{}, plugins.MakePluginConfig(sampleCfgStr), logrus.New())
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
    - tag: txn.rcv
      expression-type: regex 
      expression: "` + sampleAddr2.String() + `"
    - tag: txn.snd
      expression-type: exact
      expression: "` + sampleAddr2.String() + `"
`

	fpBuilder, err := processors.ProcessorBuilderByName(implementationName)
	assert.NoError(t, err)

	fp := fpBuilder.New()
	err = fp.Init(context.Background(), &conduit.PipelineInitProvider{}, plugins.MakePluginConfig(sampleCfgStr), logrus.New())
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
    - tag: txn.rcv
      expression-type: regex 
      expression: "` + sampleAddr1.String() + `"
    - tag: txn.rcv
      expression-type: exact
      expression: "` + sampleAddr2.String() + `"
`

	fpBuilder, err := processors.ProcessorBuilderByName(implementationName)
	assert.NoError(t, err)

	fp := fpBuilder.New()
	err = fp.Init(context.Background(), &conduit.PipelineInitProvider{}, plugins.MakePluginConfig(sampleCfgStr), logrus.New())
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
