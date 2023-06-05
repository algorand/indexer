package encoding

import (
	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/crypto/merklesignature"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
)

// AlgodEncodedAddress is an address encoded in the format used by algod.
type AlgodEncodedAddress basics.Address

type blockHeader struct {
	bookkeeping.BlockHeader
	BranchOverride                       crypto.Digest         `codec:"prev"`
	FeeSinkOverride                      crypto.Digest         `codec:"fees"`
	RewardsPoolOverride                  crypto.Digest         `codec:"rwd"`
	ExpiredParticipationAccountsOverride []AlgodEncodedAddress `codec:"partupdrmv"`
}

type assetParams struct {
	basics.AssetParams
	UnitNameBytes    []byte        `codec:"un64"`
	AssetNameBytes   []byte        `codec:"an64"`
	URLBytes         []byte        `codec:"au64"`
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
	LogsOverride        [][]byte              `codec:"lg"`
	InnerTxnsOverride   []signedTxnWithAD     `codec:"itx"`
}

type signedTxnWithAD struct {
	transactions.SignedTxnWithAD
	TxnOverride       transaction   `codec:"txn"`
	AuthAddrOverride  crypto.Digest `codec:"sgnr"`
	EvalDeltaOverride evalDelta     `codec:"dt"`
}

type trimmedAccountData struct {
	basics.AccountData
	AuthAddrOverride crypto.Digest `codec:"spend"`
}

type tealValue struct {
	basics.TealValue
	BytesOverride []byte `codec:"tb"`
}

type tealKeyValue map[byteArray]tealValue

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

type baseOnlineAccountData struct {
	_struct struct{} `codec:",omitempty,omitemptyarray"`

	VoteID          crypto.OneTimeSignatureVerifier `codec:"vote"`
	SelectionID     crypto.VRFVerifier              `codec:"sel"`
	StateProofID    merklesignature.Commitment      `codec:"stprf"`
	VoteFirstValid  basics.Round                    `codec:"voteFst"`
	VoteLastValid   basics.Round                    `codec:"voteLst"`
	VoteKeyDilution uint64                          `codec:"voteKD"`
}

type baseAccountData struct {
	_struct struct{} `codec:",omitempty,omitemptyarray"`

	Status              basics.Status      `codec:"onl"`
	AuthAddr            crypto.Digest      `codec:"spend"`
	TotalAppSchema      basics.StateSchema `codec:"tsch"`
	TotalExtraAppPages  uint32             `codec:"teap"`
	TotalAssetParams    uint64             `codec:"tasp"`
	TotalAssets         uint64             `codec:"tas"`
	TotalAppParams      uint64             `codec:"tapp"`
	TotalAppLocalStates uint64             `codec:"tapl"`
	TotalBoxes          uint64             `codec:"tbx"`
	TotalBoxBytes       uint64             `codec:"tbxb"`

	baseOnlineAccountData
}
