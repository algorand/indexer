package helpers

import (
	"encoding/base64"
	"fmt"

	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	sdk "github.com/algorand/go-algorand-sdk/types"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	protocol2 "github.com/algorand/go-algorand/protocol"
	"github.com/algorand/indexer/protocol"
	"github.com/algorand/indexer/types"
)

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
	b64data := base64.StdEncoding.EncodeToString(msgpack.Encode(vb.Block()))
	err := ret.Block.FromBase64String(b64data)
	if err != nil {
		return ret, fmt.Errorf("ConvertValidatedBlock err: %v", err)
	}
	ret.Delta = vb.Delta()
	return ret, nil
}

// ConvertConsensusType returns a go-algorand/protocol.ConsensusVersion type
func ConvertConsensusType(v protocol.ConsensusVersion) protocol2.ConsensusVersion {
	return protocol2.ConsensusVersion(v)
}
