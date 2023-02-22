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
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	protocol2 "github.com/algorand/go-algorand/protocol"
	"github.com/algorand/go-algorand/rpcs"
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

// UnconvertValidatedBlock converts types.ValidatedBlock to ledgercore.ValidatedBlock
func UnconvertValidatedBlock(vb types.ValidatedBlock) (ledgercore.ValidatedBlock, error) {
	var block bookkeeping.Block
	var delta ledgercore.StateDelta
	// block
	b := msgpack.Encode(vb.Block)
	err := msgpack.Decode(b, &block)
	if err != nil {
		return ledgercore.ValidatedBlock{}, fmt.Errorf("ConvertValidatedBlock block err: %v", err)
	}
	//delta
	d := msgpack.Encode(vb.Delta)
	err = msgpack.Decode(d, &delta)
	if err != nil {
		return ledgercore.ValidatedBlock{}, fmt.Errorf("ConvertValidatedBlock delta err: %v", err)
	}
	return ledgercore.MakeValidatedBlock(block, delta), nil
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

// ConvertEncodedBlockCert returns rpcs.EncodedBlockCert
func ConvertEncodedBlockCert(blockCert types.EncodedBlockCert) (rpcs.EncodedBlockCert, error) {
	var ret rpcs.EncodedBlockCert
	b := msgpack.Encode(blockCert)
	err := msgpack.Decode(b, &ret)
	if err != nil {
		return ret, fmt.Errorf("ConvertEncodedBlockCert err: %v", err)
	}
	return ret, nil
}

// UnonvertEncodedBlockCert returns types.EncodedBlockCert
func UnonvertEncodedBlockCert(blockCert *rpcs.EncodedBlockCert) (*types.EncodedBlockCert, error) {
	var ret types.EncodedBlockCert
	b := msgpack.Encode(*blockCert)
	err := msgpack.Decode(b, &ret)
	if err != nil {
		return &ret, fmt.Errorf("UnonvertEncodedBlockCert err: %v", err)
	}
	return &ret, nil
}

// ConvertPayset returns sdk.SignedTxnInBlock
func ConvertPayset(payset transactions.Payset) ([]sdk.SignedTxnInBlock, error) {
	var ret []sdk.SignedTxnInBlock
	b := msgpack.Encode(payset)
	err := msgpack.Decode(b, &ret)
	if err != nil {
		return ret, fmt.Errorf("ConvertPayset err: %v", err)
	}
	return ret, nil
}

// ConvertLedgerStateDelta returns sdk.LedgerStateDelta
func ConvertLedgerStateDelta(payset ledgercore.StateDelta) (sdk.LedgerStateDelta, error) {
	var ret sdk.LedgerStateDelta
	b := msgpack.Encode(payset)
	err := msgpack.Decode(b, &ret)
	if err != nil {
		return ret, fmt.Errorf("ConvertLedgerStateDelta err: %v", err)
	}
	return ret, nil
}
