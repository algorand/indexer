package util

import (
	"bytes"
	"compress/gzip"
	"encoding/base32"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/algorand/go-algorand-sdk/encoding/json"
	sdk "github.com/algorand/go-algorand-sdk/types"
	"github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/protocol"
	"github.com/algorand/go-codec/codec"
)

const (
	checksumLength = 4
)

var base32Encoder = base32.StdEncoding.WithPadding(base32.NoPadding)

// EncodeToFile is used to encode an object to a file. If the file ends in .gz it will be gzipped.
func EncodeToFile(filename string, v interface{}, pretty bool) error {
	var writer io.Writer

	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("EncodeToFile(): failed to create %s: %w", filename, err)
	}
	defer file.Close()

	if strings.HasSuffix(filename, ".gz") {
		gz := gzip.NewWriter(file)
		gz.Name = filename
		defer gz.Close()
		writer = gz
	} else {
		writer = file
	}

	handle := json.CodecHandle
	if pretty {
		handle.Indent = 2
	} else {
		handle.Indent = 0
	}
	enc := codec.NewEncoder(writer, handle)
	return enc.Encode(v)
}

// DecodeFromFile is used to decode a file to an object.
func DecodeFromFile(filename string, v interface{}) error {
	// Streaming into the decoder was slow.
	fileBytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("DecodeFromFile(): failed to read %s: %w", filename, err)
	}

	var reader io.Reader = bytes.NewReader(fileBytes)

	if strings.HasSuffix(filename, ".gz") {
		gz, err := gzip.NewReader(reader)
		defer gz.Close()
		if err != nil {
			return fmt.Errorf("DecodeFromFile(): failed to make gzip reader: %w", err)
		}
		reader = gz
	}

	enc := codec.NewDecoder(reader, json.CodecHandle)
	return enc.Decode(v)
}

// PrintableUTF8OrEmpty checks to see if the entire string is a UTF8 printable string.
// If this is the case, the string is returned as is. Otherwise, the empty string is returned.
func PrintableUTF8OrEmpty(in string) string {
	// iterate throughout all the characters in the string to see if they are all printable.
	// when range iterating on go strings, go decode each element as a utf8 rune.
	for _, c := range in {
		// is this a printable character, or invalid rune ?
		if c == utf8.RuneError || !unicode.IsPrint(c) {
			return ""
		}
	}
	return in
}

// KeysStringBool returns all of the keys in the map joined by a comma.
func KeysStringBool(m map[string]bool) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}

// MaybeFail exits if there was an error.
func MaybeFail(err error, errfmt string, params ...interface{}) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, errfmt, params...)
	fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
	os.Exit(1)
}

var oneLineJSONCodecHandle *codec.JsonHandle

// JSONOneLine converts an object into JSON
func JSONOneLine(obj interface{}) string {
	var b []byte
	enc := codec.NewEncoderBytes(&b, oneLineJSONCodecHandle)
	enc.MustEncode(obj)
	return string(b)
}

// DecodeSignedTxn converts a SignedTxnInBlock from a block to SignedTxn and its
// associated ApplyData.
func DecodeSignedTxn(bh sdk.BlockHeader, stb sdk.SignedTxnInBlock) (sdk.SignedTxn, sdk.ApplyData, error) {
	st := stb.SignedTxn
	ad := stb.ApplyData

	proto, ok := config.Consensus[protocol.ConsensusVersion(bh.CurrentProtocol)]
	if !ok {
		return sdk.SignedTxn{}, sdk.ApplyData{},
			fmt.Errorf("consensus protocol %s not found", bh.CurrentProtocol)
	}
	if !proto.SupportSignedTxnInBlock {
		return st, sdk.ApplyData{}, nil
	}

	if st.Txn.GenesisID != "" {
		return sdk.SignedTxn{}, sdk.ApplyData{}, fmt.Errorf("GenesisID <%s> not empty", st.Txn.GenesisID)
	}

	if stb.HasGenesisID {
		st.Txn.GenesisID = bh.GenesisID
	}

	if st.Txn.GenesisHash != (sdk.Digest{}) {
		return sdk.SignedTxn{}, sdk.ApplyData{}, fmt.Errorf("GenesisHash <%v> not empty", st.Txn.GenesisHash)
	}

	if proto.RequireGenesisHash {
		if stb.HasGenesisHash {
			return sdk.SignedTxn{}, sdk.ApplyData{}, fmt.Errorf("HasGenesisHash set to true but RequireGenesisHash obviates the flag")
		}
		st.Txn.GenesisHash = bh.GenesisHash
	} else {
		if stb.HasGenesisHash {
			st.Txn.GenesisHash = bh.GenesisHash
		}
	}

	return st, ad, nil
}

// EncodeSignedTxn converts a SignedTxn and ApplyData into a SignedTxnInBlock
// for that block.
func EncodeSignedTxn(bh sdk.BlockHeader, st sdk.SignedTxn, ad sdk.ApplyData) (sdk.SignedTxnInBlock, error) {
	var stb sdk.SignedTxnInBlock

	proto, ok := config.Consensus[protocol.ConsensusVersion(bh.CurrentProtocol)]
	if !ok {
		return sdk.SignedTxnInBlock{},
			fmt.Errorf("consensus protocol %s not found", bh.CurrentProtocol)
	}
	if !proto.SupportSignedTxnInBlock {
		stb.SignedTxn = st
		return stb, nil
	}

	if st.Txn.GenesisID != "" {
		if st.Txn.GenesisID == bh.GenesisID {
			st.Txn.GenesisID = ""
			stb.HasGenesisID = true
		} else {
			return sdk.SignedTxnInBlock{}, fmt.Errorf("GenesisID mismatch: %s != %s", st.Txn.GenesisID, bh.GenesisID)
		}
	}

	if (st.Txn.GenesisHash != sdk.Digest{}) {
		if st.Txn.GenesisHash == bh.GenesisHash {
			st.Txn.GenesisHash = sdk.Digest{}
			if !proto.RequireGenesisHash {
				stb.HasGenesisHash = true
			}
		} else {
			return sdk.SignedTxnInBlock{}, fmt.Errorf("GenesisHash mismatch: %v != %v", st.Txn.GenesisHash, bh.GenesisHash)
		}
	} else {
		if proto.RequireGenesisHash {
			return sdk.SignedTxnInBlock{}, fmt.Errorf("GenesisHash required but missing")
		}
	}

	stb.SignedTxn = st
	stb.ApplyData = ad
	return stb, nil
}

func init() {
	oneLineJSONCodecHandle = new(codec.JsonHandle)
	oneLineJSONCodecHandle.ErrorIfNoField = true
	oneLineJSONCodecHandle.ErrorIfNoArrayExpand = true
	oneLineJSONCodecHandle.Canonical = true
	oneLineJSONCodecHandle.RecursiveEmptyCheck = true
	oneLineJSONCodecHandle.HTMLCharsAsIs = true
	oneLineJSONCodecHandle.Indent = 0
	oneLineJSONCodecHandle.MapKeyAsString = true
}
