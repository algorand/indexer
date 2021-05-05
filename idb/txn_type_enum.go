package idb

import (
	"strings"

	"github.com/algorand/go-algorand/protocol"
)

// The type enum is stored in the database for each transaction. The api layer can filter
// transactions by type by including the enum in the transaction filter.
type TxnTypeEnum int

const (
	TypeEnumPay TxnTypeEnum = iota + 1
	TypeEnumKeyreg
	TypeEnumAssetConfig
	TypeEnumAssetTransfer
	TypeEnumAssetFreeze
	TypeEnumApplication
)

var typeEnumMap = map[string]TxnTypeEnum{
	"pay":    TypeEnumPay,
	"keyreg": TypeEnumKeyreg,
	"acfg":   TypeEnumAssetConfig,
	"axfer":  TypeEnumAssetTransfer,
	"afrz":   TypeEnumAssetFreeze,
	"appl":   TypeEnumApplication,
}

// Given a transaction type string `t`, return the type enum.
func GetTypeEnum(t protocol.TxType) (TxnTypeEnum, bool /*ok*/) {
	e, ok := typeEnumMap[string(t)]
	return e, ok
}

func TypeEnumString() string {
	keys := make([]string, 0, len(typeEnumMap))
	for k := range typeEnumMap {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}
