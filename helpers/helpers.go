package helpers

import (
	"bytes"
	"crypto/sha512"
	"encoding/base32"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/algorand/go-algorand-sdk/encoding/json"
	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	sdk "github.com/algorand/go-algorand-sdk/types"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-codec/codec"
	"github.com/algorand/go-stateproof-verification/stateproof"
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

// IndexerTxn overrides StateProof
type IndexerTxn struct {
	_struct struct{} `codec:",omitempty,omitemptyarray"`
	sdk.Transaction
	// StateProof override with concrete type
	StateProof stateproof.StateProof `codec:"sp"`
}

// GetStateProofTxID computes StateProof txid
func GetStateProofTxID(txn sdk.Transaction) string {
	var buf strings.Builder
	enc := codec.NewEncoder(&buf, json.CodecHandle)
	enc.Encode(txn)
	dec := codec.NewDecoder(strings.NewReader(buf.String()), json.CodecHandle)
	var t IndexerTxn
	err := dec.Decode(&t)
	if err != nil {
		return ""
	}
	// Encode the transaction as msgpack
	encodedTx := msgpack.Encode(t)
	var txidPrefix = []byte("TX")
	msgParts := [][]byte{txidPrefix, encodedTx}
	toBeSigned := bytes.Join(msgParts, nil)
	txid32 := sha512.Sum512_256(toBeSigned)
	txidbytes := txid32[:]
	txid := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(txidbytes)
	return txid
}
