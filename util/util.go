package util

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/algorand/go-codec/codec"
	"github.com/algorand/indexer/v3/idb"

	"github.com/algorand/go-algorand-sdk/v2/encoding/json"
	"github.com/algorand/go-algorand-sdk/v2/protocol"
	"github.com/algorand/go-algorand-sdk/v2/protocol/config"
	sdk "github.com/algorand/go-algorand-sdk/v2/types"
)

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

// IsDir returns true if the specified directory is valid
func IsDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// FileExists checks to see if the specified file (or directory) exists
func FileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	fileExists := err == nil
	return fileExists
}

// GetConfigFromDataDir Given the data directory, configuration filename and a list of types, see if
// a configuration file that matches was located there.  If no configuration file was there then an
// empty string is returned.  If more than one filetype was matched, an error is returned.
func GetConfigFromDataDir(dataDirectory string, configFilename string, configFileTypes []string) (string, error) {
	count := 0
	fullPath := ""
	var err error

	for _, configFileType := range configFileTypes {
		autoloadParamConfigPath := filepath.Join(dataDirectory, configFilename+"."+configFileType)
		if FileExists(autoloadParamConfigPath) {
			count++
			fullPath = autoloadParamConfigPath
		}
	}

	if count > 1 {
		return "", fmt.Errorf("config filename (%s) in data directory (%s) matched more than one filetype: %v",
			configFilename, dataDirectory, configFileTypes)
	}

	// if count == 0 then the fullpath will be set to "" and error will be nil
	// if count == 1 then it fullpath will be correct
	return fullPath, err
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

// EnsureInitialImport imports the genesis block if needed. Returns true if the initial import occurred.
func EnsureInitialImport(db idb.IndexerDb, genesis sdk.Genesis) (bool, error) {
	_, err := db.GetNextRoundToAccount()
	// Exit immediately or crash if we don't see ErrorNotInitialized.
	if err != idb.ErrorNotInitialized {
		if err != nil {
			return false, fmt.Errorf("getting import state, %v", err)
		}
		err = checkGenesisHash(db, genesis.Hash())
		if err != nil {
			return false, err
		}
		return false, nil
	}

	// Import genesis file from file or algod.
	err = db.LoadGenesis(genesis)
	if err != nil {
		return false, fmt.Errorf("could not load genesis json, %v", err)
	}
	return true, nil
}

func checkGenesisHash(db idb.IndexerDb, gh sdk.Digest) error {
	network, err := db.GetNetworkState()
	if errors.Is(err, idb.ErrorNotInitialized) {
		err = db.SetNetworkState(gh)
		if err != nil {
			return fmt.Errorf("error setting network state %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("unable to fetch network state from db %w", err)
	}
	if network.GenesisHash != gh {
		return fmt.Errorf("genesis hash not matching")
	}
	return nil
}

// ReadGenesis converts a reader into a Genesis file.
func ReadGenesis(in io.Reader) (sdk.Genesis, error) {
	var genesis sdk.Genesis
	if in == nil {
		return sdk.Genesis{}, fmt.Errorf("ReadGenesis() err: reader is nil")
	}
	gbytes, err := ioutil.ReadAll(in)
	if err != nil {
		return sdk.Genesis{}, fmt.Errorf("ReadGenesis() err: %w", err)
	}
	err = json.Decode(gbytes, &genesis)
	if err != nil {
		return sdk.Genesis{}, fmt.Errorf("ReadGenesis() decode err: %w", err)
	}
	return genesis, nil
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
