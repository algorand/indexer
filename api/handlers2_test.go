package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/algorand/indexer/api/generated"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/mocks"
)

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
			idb.TransactionFilter{Limit: defaultTransactionsLimit},
			nil,
		},
		{
			"Limit",
			generated.SearchForTransactionsParams{Limit: uint64Ptr(defaultTransactionsLimit + 10)},
			idb.TransactionFilter{Limit: defaultTransactionsLimit + 10},
			nil,
		},
		{
			"Limit Max",
			generated.SearchForTransactionsParams{Limit: uint64Ptr(maxTransactionsLimit + 10)},
			idb.TransactionFilter{Limit: maxTransactionsLimit},
			nil,
		},
		{
			"Int field",
			generated.SearchForTransactionsParams{AssetId: uint64Ptr(1234)},
			idb.TransactionFilter{AssetId: 1234, Limit: defaultTransactionsLimit},
			nil,
		},
		{
			"Pointer field",
			generated.SearchForTransactionsParams{Round: uint64Ptr(1234)},
			idb.TransactionFilter{Round: uint64Ptr(1234), Limit: defaultTransactionsLimit},
			nil,
		},
		{
			"Base64 field",
			generated.SearchForTransactionsParams{NotePrefix: bytePtr([]byte("SomeData"))},
			idb.TransactionFilter{NotePrefix: []byte("SomeData"), Limit: defaultTransactionsLimit},
			nil,
		},
		{
			"Enum fields",
			generated.SearchForTransactionsParams{TxType: strPtr("pay"), SigType: strPtr("lsig")},
			idb.TransactionFilter{TypeEnum: 1, SigType: "lsig", Limit: defaultTransactionsLimit},
			nil,
		},
		{
			"Date time fields",
			generated.SearchForTransactionsParams{AfterTime: timePtr(time.Date(2020, 3, 4, 12, 0, 0, 0, time.FixedZone("UTC", 0)))},
			idb.TransactionFilter{AfterTime: time.Date(2020, 3, 4, 12, 0, 0, 0, time.FixedZone("UTC", 0)), Limit: defaultTransactionsLimit},
			nil,
		},
		{
			"Invalid Enum fields",
			generated.SearchForTransactionsParams{TxType: strPtr("micro"), SigType: strPtr("handshake")},
			idb.TransactionFilter{},
			[]string{errUnknownSigType, errUnknownTxType},
		},
		{
			"As many fields as possible",
			generated.SearchForTransactionsParams{
				Limit:               uint64Ptr(defaultTransactionsLimit + 1),
				Next:                strPtr("next-token"),
				NotePrefix:          bytePtr([]byte("custom-note")),
				TxType:              strPtr("pay"),
				SigType:             strPtr("sig"),
				TxId:                bytePtr([]byte("tx-id")),
				Round:               nil,
				MinRound:            uint64Ptr(2),
				MaxRound:            uint64Ptr(3),
				AssetId:             uint64Ptr(4),
				BeforeTime:          timePtr(time.Date(2021, 1, 1, 1, 0, 0, 0, time.FixedZone("UTC", 0))),
				AfterTime:           timePtr(time.Date(2022, 2, 2, 2, 0, 0, 0, time.FixedZone("UTC", 0))),
				CurrencyGreaterThan: uint64Ptr(5),
				CurrencyLessThan:    uint64Ptr(8),
				Address:             strPtr("YXGBWVBK764KGYPX6ENIADKXPWLBNAZ7MTXDZULZWGOBO2W6IAR622VSLA"),
				AddressRole:         strPtr("sender"),
				ExcludeCloseTo:      boolPtr(true),
			},
			idb.TransactionFilter{
				Limit:             defaultTransactionsLimit + 1,
				NextToken:         "next-token",
				NotePrefix:        []byte("custom-note"),
				TypeEnum:          1,
				SigType:           "sig",
				Txid:              []byte("tx-id"),
				Round:             nil,
				MinRound:          2,
				MaxRound:          3,
				AssetId:           4,
				BeforeTime:        time.Date(2021, 1, 1, 1, 0, 0, 0, time.FixedZone("UTC", 0)),
				AfterTime:         time.Date(2022, 2, 2, 2, 0, 0, 0, time.FixedZone("UTC", 0)),
				MinAlgos:          0,
				MaxAlgos:          0,
				MinAssetAmount:    5 + 1, // GT -> Min
				MaxAssetAmount:    8 - 1, // LT -> Max
				EffectiveAmountGt: 0,
				EffectiveAmountLt: 0,
				Address:           []byte{197, 204, 27, 84, 42, 255, 184, 163, 97, 247, 241, 26, 128, 13, 87, 125, 150, 22, 131, 63, 100, 238, 60, 209, 121, 177, 156, 23, 106, 222, 64, 35},
				AddressRole:       9,
				Offset:            nil,
				OffsetLT:          nil,
				OffsetGT:          nil,
			},
			nil,
		},
		{
			name: "Round + Min/Max Error",
			params: generated.SearchForTransactionsParams{
				Round:    uint64Ptr(10),
				MinRound: uint64Ptr(5),
				MaxRound: uint64Ptr(15),
			},
			filter:        idb.TransactionFilter{},
			errorContains: []string{errInvalidRoundMinMax},
		},
		{
			name:          "Swapped Min/Max Round",
			params:        generated.SearchForTransactionsParams{MinRound: uint64Ptr(20), MaxRound: uint64Ptr(10)},
			filter:        idb.TransactionFilter{MinRound: 10, MaxRound: 20, Limit: defaultTransactionsLimit},
			errorContains: nil,
		},
		{
			name:          "Illegal Address",
			params:        generated.SearchForTransactionsParams{Address: strPtr("Not-our-base32-thing")},
			filter:        idb.TransactionFilter{},
			errorContains: []string{errUnableToParseAddress},
		},
		{
			name:          "Unknown address role error",
			params:        generated.SearchForTransactionsParams{AddressRole: strPtr("unknown")},
			filter:        idb.TransactionFilter{},
			errorContains: []string{errUnknownAddressRole},
		},
		{
			name:          "Bitmask sender + closeTo(true)",
			params:        generated.SearchForTransactionsParams{AddressRole: strPtr("sender"), ExcludeCloseTo: boolPtr(true)},
			filter:        idb.TransactionFilter{AddressRole: 9, Limit: defaultTransactionsLimit},
			errorContains: nil,
		},
		{
			name:          "Bitmask sender + closeTo(false)",
			params:        generated.SearchForTransactionsParams{AddressRole: strPtr("sender"), ExcludeCloseTo: boolPtr(false)},
			filter:        idb.TransactionFilter{AddressRole: 9, Limit: defaultTransactionsLimit},
			errorContains: nil,
		},
		{
			name:          "Bitmask receiver + closeTo(true)",
			params:        generated.SearchForTransactionsParams{AddressRole: strPtr("receiver"), ExcludeCloseTo: boolPtr(true)},
			filter:        idb.TransactionFilter{AddressRole: 18, Limit: defaultTransactionsLimit},
			errorContains: nil,
		},
		{
			name:          "Bitmask receiver + closeTo(false)",
			params:        generated.SearchForTransactionsParams{AddressRole: strPtr("receiver"), ExcludeCloseTo: boolPtr(false)},
			filter:        idb.TransactionFilter{AddressRole: 54, Limit: defaultTransactionsLimit},
			errorContains: nil,
		},
		{
			name:          "Bitmask receiver + implicit closeTo (false)",
			params:        generated.SearchForTransactionsParams{AddressRole: strPtr("receiver")},
			filter:        idb.TransactionFilter{AddressRole: 54, Limit: defaultTransactionsLimit},
			errorContains: nil,
		},
		{
			name:          "Bitmask freeze-target",
			params:        generated.SearchForTransactionsParams{AddressRole: strPtr("freeze-target")},
			filter:        idb.TransactionFilter{AddressRole: 64, Limit: defaultTransactionsLimit},
			errorContains: nil,
		},
		{
			name:          "Currency to Algos when no asset-id",
			params:        generated.SearchForTransactionsParams{CurrencyGreaterThan: uint64Ptr(9), CurrencyLessThan: uint64Ptr(21)},
			filter:        idb.TransactionFilter{MinAlgos: 10, MaxAlgos: 20, Limit: defaultTransactionsLimit},
			errorContains: nil,
		},
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
				assert.Equal(t, test.errorContains != nil, err != nil)
				assert.Equal(t, test.filter, filter)
			}
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
			indexerDb = mockIndexer
			si := ServerImplementation{
				EnableAddressSearchRoundRewind: true,
				db:                             mockIndexer,
			}

			roundTime := time.Now()
			roundTime64 := uint64(roundTime.Unix())

			ch := make(chan idb.TxnRow, len(test.txnBytes))
			for _, bytes := range test.txnBytes {
				txnRow := idb.TxnRow{
					Round:     1,
					Intra:     2,
					RoundTime: roundTime,
					TxnBytes:  bytes,
					AssetId:   0,
					Extra: idb.TxnExtra{
						AssetCloseAmount: 0,
					},
					Error: nil,
				}
				ch <- txnRow
			}

			close(ch)
			var outCh <-chan idb.TxnRow = ch
			mockIndexer.On("Transactions", mock.Anything, mock.Anything).Return(outCh)

			// Call the function
			results, _, err := si.fetchTransactions(context.Background(), idb.TransactionFilter{})
			assert.NoError(t, err)

			printIt := false
			if printIt {
				fmt.Printf("Test: %s\n", test.name)
				for _, result := range results {
					fmt.Println("-------------------")
					str, _ := json.Marshal(result)
					fmt.Printf("%s\n", str)
				}
				fmt.Println("-------------------")
			}

			// Verify the results
			assert.Equal(t, len(test.response), len(results))
			for i, expected := range test.response {
				actual := results[i]
				// This is set in the mock above, so override it in the expected value.
				expected.RoundTime = &roundTime64
				fmt.Println(roundTime64)
				assert.EqualValues(t, expected, actual)
			}
		})
	}
}
