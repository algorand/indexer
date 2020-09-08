package importer

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/types"

	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	"github.com/algorand/indexer/util"
)

type Importer interface {
	ImportBlock(blockbytes []byte) (txCount int, err error)
	ImportDecodedBlock(block *types.EncodedBlockCert) (txCount int, err error)
}

type dbImporter struct {
	db idb.IndexerDb
}

var TypeEnumMap = map[string]int{
	"pay":    idb.TypeEnumPay,
	"keyreg": idb.TypeEnumKeyreg,
	"acfg":   idb.TypeEnumAssetConfig,
	"axfer":  idb.TypeEnumAssetTransfer,
	"afrz":   idb.TypeEnumAssetFreeze,
	"appl":   idb.TypeEnumApplication,
}

var TypeEnumString string

func init() {
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

func (imp *dbImporter) ImportBlock(blockbytes []byte) (txCount int, err error) {
	var blockContainer types.EncodedBlockCert
	err = msgpack.Decode(blockbytes, &blockContainer)
	if err != nil {
		return txCount, fmt.Errorf("error decoding blockbytes, %v", err)
	}
	return imp.ImportDecodedBlock(&blockContainer)
}

func (imp *dbImporter) ImportDecodedBlock(blockContainer *types.EncodedBlockCert) (txCount int, err error) {
	txCount = 0
	proto, err := types.Protocol(string(blockContainer.Block.CurrentProtocol))
	if err != nil {
		return txCount, fmt.Errorf("block %d, %v", blockContainer.Block.Round, err)
	}
	err = imp.db.StartBlock()
	if err != nil {
		return txCount, fmt.Errorf("error starting block, %v", err)
	}
	block := blockContainer.Block
	round := uint64(block.Round)
	for intra := range block.Payset {
		stxn := &block.Payset[intra]
		txtype := string(stxn.Txn.Type)
		txtypeenum, ok := TypeEnumMap[txtype]
		if !ok {
			return txCount, fmt.Errorf("%d:%d unknown txn type %v", round, intra, txtype)
		}
		assetid := uint64(0)
		switch txtypeenum {
		case 3:
			assetid = uint64(stxn.Txn.ConfigAsset)
			if assetid == 0 {
				assetid = block.TxnCounter - uint64(len(block.Payset)) + uint64(intra) + 1
			}
		case 4:
			assetid = uint64(stxn.Txn.XferAsset)
		case 5:
			assetid = uint64(stxn.Txn.FreezeAsset)
		case 6:
			assetid = uint64(stxn.Txn.ApplicationID)
			if assetid == 0 {
				assetid = block.TxnCounter - uint64(len(block.Payset)) + uint64(intra) + 1
			}
		}
		if stxn.HasGenesisID {
			stxn.Txn.GenesisID = block.GenesisID
		}
		if stxn.HasGenesisHash || proto.RequireGenesisHash {
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
			return txCount, fmt.Errorf("error importing txn r=%d i=%d, %v", round, intra, err)
		}
		txCount++
	}
	blockHeader := block
	blockHeader.Payset = nil
	blockheaderBytes := msgpack.Encode(blockHeader)
	err = imp.db.CommitBlock(round, block.TimeStamp, block.RewardsLevel, blockheaderBytes)
	if err != nil {
		return txCount, fmt.Errorf("error committing block, %v", err)
	}
	return
}

func NewDBImporter(db idb.IndexerDb) Importer {
	return &dbImporter{db: db}
}

// ImportProto writes compiled-in protocol information to the database
func ImportProto(db idb.IndexerDb) (err error) {
	types.ForeachProtocol(func(version string, proto types.ConsensusParams) (err error) {
		err = db.SetProto(version, proto)
		if err != nil {
			return fmt.Errorf("db set proto %s, %v", version, err)
		}
		return nil
	})
	return nil
}
