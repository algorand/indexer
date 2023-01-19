package idb

import (
	"strings"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
)

// TxnTypeEnum describes the type of a transaction. It is stored in the database
// for each transaction. The api layer can filter transactions by type by including
// the enum in the transaction filter.
type TxnTypeEnum int

// All possible transaction types.
const (
	TypeEnumPay TxnTypeEnum = iota + 1
	TypeEnumKeyreg
	TypeEnumAssetConfig
	TypeEnumAssetTransfer
	TypeEnumAssetFreeze
	TypeEnumApplication
	TypeEnumStateProof
)

var typeEnumMap = map[string]TxnTypeEnum{
	"pay":    TypeEnumPay,
	"keyreg": TypeEnumKeyreg,
	"acfg":   TypeEnumAssetConfig,
	"axfer":  TypeEnumAssetTransfer,
	"afrz":   TypeEnumAssetFreeze,
	"appl":   TypeEnumApplication,
	"stpf":   TypeEnumStateProof,
}

func makeTypeEnumString() string {
	keys := make([]string, 0, len(typeEnumMap))
	for k := range typeEnumMap {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}

// TxnTypeEnumString is a comma-separated list of possible transaction types.
var TxnTypeEnumString = makeTypeEnumString()

// GetTypeEnum returns the enum for the given transaction type string.
func GetTypeEnum(t sdk.TxType) (TxnTypeEnum, bool /*ok*/) {
	e, ok := typeEnumMap[string(t)]
	return e, ok
}
