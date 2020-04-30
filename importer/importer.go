package importer

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/types"

	"github.com/algorand/go-algorand-sdk/encoding/json"
	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	"github.com/algorand/indexer/util"
)

// protocols.json from code run in go-algorand:
// func main() { os.Stdout.Write(protocol.EncodeJSON(config.Consensus)) }
//go:generate go run ../cmd/texttosource/main.go importer protocols.json

type Importer interface {
	ImportBlock(blockbytes []byte) error
	ImportDecodedBlock(block *types.EncodedBlockCert) error
}

type dbImporter struct {
	db idb.IndexerDb
}

var typeEnumList = []util.StringInt{
	{"pay", 1},
	{"keyreg", 2},
	{"acfg", 3},
	{"axfer", 4},
	{"afrz", 5},
}
var TypeEnumMap map[string]int
var TypeEnumString string

func init() {
	TypeEnumMap = util.EnumListToMap(typeEnumList)
	TypeEnumString = util.KeysStringInt(TypeEnumMap)
}

func keysString(m map[string]bool) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return "[" + strings.Join(keys, ", ") + "]"
}

var zeroAddr = [32]byte{}

func participate(participants [][]byte, addr []byte) [][]byte {
	if len(addr) == 0 || bytes.Equal(addr, zeroAddr[:]) {
		return participants
	}
	for _, pp := range participants {
		if bytes.Equal(pp, addr) {
			return participants
		}
	}
	return append(participants, addr)
}

func (imp *dbImporter) ImportBlock(blockbytes []byte) (err error) {
	var blockContainer types.EncodedBlockCert
	err = msgpack.Decode(blockbytes, &blockContainer)
	if err != nil {
		return fmt.Errorf("error decoding blockbytes, %v", err)
	}
	return imp.ImportDecodedBlock(&blockContainer)
}
func (imp *dbImporter) ImportDecodedBlock(blockContainer *types.EncodedBlockCert) (err error) {
	ensureProtos()
	_, okversion := protocols[string(blockContainer.Block.CurrentProtocol)]
	if !okversion {
		return fmt.Errorf("block %d unknown protocol version %#v", blockContainer.Block.Round, string(blockContainer.Block.CurrentProtocol))
	}
	err = imp.db.StartBlock()
	if err != nil {
		return fmt.Errorf("error starting block, %v", err)
	}
	block := blockContainer.Block
	round := uint64(block.Round)
	for intra, stxn := range block.Payset {
		txtype := string(stxn.Txn.Type)
		txtypeenum := TypeEnumMap[txtype]
		assetid := uint64(0)
		switch txtypeenum {
		case 3:
			assetid = uint64(stxn.Txn.ConfigAsset)
		case 4:
			assetid = uint64(stxn.Txn.XferAsset)
		case 5:
			assetid = uint64(stxn.Txn.FreezeAsset)
		}
		if stxn.HasGenesisID {
			stxn.Txn.GenesisID = block.GenesisID
		}
		if stxn.HasGenesisHash {
			stxn.Txn.GenesisHash = block.GenesisHash
		}
		stxnad := stxn.SignedTxnWithAD
		participants := make([][]byte, 0, 10)
		participants = participate(participants, stxn.Txn.Sender[:])
		participants = participate(participants, stxn.Txn.Receiver[:])
		participants = participate(participants, stxn.Txn.CloseRemainderTo[:])
		participants = participate(participants, stxn.Txn.AssetSender[:])
		participants = participate(participants, stxn.Txn.AssetReceiver[:])
		participants = participate(participants, stxn.Txn.AssetCloseTo[:])
		err = imp.db.AddTransaction(round, intra, txtypeenum, assetid, stxnad, participants)
		if err != nil {
			return fmt.Errorf("error importing txn r=%d i=%d, %v", round, intra, err)
		}
	}
	blockHeader := block
	blockHeader.Payset = nil
	blockheaderBytes := msgpack.Encode(blockHeader)
	err = imp.db.CommitBlock(round, block.TimeStamp, block.RewardsLevel, blockheaderBytes)
	if err != nil {
		return fmt.Errorf("error committing block, %v", err)
	}
	return nil
}

func NewDBImporter(db idb.IndexerDb) Importer {
	return &dbImporter{db: db}
}

var protocols map[string]types.ConsensusParams

func ensureProtos() (err error) {
	if protocols != nil {
		return nil
	}
	protos := make(map[string]types.ConsensusParams, 30)
	// Load text from protocols.json as compiled-in.
	err = json.Decode([]byte(protocols_json), &protos)
	if err != nil {
		return fmt.Errorf("proto decode, %v", err)
	}
	protocols = protos
	return nil
}

// ImportProto writes compiled-in protocol information to the database
func ImportProto(db idb.IndexerDb) (err error) {
	err = ensureProtos()
	if err != nil {
		return
	}
	for version, proto := range protocols {
		err = db.SetProto(version, proto)
		if err != nil {
			return fmt.Errorf("db set proto %s, %v", version, err)
		}
	}
	return nil
}
