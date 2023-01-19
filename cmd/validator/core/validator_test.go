package core

import (
	"fmt"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/require"

	"github.com/algorand/go-algorand-sdk/v2/types"

	"github.com/algorand/indexer/api"
)

type mockProcessor struct {
	result Result
	err    error
}

func (m *mockProcessor) ProcessAddress(_ []byte, _ []byte) (Result, error) {
	return m.result, m.err
}

func TestCallProcessor(t *testing.T) {
	mockParams := Params{
		AlgodURL:   "http://localhost/algod",
		IndexerURL: "http://localhost/indexer",
	}
	var testAddr types.Address
	testAddr[0] = 1

	type args struct {
		processor   Processor
		addrInput   string
		config      Params
		algodData   string
		algodCode   int
		indexerData string
		indexerCode int
		errorStr    string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "Bad address",
			args: args{addrInput: "399 Boylston Street", errorStr: "unable to decode address"},
		}, {
			name: "algod: acct limit",
			args: args{
				algodCode: 400,
				algodData: api.ErrResultLimitReached,
				errorStr:  fmt.Sprintf("error getting algod data (%s)", api.ErrResultLimitReached),
			},
		}, {
			name: "algod: unknown error",
			args: args{
				algodCode: 500,
				algodData: "server sent GOAWAY and closed the connection",
				errorStr:  "error getting algod data (server sent GOAWAY and closed the connection): bad status: 500",
			},
		}, {
			name: "indexer: acct limit",
			args: args{
				indexerCode: 400,
				indexerData: api.ErrResultLimitReached,
				errorStr:    fmt.Sprintf("error getting indexer data (%s)", api.ErrResultLimitReached),
			},
		}, {
			name: "indexer: acct not found",
			args: args{
				indexerCode: 404,
				indexerData: api.ErrNoAccountsFound,
				errorStr:    fmt.Sprintf("error getting indexer data (%s)", api.ErrNoAccountsFound),
			},
		}, {
			name: "indexer: unknown",
			args: args{
				indexerCode: 500,
				indexerData: "server sent GOAWAY and closed the connection",
				errorStr:    "error getting indexer data (server sent GOAWAY and closed the connection): bad status: 500",
			},
		}, {
			name: "processor: error",
			args: args{
				processor: &mockProcessor{
					err: fmt.Errorf("ask again tomorrow"),
				},
				addrInput: testAddr.String(),
				errorStr:  fmt.Sprintf("error processing account %s: ask again tomorrow", testAddr.String()),
			},
		},
	}
	setDefaults := func(a args) args {
		if a.config == (Params{}) {
			a.config = mockParams
		}
		if a.algodCode == 0 {
			a.algodCode = 200
			if a.algodData == "" {
				a.algodData = "{}"
			}
		}
		if a.indexerCode == 0 {
			a.indexerCode = 200
			if a.indexerData == "" {
				a.indexerData = "{}"
			}
		}
		return a
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.args = setDefaults(tt.args)
			// Given: mock http responses
			if tt.args.algodData != "" || tt.args.indexerData != "" {
				httpmock.Activate()
				defer httpmock.DeactivateAndReset()
			}
			if tt.args.algodData != "" {
				httpmock.RegisterResponder("GET", "=~"+mockParams.AlgodURL+".*",
					httpmock.NewStringResponder(tt.args.algodCode, tt.args.algodData))
			}
			if tt.args.indexerData != "" {
				httpmock.RegisterResponder("GET", "=~"+mockParams.IndexerURL+".*",
					httpmock.NewStringResponder(tt.args.indexerCode, tt.args.indexerData))
			}

			// When: call CallProcessor
			// We only use CallProcessor once, so a buffered channel of 1 should be sufficient.
			results := make(chan Result, 1)
			CallProcessor(tt.args.processor, tt.args.addrInput, tt.args.config, results)
			close(results)

			// Then: we get the expected result.
			for result := range results {
				if tt.args.errorStr != "" {
					require.ErrorContains(t, result.Error, tt.args.errorStr)
				} else {
					require.NoError(t, result.Error)
				}
			}
		})
	}
}
