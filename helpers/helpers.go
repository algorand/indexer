package helpers

import (
	"fmt"

	"github.com/algorand/indexer/protocol"
	"github.com/algorand/indexer/types"

	"github.com/algorand/go-algorand-sdk/v2/encoding/json"
	"github.com/algorand/go-algorand-sdk/v2/encoding/msgpack"
	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	protocol2 "github.com/algorand/go-algorand/protocol"
)

// TODO: remove this file once all types have been converted to sdk types.

// ConvertParams converts basics.AssetParams to sdk.AssetParams
func ConvertParams(params basics.AssetParams) sdk.AssetParams {
	return sdk.AssetParams{
		Total:         params.Total,
		Decimals:      params.Decimals,
		DefaultFrozen: params.DefaultFrozen,
		UnitName:      params.UnitName,
		AssetName:     params.AssetName,
		URL:           params.URL,
		MetadataHash:  params.MetadataHash,
		Manager:       sdk.Address(params.Manager),
		Reserve:       sdk.Address(params.Reserve),
		Freeze:        sdk.Address(params.Freeze),
		Clawback:      sdk.Address(params.Clawback),
	}
}

// ConvertValidatedBlock converts ledgercore.ValidatedBlock to types.ValidatedBlock
func ConvertValidatedBlock(vb ledgercore.ValidatedBlock) (types.ValidatedBlock, error) {
	var ret types.ValidatedBlock
	var block sdk.Block
	var delta sdk.LedgerStateDelta
	// block
	b := msgpack.Encode(vb.Block())
	err := msgpack.Decode(b, &block)
	if err != nil {
		return ret, fmt.Errorf("ConvertValidatedBlock block err: %v", err)
	}
	//delta
	d := msgpack.Encode(vb.Delta())
	err = msgpack.Decode(d, &delta)
	if err != nil {
		return ret, fmt.Errorf("ConvertValidatedBlock delta err: %v", err)
	}
	ret.Block = block
	ret.Delta = delta
	return ret, nil
}

// ConvertConsensusType returns a go-algorand/protocol.ConsensusVersion type
func ConvertConsensusType(v protocol.ConsensusVersion) protocol2.ConsensusVersion {
	return protocol2.ConsensusVersion(v)
}

// ConvertAccountType returns a basics.AccountData type
func ConvertAccountType(account sdk.Account) basics.AccountData {
	return basics.AccountData{
		Status:          basics.Status(account.Status),
		MicroAlgos:      basics.MicroAlgos{Raw: account.MicroAlgos},
		VoteID:          account.VoteID,
		SelectionID:     account.SelectionID,
		VoteLastValid:   basics.Round(account.VoteLastValid),
		VoteKeyDilution: account.VoteKeyDilution,
	}
}

// ConvertGenesis returns bookkeeping.Genesis
func ConvertGenesis(genesis *sdk.Genesis) (*bookkeeping.Genesis, error) {
	var ret bookkeeping.Genesis
	b := json.Encode(genesis)
	err := json.Decode(b, &ret)
	if err != nil {
		return &ret, fmt.Errorf("ConvertGenesis err: %v", err)
	}
	return &ret, nil
}
