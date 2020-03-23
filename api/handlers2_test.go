package api

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/algorand/indexer/api/generated"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"io/ioutil"
	"testing"
	"time"
)

/*
func init() {
	gen := genesis{
		genesisHash: []byte("TestGenesisHash"),
		genesisID:   "TestGenesisID",
	}
}
*/

func TestTransactionParamToTransactionFilter(t *testing.T) {
	tests := []struct {
		name          string
		params        generated.SearchForTransactionsParams
		filter        idb.TransactionFilter
		errorContains []string
	}{
		{
			"Default",
			generated.SearchForTransactionsParams{},
			idb.TransactionFilter{},
			nil,
		},
		{
			"Int field",
			generated.SearchForTransactionsParams{AssetId: uint64Ptr(1234)},
			idb.TransactionFilter{AssetId: 1234},
			nil,
		},
		{
			"Pointer field",
			generated.SearchForTransactionsParams{Round: uint64Ptr(1234)},
			idb.TransactionFilter{Round: uint64Ptr(1234)},
			nil,
		},
		{
			"Base64 field",
			generated.SearchForTransactionsParams{NotePrefix: bytePtr([]byte("SomeData"))},
			idb.TransactionFilter{NotePrefix: []byte("SomeData")},
			nil,
		},
		{
			"Enum fields",
			generated.SearchForTransactionsParams{TxType: strPtr("pay"), SigType: strPtr("lsig")},
			idb.TransactionFilter{TypeEnum: 1, SigType: "lsig"},
			nil,
		},
		{
			"Date time fields",
			generated.SearchForTransactionsParams{AfterTime: timePtr(time.Date(2020, 3, 4, 12, 0, 0, 0, time.FixedZone("UTC", 0)))},
			idb.TransactionFilter{AfterTime: time.Date(2020, 3, 4, 12, 0, 0, 0, time.FixedZone("UTC", 0))},
			nil,
		},
		{
			"Invalid Enum fields",
			generated.SearchForTransactionsParams{TxType: strPtr("micro"), SigType: strPtr("handshake")},
			idb.TransactionFilter{},
			[]string{"invalid sigtype", "invalid transaction type"},
		},
		/*
			{
				"Invalid Base64 field",
				generated.SearchForTransactionsParams{Noteprefix:strPtr("U29tZURhdGE{}{}{}=")},
				idb.TransactionFilter{},
				[]string{ "illegal base64 data" },
			},
			{
				"Invalid Date time fields",
				generated.SearchForTransactionsParams{AfterTime:strPtr("2020-03-04T12:00:00")},
				idb.TransactionFilter{},
				[]string{"unable to decode 'after-time'"},
			},
		*/
	}

	for _, test := range tests {
		//test := test
		t.Run(test.name, func(t *testing.T) {
			//t.Parallel()
			filter, err := transactionParamsToTransactionFilter(test.params)
			if test.errorContains != nil {
				for _, msg := range test.errorContains {
					assert.Contains(t, err.Error(), msg)
				}
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, test.errorContains != nil, err != nil)
			assert.Equal(t, test.filter, filter)
		})
	}
}

func loadResourceFileOrPanic(path string) []byte {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("Failed to load resource file: '%s'", path))
	}
	return data
}

func loadTransactionFromFile(path string) generated.Transaction {
	data := loadResourceFileOrPanic(path)
	var ret generated.Transaction
	if err := json.Unmarshal(data, &ret); err != nil {
		panic(fmt.Sprintf("Failed to build transaction from file: %s", path))
	}
	return ret
}

func TestFetchTransactions(t *testing.T) {
	// Add in txnRows (with TxnBytes to parse), verify that they are properly serialized to generated.TransactionResponse
	tests := []struct {
		name     string
		txnBytes [][]byte
		response []generated.Transaction
	}{
		{
			name: "Payment",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/payment.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/payment.response"),
			},
		},
		{
			name: "Key Registration",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/keyreg.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/keyreg.response"),
			},
		},
		{
			name: "Asset Configuration",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/asset_config.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/asset_config.response"),
			},
		},
		{
			name: "Asset Transfer",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/asset_transfer.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/asset_transfer.response"),
			},
		},
		{
			name: "Asset Freeze",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/asset_freeze.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/asset_freeze.response"),
			},
		},
		{
			name: "Multisig Transaction",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/multisig.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/multisig.response"),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Setup the mocked responses

			mockIndexer := &mocks.IndexerDb{}
			IndexerDb = mockIndexer

			ch := make(chan idb.TxnRow, len(test.txnBytes))
			for _, bytes := range test.txnBytes {
				txnRow := idb.TxnRow{
					Round:     1,
					RoundTime: time.Now(),
					Extra: idb.TxnExtra{
						AssetCloseAmount: 0,
					},
					Intra:    2,
					TxnBytes: bytes,
					Error:    nil,
				}
				ch <- txnRow
			}

			close(ch)
			var outCh <-chan idb.TxnRow = ch
			mockIndexer.On("Transactions", mock.Anything, mock.Anything).Return(outCh)

			// Call the function
			results, err := fetchTransactions(idb.TransactionFilter{}, context.Background())
			assert.NoError(t, err)

			/*
				fmt.Printf("Test: %s\n", test.name)
				for _, result := range results {
					fmt.Println("-------------------")
					str, _ := json.Marshal(result)
					fmt.Printf("%s\n", str)
				}
				fmt.Println("-------------------")
			*/

			// Verify the results
			assert.Equal(t, len(test.response), len(results))
			for i, _ := range test.response {
				expected := test.response[i]
				actual := results[i]
				assert.Equal(t, expected, actual)
			}
		})
	}
}
