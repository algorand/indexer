package encoding

import (
	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/transactions"
)

type assetParams struct {
	basics.AssetParams
	UnitNameBytes  []byte `codec:"un64"`
	AssetNameBytes []byte `codec:"an64"`
	URLBytes       []byte `codec:"au64"`
}

type transaction struct {
	transactions.Transaction
	AssetParamsOverride assetParams `codec:"apar"`
}

type valueDelta struct {
	basics.ValueDelta
	BytesOverride []byte `codec:"bs"`
}

type byteArray struct {
	data string
}

func (ba byteArray) MarshalText() ([]byte, error) {
	return []byte(Base64([]byte(ba.data))), nil
}

func (ba *byteArray) UnmarshalText(text []byte) error {
	baNew, err := decodeBase64(string(text))
	if err != nil {
		return err
	}

	*ba = byteArray{string(baNew)}
	return nil
}

type stateDelta map[byteArray]valueDelta

type evalDelta struct {
	transactions.EvalDelta
	GlobalDeltaOverride stateDelta            `codec:"gd"`
	LocalDeltasOverride map[uint64]stateDelta `codec:"ld"`
}

type signedTxnWithAD struct {
	transactions.SignedTxnWithAD
	TxnOverride       transaction   `codec:"txn"`
	AuthAddrOverride  crypto.Digest `codec:"sgnr"`
	EvalDeltaOverride evalDelta     `codec:"dt"`
}

type tealValue struct {
	basics.TealValue
	BytesOverride []byte `codec:"tb"`
}

type keyTealValue struct {
	Key []byte    `codec:"k"`
	Tv  tealValue `codec:"v"`
}

type tealKeyValue []keyTealValue

type appLocalState struct {
	basics.AppLocalState
	KeyValueOverride tealKeyValue `codec:"tkv"`
}

type appParams struct {
	basics.AppParams
	GlobalStateOverride tealKeyValue `codec:"gs"`
}
