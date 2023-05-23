package encoding

import (
	sdk "github.com/algorand/go-algorand-sdk/v2/types"
)

type AlgodEncodedAddress sdk.Address

// MarshalText returns the address string as an array of bytes
func (addr *AlgodEncodedAddress) MarshalText() ([]byte, error) {
	var a sdk.Address
	copy(a[:], addr[:])
	return []byte(a.String()), nil
}

// UnmarshalText initializes the Address from an array of bytes.
// The bytes may be in the base32 checksum format, or the raw bytes base64 encoded.
func (addr *AlgodEncodedAddress) UnmarshalText(text []byte) error {
	var a sdk.Address
	err := a.UnmarshalText(text)
	if err != nil {
		return err
	}
	copy(addr[:], a[:])
	return nil
}

type blockHeader struct {
	sdk.BlockHeader
	ExpiredParticipationAccountsOverride []AlgodEncodedAddress `codec:"partupdrmv"`
}

type assetParams struct {
	sdk.AssetParams
	UnitNameBytes  []byte `codec:"un64"`
	AssetNameBytes []byte `codec:"an64"`
	URLBytes       []byte `codec:"au64"`
}

type transaction struct {
	sdk.Transaction
	AssetParamsOverride assetParams `codec:"apar"`
}

type valueDelta struct {
	sdk.ValueDelta
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
	sdk.EvalDelta
	GlobalDeltaOverride stateDelta            `codec:"gd"`
	LocalDeltasOverride map[uint64]stateDelta `codec:"ld"`
	LogsOverride        [][]byte              `codec:"lg"`
	InnerTxnsOverride   []signedTxnWithAD     `codec:"itx"`
}

type signedTxnWithAD struct {
	sdk.SignedTxnWithAD
	TxnOverride       transaction `codec:"txn"`
	EvalDeltaOverride evalDelta   `codec:"dt"`
}

type trimmedAccountData struct {
	baseAccountData
	MicroAlgos         uint64 `codec:"algo"`
	RewardsBase        uint64 `codec:"ebase"`
	RewardedMicroAlgos uint64 `codec:"ern"`
}

type tealValue struct {
	sdk.TealValue
	BytesOverride []byte `codec:"tb"`
}

type tealKeyValue map[byteArray]tealValue

type appLocalState struct {
	sdk.AppLocalState
	KeyValueOverride tealKeyValue `codec:"tkv"`
}

type appParams struct {
	sdk.AppParams
	GlobalStateOverride tealKeyValue `codec:"gs"`
}

type baseOnlineAccountData struct {
	_struct struct{} `codec:",omitempty,omitemptyarray"`

	VoteID          sdk.OneTimeSignatureVerifier `codec:"vote"`
	SelectionID     sdk.VRFVerifier              `codec:"sel"`
	StateProofID    sdk.Commitment               `codec:"stprf"`
	VoteFirstValid  sdk.Round                    `codec:"voteFst"`
	VoteLastValid   sdk.Round                    `codec:"voteLst"`
	VoteKeyDilution uint64                       `codec:"voteKD"`
}

type baseAccountData struct {
	_struct struct{} `codec:",omitempty,omitemptyarray"`

	Status              sdk.Status      `codec:"onl"`
	AuthAddr            sdk.Address     `codec:"spend"`
	TotalAppSchema      sdk.StateSchema `codec:"tsch"`
	TotalExtraAppPages  uint32          `codec:"teap"`
	TotalAssetParams    uint64          `codec:"tasp"`
	TotalAssets         uint64          `codec:"tas"`
	TotalAppParams      uint64          `codec:"tapp"`
	TotalAppLocalStates uint64          `codec:"tapl"`
	TotalBoxes          uint64          `codec:"tbx"`
	TotalBoxBytes       uint64          `codec:"tbxb"`

	baseOnlineAccountData
}
