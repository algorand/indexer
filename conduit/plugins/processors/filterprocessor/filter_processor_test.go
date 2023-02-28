package filterprocessor

import (
	"context"
	"testing"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/algorand/indexer/conduit"
	"github.com/algorand/indexer/conduit/plugins"
	"github.com/algorand/indexer/conduit/plugins/processors"
	"github.com/algorand/indexer/data"
)

// create an empty block data so that txn fields can be set with less vertical space
func testBlock(numTxn int) data.BlockData {
	return data.BlockData{
		Payset: make([]sdk.SignedTxnInBlock, numTxn),
	}
}

// TestFilterProcessor_Init_None
func TestFilterProcessor_Init_None(t *testing.T) {

	sampleAddr1 := sdk.Address{1}
	sampleAddr2 := sdk.Address{2}
	sampleAddr3 := sdk.Address{3}

	sampleCfgStr := `---
filters:
  - none: 
    - tag: sgnr
      expression-type: equal
      expression: "` + sampleAddr1.String() + `"
    - tag: txn.asnd
      expression-type: regex
      expression: "` + sampleAddr3.String() + `"
  - all:
    - tag: txn.rcv
      expression-type: regex 
      expression: "` + sampleAddr2.String() + `"
    - tag: txn.snd
      expression-type: equal
      expression: "` + sampleAddr2.String() + `"
  - any: 
    - tag: txn.aclose
      expression-type: equal
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

		sdk.SignedTxnInBlock{
			SignedTxnWithAD: sdk.SignedTxnWithAD{
				SignedTxn: sdk.SignedTxn{
					AuthAddr: sampleAddr1,
				},
			},
		},
		sdk.SignedTxnInBlock{
			SignedTxnWithAD: sdk.SignedTxnWithAD{
				SignedTxn: sdk.SignedTxn{
					AuthAddr: sampleAddr1,
					Txn: sdk.Transaction{
						PaymentTxnFields: sdk.PaymentTxnFields{
							Receiver: sampleAddr2,
						},
						Header: sdk.Header{
							Sender: sampleAddr2,
						},
						AssetTransferTxnFields: sdk.AssetTransferTxnFields{
							AssetCloseTo: sampleAddr2,
						},
					},
				},
			},
		},
		sdk.SignedTxnInBlock{
			SignedTxnWithAD: sdk.SignedTxnWithAD{
				SignedTxn: sdk.SignedTxn{
					AuthAddr: sampleAddr1,
					Txn: sdk.Transaction{
						AssetTransferTxnFields: sdk.AssetTransferTxnFields{
							AssetSender: sampleAddr3,
						},
						PaymentTxnFields: sdk.PaymentTxnFields{
							Receiver: sampleAddr3,
						},
					},
				},
			},
		},
		sdk.SignedTxnInBlock{
			SignedTxnWithAD: sdk.SignedTxnWithAD{
				SignedTxn: sdk.SignedTxn{
					AuthAddr: sampleAddr1,
					Txn: sdk.Transaction{
						PaymentTxnFields: sdk.PaymentTxnFields{
							Receiver: sampleAddr2,
						},
						Header: sdk.Header{
							Sender: sampleAddr2,
						},
						AssetTransferTxnFields: sdk.AssetTransferTxnFields{
							AssetSender:   sampleAddr3,
							AssetCloseTo:  sampleAddr2,
							AssetReceiver: sampleAddr2,
						},
					},
				},
			},
		},
		// The one transaction that will be allowed through
		sdk.SignedTxnInBlock{
			SignedTxnWithAD: sdk.SignedTxnWithAD{
				SignedTxn: sdk.SignedTxn{
					AuthAddr: sampleAddr2,
					Txn: sdk.Transaction{
						PaymentTxnFields: sdk.PaymentTxnFields{
							Receiver: sampleAddr2,
						},
						Header: sdk.Header{
							Sender: sampleAddr2,
						},
						AssetTransferTxnFields: sdk.AssetTransferTxnFields{
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
	assert.Equal(t, 1, len(output.Payset))
	assert.Equal(t, sampleAddr2, output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Receiver)
	assert.Equal(t, sampleAddr2, output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.Header.Sender)
	assert.Equal(t, sampleAddr1, output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.AssetTransferTxnFields.AssetSender)
	assert.Equal(t, sampleAddr2, output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.AssetTransferTxnFields.AssetCloseTo)
	assert.Equal(t, sampleAddr2, output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.AssetTransferTxnFields.AssetReceiver)
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
`, "target type (string) does not support less-than filters"},

		{
			"illegal 2", `---
filters:
  - any:
    - tag: txn.type
      expression-type: less-than-equal
      expression: 4
`, "target type (string) does not support less-than-equal filters"},

		{
			"illegal 3", `---
filters:
  - any:
    - tag: txn.type
      expression-type: greater-than 
      expression: 4
`, "target type (string) does not support greater-than filters"},

		{
			"illegal 4", `---
filters:
  - any:
    - tag: txn.type
      expression-type: greater-than-equal
      expression: 4
`, "target type (string) does not support greater-than-equal filters"},

		{
			"illegal 5", `---
filters:
  - any:
    - tag: txn.type
      expression-type: not-equal
      expression: 4
`, "target type (string) does not support not-equal filters"},
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

			assert.Equal(t, 1, len(output.Payset))
			assert.Equal(t, uint64(2), output.Payset[0].SignedTxnWithAD.ApplicationID)
		},
		},
		{"alias 2", `---
filters:
  - any:
    - tag: apid
      expression-type: less-than 
      expression: 5
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, 2, len(output.Payset))
			assert.Equal(t, uint64(4), output.Payset[0].SignedTxnWithAD.ApplicationID)
			assert.Equal(t, uint64(2), output.Payset[1].SignedTxnWithAD.ApplicationID)
		},
		},

		{"alias 3", `---
filters:
  - any:
    - tag: apid
      expression-type: less-than-equal
      expression: 4
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, 2, len(output.Payset))
			assert.Equal(t, uint64(4), output.Payset[0].SignedTxnWithAD.ApplicationID)
			assert.Equal(t, uint64(2), output.Payset[1].SignedTxnWithAD.ApplicationID)
		},
		},
		{"alias 4", `---
filters:
  - any:
    - tag: apid
      expression-type: equal
      expression: 11
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, 1, len(output.Payset))
			assert.Equal(t, uint64(11), output.Payset[0].SignedTxnWithAD.ApplicationID)
		},
		},

		{"alias 5", `---
filters:
  - any:
    - tag: apid
      expression-type: not-equal
      expression: 11
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, 2, len(output.Payset))
			assert.Equal(t, uint64(4), output.Payset[0].SignedTxnWithAD.ApplicationID)
			assert.Equal(t, uint64(2), output.Payset[1].SignedTxnWithAD.ApplicationID)
		},
		},

		{"alias 6", `---
filters:
  - any:
    - tag: apid
      expression-type: greater-than 
      expression: 4
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, 1, len(output.Payset))
			assert.Equal(t, uint64(11), output.Payset[0].SignedTxnWithAD.ApplicationID)
		},
		},
		{"alias 7", `---
filters:
  - any:
    - tag: apid
      expression-type: greater-than 
      expression: 3
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, 2, len(output.Payset))
			assert.Equal(t, uint64(4), output.Payset[0].SignedTxnWithAD.ApplicationID)
			assert.Equal(t, uint64(11), output.Payset[1].SignedTxnWithAD.ApplicationID)
		},
		},

		{"alias 8", `---
filters:
  - any:
    - tag: apid
      expression-type: greater-than-equal
      expression: 4
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, 2, len(output.Payset))
			assert.Equal(t, uint64(4), output.Payset[0].SignedTxnWithAD.ApplicationID)
			assert.Equal(t, uint64(11), output.Payset[1].SignedTxnWithAD.ApplicationID)
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

				sdk.SignedTxnInBlock{
					SignedTxnWithAD: sdk.SignedTxnWithAD{
						ApplyData: sdk.ApplyData{
							ApplicationID: 4,
						},
					},
				},
				sdk.SignedTxnInBlock{

					SignedTxnWithAD: sdk.SignedTxnWithAD{
						ApplyData: sdk.ApplyData{
							ApplicationID: 2,
						},
					},
				},
				sdk.SignedTxnInBlock{

					SignedTxnWithAD: sdk.SignedTxnWithAD{
						ApplyData: sdk.ApplyData{
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

			assert.Equal(t, 1, len(output.Payset))
			assert.Equal(t, uint64(2), output.Payset[0].SignedTxnWithAD.AssetClosingAmount)
		},
		},
		{"numerical 2", `---
filters:
  - any:
    - tag: aca
      expression-type: less-than 
      expression: 5
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, 2, len(output.Payset))
			assert.Equal(t, uint64(4), output.Payset[0].SignedTxnWithAD.AssetClosingAmount)
			assert.Equal(t, uint64(2), output.Payset[1].SignedTxnWithAD.AssetClosingAmount)
		},
		},

		{"numerical 3", `---
filters:
  - any:
    - tag: aca
      expression-type: less-than-equal
      expression: 4
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, 2, len(output.Payset))
			assert.Equal(t, uint64(4), output.Payset[0].SignedTxnWithAD.AssetClosingAmount)
			assert.Equal(t, uint64(2), output.Payset[1].SignedTxnWithAD.AssetClosingAmount)
		},
		},
		{"numerical 4", `---
filters:
  - any:
    - tag: aca
      expression-type: equal
      expression: 11
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, 1, len(output.Payset))
			assert.Equal(t, uint64(11), output.Payset[0].SignedTxnWithAD.AssetClosingAmount)
		},
		},

		{"numerical 5", `---
filters:
  - any:
    - tag: aca
      expression-type: not-equal
      expression: 11
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, 2, len(output.Payset))
			assert.Equal(t, uint64(4), output.Payset[0].SignedTxnWithAD.AssetClosingAmount)
			assert.Equal(t, uint64(2), output.Payset[1].SignedTxnWithAD.AssetClosingAmount)
		},
		},

		{"numerical 6", `---
filters:
  - any:
    - tag: aca
      expression-type: greater-than 
      expression: 4
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, 1, len(output.Payset))
			assert.Equal(t, uint64(11), output.Payset[0].SignedTxnWithAD.AssetClosingAmount)
		},
		},
		{"numerical 7", `---
filters:
  - any:
    - tag: aca
      expression-type: greater-than 
      expression: 3
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, 2, len(output.Payset))
			assert.Equal(t, uint64(4), output.Payset[0].SignedTxnWithAD.AssetClosingAmount)
			assert.Equal(t, uint64(11), output.Payset[1].SignedTxnWithAD.AssetClosingAmount)
		},
		},

		{"numerical 8", `---
filters:
  - any:
    - tag: aca
      expression-type: greater-than-equal
      expression: 4
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, 2, len(output.Payset))
			assert.Equal(t, uint64(4), output.Payset[0].SignedTxnWithAD.AssetClosingAmount)
			assert.Equal(t, uint64(11), output.Payset[1].SignedTxnWithAD.AssetClosingAmount)
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

				sdk.SignedTxnInBlock{
					SignedTxnWithAD: sdk.SignedTxnWithAD{
						ApplyData: sdk.ApplyData{
							AssetClosingAmount: 4,
						},
					},
				},
				sdk.SignedTxnInBlock{

					SignedTxnWithAD: sdk.SignedTxnWithAD{
						ApplyData: sdk.ApplyData{
							AssetClosingAmount: 2,
						},
					},
				},
				sdk.SignedTxnInBlock{

					SignedTxnWithAD: sdk.SignedTxnWithAD{
						ApplyData: sdk.ApplyData{
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

			assert.Equal(t, 1, len(output.Payset))
			assert.Equal(t, uint64(2), uint64(output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Amount))
		},
		},
		{"micro algo 2", `---
filters:
  - any:
    - tag: txn.amt
      expression-type: less-than 
      expression: 5
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, 2, len(output.Payset))
			assert.Equal(t, uint64(4), uint64(output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Amount))
			assert.Equal(t, uint64(2), uint64(output.Payset[1].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Amount))
		},
		},

		{"micro algo 3", `---
filters:
  - any:
    - tag: txn.amt
      expression-type: less-than-equal
      expression: 4
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, 2, len(output.Payset))
			assert.Equal(t, uint64(4), uint64(output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Amount))
			assert.Equal(t, uint64(2), uint64(output.Payset[1].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Amount))
		},
		},
		{"micro algo 4", `---
filters:
  - any:
    - tag: txn.amt
      expression-type: equal
      expression: 11
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, 1, len(output.Payset))
			assert.Equal(t, uint64(11), uint64(output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Amount))
		},
		},

		{"micro algo 5", `---
filters:
  - any:
    - tag: txn.amt
      expression-type: not-equal
      expression: 11
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, 2, len(output.Payset))
			assert.Equal(t, uint64(4), uint64(output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Amount))
			assert.Equal(t, uint64(2), uint64(output.Payset[1].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Amount))
		},
		},

		{"micro algo 6", `---
filters:
  - any:
    - tag: txn.amt
      expression-type: greater-than 
      expression: 4
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, 1, len(output.Payset))
			assert.Equal(t, uint64(11), uint64(output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Amount))
		},
		},
		{"micro algo 7", `---
filters:
  - any:
    - tag: txn.amt
      expression-type: greater-than 
      expression: 3
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, 2, len(output.Payset))
			assert.Equal(t, uint64(4), uint64(output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Amount))
			assert.Equal(t, uint64(11), uint64(output.Payset[1].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Amount))
		},
		},

		{"micro algo 8", `---
filters:
  - any:
    - tag: txn.amt
      expression-type: greater-than-equal
      expression: 4
`, func(t *testing.T, output *data.BlockData) {

			assert.Equal(t, 2, len(output.Payset))
			assert.Equal(t, uint64(4), uint64(output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Amount))
			assert.Equal(t, uint64(11), uint64(output.Payset[1].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Amount))
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

				sdk.SignedTxnInBlock{
					SignedTxnWithAD: sdk.SignedTxnWithAD{
						SignedTxn: sdk.SignedTxn{
							Txn: sdk.Transaction{
								PaymentTxnFields: sdk.PaymentTxnFields{
									Amount: 4,
								},
							},
						},
					},
				},
				sdk.SignedTxnInBlock{
					SignedTxnWithAD: sdk.SignedTxnWithAD{
						SignedTxn: sdk.SignedTxn{
							Txn: sdk.Transaction{
								PaymentTxnFields: sdk.PaymentTxnFields{
									Amount: 2,
								},
							},
						},
					},
				},
				sdk.SignedTxnInBlock{
					SignedTxnWithAD: sdk.SignedTxnWithAD{
						SignedTxn: sdk.SignedTxn{
							Txn: sdk.Transaction{
								PaymentTxnFields: sdk.PaymentTxnFields{
									Amount: 11,
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
     expression-type: equal
     expression: "sample"
`, "unknown tag"},

		{"MakeExpressionError", `---
filters:
 - any:
   - tag: sgnr
     expression-type: wrong-expression-type
     expression: "sample"
`, "could not make expression"},

		{"CorrectFilterType", `---
filters:
  - wrong-filter-type: 
    - tag: sgnr
      expression-type: equal
      expression: "sample"

`, "filter key was not a valid value"},

		{"FilterTagFormation", `---
filters:
  - any: 
    - tag: sgnr
      expression-type: equal
      expression: "sample"
    all:
    - tag: sgnr
      expression-type: equal
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

	sampleAddr1 := sdk.Address{1}
	sampleAddr2 := sdk.Address{2}
	sampleAddr3 := sdk.Address{3}

	sampleCfgStr := `---
filters:
  - any: 
    - tag: sgnr
      expression-type: equal
      expression: "` + sampleAddr1.String() + `"
    - tag: txn.asnd
      expression-type: regex
      expression: "` + sampleAddr3.String() + `"
  - all:
    - tag: txn.rcv
      expression-type: regex 
      expression: "` + sampleAddr2.String() + `"
    - tag: txn.snd
      expression-type: equal
      expression: "` + sampleAddr2.String() + `"
  - any: 
    - tag: txn.aclose
      expression-type: equal
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

		sdk.SignedTxnInBlock{
			SignedTxnWithAD: sdk.SignedTxnWithAD{
				SignedTxn: sdk.SignedTxn{
					AuthAddr: sampleAddr1,
				},
			},
		},
		sdk.SignedTxnInBlock{
			SignedTxnWithAD: sdk.SignedTxnWithAD{
				SignedTxn: sdk.SignedTxn{
					Txn: sdk.Transaction{
						PaymentTxnFields: sdk.PaymentTxnFields{
							Receiver: sampleAddr2,
						},
						Header: sdk.Header{
							Sender: sampleAddr2,
						},
						AssetTransferTxnFields: sdk.AssetTransferTxnFields{
							AssetCloseTo: sampleAddr2,
						},
					},
				},
			},
		},
		sdk.SignedTxnInBlock{
			SignedTxnWithAD: sdk.SignedTxnWithAD{
				SignedTxn: sdk.SignedTxn{
					Txn: sdk.Transaction{
						AssetTransferTxnFields: sdk.AssetTransferTxnFields{
							AssetSender: sampleAddr3,
						},
						PaymentTxnFields: sdk.PaymentTxnFields{
							Receiver: sampleAddr3,
						},
					},
				},
			},
		},
		// The one transaction that will be allowed through
		sdk.SignedTxnInBlock{
			SignedTxnWithAD: sdk.SignedTxnWithAD{
				SignedTxn: sdk.SignedTxn{
					Txn: sdk.Transaction{
						PaymentTxnFields: sdk.PaymentTxnFields{
							Receiver: sampleAddr2,
						},
						Header: sdk.Header{
							Sender: sampleAddr2,
						},
						AssetTransferTxnFields: sdk.AssetTransferTxnFields{
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
	assert.Equal(t, 1, len(output.Payset))
	assert.Equal(t, sampleAddr2, output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Receiver)
	assert.Equal(t, sampleAddr2, output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.Header.Sender)
	assert.Equal(t, sampleAddr3, output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.AssetTransferTxnFields.AssetSender)
	assert.Equal(t, sampleAddr2, output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.AssetTransferTxnFields.AssetCloseTo)
	assert.Equal(t, sampleAddr2, output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.AssetTransferTxnFields.AssetReceiver)

}

// TestFilterProcessor_Init_All tests initialization of the filter processor with the "all" filter type
func TestFilterProcessor_Init_All(t *testing.T) {

	sampleAddr1 := sdk.Address{1}
	sampleAddr2 := sdk.Address{2}
	sampleAddr3 := sdk.Address{3}

	sampleCfgStr := `---
filters:
  - all:
    - tag: txn.rcv
      expression-type: regex 
      expression: "` + sampleAddr2.String() + `"
    - tag: txn.snd
      expression-type: equal
      expression: "` + sampleAddr2.String() + `"
`

	fpBuilder, err := processors.ProcessorBuilderByName(implementationName)
	assert.NoError(t, err)

	fp := fpBuilder.New()
	err = fp.Init(context.Background(), &conduit.PipelineInitProvider{}, plugins.MakePluginConfig(sampleCfgStr), logrus.New())
	assert.NoError(t, err)

	bd := data.BlockData{}
	bd.Payset = append(bd.Payset,

		sdk.SignedTxnInBlock{
			SignedTxnWithAD: sdk.SignedTxnWithAD{
				SignedTxn: sdk.SignedTxn{
					Txn: sdk.Transaction{
						PaymentTxnFields: sdk.PaymentTxnFields{
							Receiver: sampleAddr1,
						},
					},
				},
			},
		},
		sdk.SignedTxnInBlock{
			SignedTxnWithAD: sdk.SignedTxnWithAD{
				SignedTxn: sdk.SignedTxn{
					Txn: sdk.Transaction{
						PaymentTxnFields: sdk.PaymentTxnFields{
							Receiver: sampleAddr2,
						},
						Header: sdk.Header{
							Sender: sampleAddr2,
						},
					},
				},
			},
		},
		sdk.SignedTxnInBlock{
			SignedTxnWithAD: sdk.SignedTxnWithAD{
				SignedTxn: sdk.SignedTxn{
					Txn: sdk.Transaction{
						PaymentTxnFields: sdk.PaymentTxnFields{
							Receiver: sampleAddr3,
						},
					},
				},
			},
		},
	)

	output, err := fp.Process(bd)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(output.Payset))
	assert.Equal(t, sampleAddr2, output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Receiver)
	assert.Equal(t, sampleAddr2, output.Payset[0].SignedTxnWithAD.SignedTxn.Txn.Header.Sender)
}

// TestFilterProcessor_Init_Some tests initialization of the filter processor with the "any" filter type
func TestFilterProcessor_Init(t *testing.T) {

	sampleAddr1 := sdk.Address{1}
	sampleAddr2 := sdk.Address{2}
	sampleAddr3 := sdk.Address{3}

	sampleCfgStr := `---
filters:
  - any:
    - tag: txn.rcv
      expression-type: regex 
      expression: "` + sampleAddr1.String() + `"
    - tag: txn.rcv
      expression-type: equal
      expression: "` + sampleAddr2.String() + `"
`

	fpBuilder, err := processors.ProcessorBuilderByName(implementationName)
	assert.NoError(t, err)

	fp := fpBuilder.New()
	err = fp.Init(context.Background(), &conduit.PipelineInitProvider{}, plugins.MakePluginConfig(sampleCfgStr), logrus.New())
	assert.NoError(t, err)

	bd := data.BlockData{}
	bd.Payset = append(bd.Payset,

		sdk.SignedTxnInBlock{
			SignedTxnWithAD: sdk.SignedTxnWithAD{
				SignedTxn: sdk.SignedTxn{
					Txn: sdk.Transaction{
						PaymentTxnFields: sdk.PaymentTxnFields{
							Receiver: sampleAddr1,
						},
					},
				},
			},
		},
		sdk.SignedTxnInBlock{
			SignedTxnWithAD: sdk.SignedTxnWithAD{
				SignedTxn: sdk.SignedTxn{
					Txn: sdk.Transaction{
						PaymentTxnFields: sdk.PaymentTxnFields{
							Receiver: sampleAddr2,
						},
					},
				},
			},
		},
		sdk.SignedTxnInBlock{
			SignedTxnWithAD: sdk.SignedTxnWithAD{
				SignedTxn: sdk.SignedTxn{
					Txn: sdk.Transaction{
						PaymentTxnFields: sdk.PaymentTxnFields{
							Receiver: sampleAddr3,
						},
					},
				},
			},
		},
	)

	output, err := fp.Process(bd)
	require.NoError(t, err)
	assert.Equal(t, output.Payset, []sdk.SignedTxnInBlock{bd.Payset[0], bd.Payset[1]})
}

func TestFilterProcessor_SearchInner(t *testing.T) {
	sampleAddr1 := sdk.Address{1}
	cfg := `---
search-inner: true
filters:
  - any:
    - tag: txn.snd
      expression-type: equal
      expression: "` + sampleAddr1.String() + `"
`

	fp := FilterProcessor{}
	err := fp.Init(context.Background(), &conduit.PipelineInitProvider{}, plugins.MakePluginConfig(cfg), logrus.New())
	require.NoError(t, err)

	bd := testBlock(5)
	bd.Payset[1].EvalDelta.InnerTxns = []sdk.SignedTxnWithAD{
		{
			SignedTxn: sdk.SignedTxn{
				Txn: sdk.Transaction{
					Header: sdk.Header{
						Sender: sampleAddr1,
					},
				},
			},
		},
	}

	output, err := fp.Process(bd)
	require.NoError(t, err)
	assert.Equal(t, output.Payset, []sdk.SignedTxnInBlock{bd.Payset[1]})
}
