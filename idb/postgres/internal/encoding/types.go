package encoding

import (
	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
)

type blockHeader struct {
	bookkeeping.BlockHeader
	BranchOverride      crypto.Digest `codec:"prev"`
	FeeSinkOverride     crypto.Digest `codec:"fees"`
	RewardsPoolOverride crypto.Digest `codec:"rwd"`
}

type assetParams struct {
	basics.AssetParams
	ManagerOverride  crypto.Digest `codec:"m"`
	ReserveOverride  crypto.Digest `codec:"r"`
	FreezeOverride   crypto.Digest `codec:"f"`
	ClawbackOverride crypto.Digest `codec:"c"`
}

type transaction struct {
	transactions.Transaction
	SenderOverride           crypto.Digest   `codec:"snd"`
	RekeyToOverride          crypto.Digest   `codec:"rekey"`
	ReceiverOverride         crypto.Digest   `codec:"rcv"`
	CloseRemainderToOverride crypto.Digest   `codec:"close"`
	AssetParamsOverride      assetParams     `codec:"apar"`
	AssetSenderOverride      crypto.Digest   `codec:"asnd"`
	AssetReceiverOverride    crypto.Digest   `codec:"arcv"`
	AssetCloseToOverride     crypto.Digest   `codec:"aclose"`
	FreezeAccountOverride    crypto.Digest   `codec:"fadd"`
	AccountsOverride         []crypto.Digest `codec:"apat"`
}

type valueDelta struct {
	basics.ValueDelta
	BytesOverride []byte `codec:"bs"`
}

type byteArray struct {
	data string
}

type stateDelta map[byteArray]valueDelta

type evalDelta struct {
	basics.EvalDelta
	GlobalDeltaOverride stateDelta            `codec:"gd"`
	LocalDeltasOverride map[uint64]stateDelta `codec:"ld"`
}

type signedTxnWithAD struct {
	transactions.SignedTxnWithAD
	TxnOverride       transaction   `codec:"txn"`
	AuthAddrOverride  crypto.Digest `codec:"sgnr"`
	EvalDeltaOverride evalDelta     `codec:"dt"`
}

type accountData struct {
	basics.AccountData
	AuthAddrOverride crypto.Digest `codec:"spend"`
}

type tealValue struct {
	basics.TealValue
	BytesOverride []byte `codec:"tb"`
}

type keyTealValue struct {
	Key []byte    `codec:"k"`
	Tv  tealValue `codec:"v"`
}

type tealKeyValue struct {
	They []keyTealValue
}

type appLocalState struct {
	basics.AppLocalState
	KeyValueOverride tealKeyValue `codec:"tkv"`
}

type appParams struct {
	basics.AppParams
	GlobalStateOverride tealKeyValue `codec:"gs"`
}

type specialAddresses struct {
	transactions.SpecialAddresses
	FeeSinkOverride     crypto.Digest `codec:"FeeSink"`
	RewardsPoolOverride crypto.Digest `codec:"RewardsPool"`
}
