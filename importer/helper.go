package importer

import (
	"archive/tar"
	"compress/bzip2"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/algorand/go-algorand-sdk/encoding/json"
	log "github.com/sirupsen/logrus"

	"github.com/algorand/indexer/accounting"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/types"
)

// NewImportHelper builds an ImportHelper
func NewImportHelper(genesisJSONPath string, numRoundsLimit, blockFileLimite int, l *log.Logger) (*ImportHelper) {
	return &ImportHelper{
		GenesisJSONPath: genesisJSONPath,
		NumRoundsLimit:  numRoundsLimit,
		BlockFileLimit:  blockFileLimite,
		Log:             l,
	}
}

// ImportHelper glues together a directory full of block files and an Importer objects.
type ImportHelper struct {
	// GenesisJSONPath is the location of the genesis file
	GenesisJSONPath string

	// NumRoundsLimit is the number of rounds to process, if 0 import continues forever.
	NumRoundsLimit int

	// BlockFileLimit is the number of block files to process.
	BlockFileLimit int

	Log *log.Logger
}

// ImportState is some metadata kept around to help the import helper.
type ImportState struct {
	AccountRound int64 `codec:"account_round"`
}

// ParseImportState decodes a json serialized import state object.
func ParseImportState(js string) (istate ImportState, err error) {
	err = json.Decode([]byte(js), &istate)
	return
}

// Import is the main ImportHelper function that glues together a directory full of block files and an Importer objects.
func (h *ImportHelper) Import(db idb.IndexerDb, args []string) {
	err := ImportProto(db)
	maybeFail(err, h.Log, "import proto, %v", err)

	imp := NewDBImporter(db)
	blocks := 0
	txCount := 0
	start := time.Now()
	for _, fname := range args {
		matches, err := filepath.Glob(fname)
		if err == nil {
			pathsSorted := blockTarPaths(matches)
			sort.Sort(&pathsSorted)
			if h.BlockFileLimit != 0 && len(pathsSorted) > h.BlockFileLimit {
				pathsSorted = pathsSorted[:h.BlockFileLimit]
			}
			for _, gfname := range pathsSorted {
				fb, ft := importFile(db, imp, gfname, h.Log)
				blocks += fb
				txCount += ft
			}
		} else {
			// try without passing throug glob
			fb, ft := importFile(db, imp, fname, h.Log)
			blocks += fb
			txCount += ft
		}
	}
	blockdone := time.Now()
	if blocks > 0 {
		dt := blockdone.Sub(start)
		h.Log.Infof("%d blocks in %s, %.0f/s, %d txn, %.0f/s", blocks, dt.String(), float64(time.Second)*float64(blocks)/float64(dt), txCount, float64(time.Second)*float64(txCount)/float64(dt))
	}

	accountingRounds, txnCount := updateAccounting(db, h.GenesisJSONPath, h.NumRoundsLimit, h.Log)

	accountingdone := time.Now()
	if accountingRounds > 0 {
		dt := accountingdone.Sub(blockdone)
		h.Log.Infof("%d rounds accounting in %s, %.1f/s (%d txns, %.1f/s)", accountingRounds, dt.String(), float64(time.Second)*float64(accountingRounds)/float64(dt), txnCount, float64(time.Second)*float64(txnCount)/float64(dt))
	}

	dt := accountingdone.Sub(start)
	h.Log.Infof(
		"%d blocks loaded (%.1f/s) and %d rounds accounting in %s, %.1f/s (%d txns, %.1f/s)",
		blocks,
		float64(time.Second)*float64(blocks)/float64(dt),
		accountingRounds,
		dt.String(),
		float64(time.Second)*float64(accountingRounds)/float64(dt),
		txnCount,
		float64(time.Second)*float64(txnCount)/float64(dt),
	)
}

func maybeFail(err error, l *log.Logger, errfmt string, params ...interface{}) {
	if err == nil {
		return
	}
	l.Errorf(errfmt, params...)
	os.Exit(1)
}

func importTar(imp Importer, tarfile io.Reader, l *log.Logger) (blocks, txCount int, err error) {
	lastlog := time.Now()
	blocks = 0
	prevBlocks := 0
	tf := tar.NewReader(tarfile)
	var header *tar.Header
	header, err = tf.Next()
	txCount = 0
	var btxns int
	for err == nil {
		if header.Typeflag != tar.TypeReg {
			err = fmt.Errorf("cannot deal with non-regular-file tar entry %#v", header.Name)
			return
		}
		blockbytes := make([]byte, header.Size)
		_, err = io.ReadFull(tf, blockbytes)
		if err != nil {
			err = fmt.Errorf("error reading tar entry %#v: %v", header.Name, err)
			return
		}
		btxns, err = imp.ImportBlock(blockbytes)
		if err != nil {
			err = fmt.Errorf("error importing tar entry %#v: %v", header.Name, err)
			return
		}
		txCount += btxns
		blocks++
		now := time.Now()
		dt := now.Sub(lastlog)
		if dt > (5 * time.Second) {
			dblocks := blocks - prevBlocks
			l.Infof("loaded from tar %v, %.1f/s", header.Name, ((float64(dblocks) * float64(time.Second)) / float64(dt)))
			lastlog = now
			prevBlocks = blocks
		}
		header, err = tf.Next()
	}
	if err == io.EOF {
		err = nil
	}
	return
}

func importFile(db idb.IndexerDb, imp Importer, fname string, l *log.Logger) (blocks, txCount int) {
	blocks = 0
	txCount = 0
	var btxns int
	imported, err := db.AlreadyImported(fname)
	maybeFail(err, l, "%s: %v", fname, err)
	if imported {
		return
	}
	l.Infof("importing %s ...", fname)
	if strings.HasSuffix(fname, ".tar") {
		fin, err := os.Open(fname)
		maybeFail(err, l, "%s: %v", fname, err)
		defer fin.Close()
		tblocks, btxns, err := importTar(imp, fin, l)
		maybeFail(err, l, "%s: %v", fname, err)
		blocks += tblocks
		txCount += btxns
	} else if strings.HasSuffix(fname, ".tar.bz2") {
		fin, err := os.Open(fname)
		maybeFail(err, l, "%s: %v", fname, err)
		defer fin.Close()
		bzin := bzip2.NewReader(fin)
		tblocks, btxns, err := importTar(imp, bzin, l)
		maybeFail(err, l, "%s: %v", fname, err)
		blocks += tblocks
		txCount += btxns
	} else {
		// assume a standalone block msgpack blob
		blockbytes, err := ioutil.ReadFile(fname)
		maybeFail(err, l, "%s: could not read, %v", fname, err)
		btxns, err = imp.ImportBlock(blockbytes)
		maybeFail(err, l, "%s: could not import, %v", fname, err)
		blocks++
		txCount += btxns
	}
	err = db.MarkImported(fname)
	maybeFail(err, l, "%s: %v", fname, err)
	return
}

func loadGenesis(db idb.IndexerDb, in io.Reader) (err error) {
	var genesis types.Genesis
	gbytes, err := ioutil.ReadAll(in)
	if err != nil {
		return fmt.Errorf("error reading genesis, %v", err)
	}
	err = json.Decode(gbytes, &genesis)
	if err != nil {
		return fmt.Errorf("error decoding genesis, %v", err)
	}

	return db.LoadGenesis(genesis)
}

// UpdateAccounting triggers an accounting update.
func UpdateAccounting(db idb.IndexerDb, genesisJSONPath string, l *log.Logger) (rounds, txnCount int) {
	return updateAccounting(db, genesisJSONPath, 0, l)
}

func updateAccounting(db idb.IndexerDb, genesisJSONPath string, numRoundsLimit int, l *log.Logger) (rounds, txnCount int) {
	rounds = 0
	txnCount = 0
	stateJSONStr, err := db.GetMetastate("state")
	maybeFail(err, l, "getting import state, %v", err)
	var state ImportState
	if stateJSONStr == "" {
		if genesisJSONPath != "" {
			l.Infof("loading genesis %s", genesisJSONPath)
			// if we're given no previous state and we're given a genesis file, import it as initial account state
			gf, err := os.Open(genesisJSONPath)
			maybeFail(err, l, "%s: %v", genesisJSONPath, err)
			err = loadGenesis(db, gf)
			maybeFail(err, l, "%s: could not load genesis json, %v", genesisJSONPath, err)
			rounds++
			state.AccountRound = -1
		} else {
			l.Errorf("no import state recorded; need --genesis genesis.json file to get started")
			os.Exit(1)
			return
		}
	} else {
		state, err = ParseImportState(stateJSONStr)
		maybeFail(err, l, "parsing import state, %v", err)
		l.Infof("will start from round >%d", state.AccountRound)
	}

	lastlog := time.Now()
	act := accounting.New(db)
	txns := db.YieldTxns(context.Background(), state.AccountRound)
	currentRound := uint64(0)
	roundsSeen := 0
	lastRoundsSeen := roundsSeen
	for txn := range txns {
		maybeFail(txn.Error, l, "updateAccounting txn fetch, %v", txn.Error)
		if txn.Round != currentRound {
			prevRound := currentRound
			roundsSeen++
			currentRound = txn.Round
			if (numRoundsLimit != 0) && (roundsSeen > numRoundsLimit) {
				l.Infof("hit rounds limit %d > %d", roundsSeen, numRoundsLimit)
				break
			}
			now := time.Now()
			dt := now.Sub(lastlog)
			if dt > (5 * time.Second) {
				drounds := roundsSeen - lastRoundsSeen
				l.Infof("accounting through %d, %.1f/s", prevRound, ((float64(drounds) * float64(time.Second)) / float64(dt)))
				lastlog = now
				lastRoundsSeen = roundsSeen
			}
		}
		err = act.AddTransaction(&txn)
		maybeFail(err, l, "txn accounting r=%d i=%d, %v", txn.Round, txn.Intra, err)
		txnCount++
	}
	err = act.Close()
	maybeFail(err, l, "accounting close %v", err)
	rounds += roundsSeen
	if rounds > 0 {
		l.Infof("accounting updated through round %d", currentRound)
	}
	return
}

type blockTarPaths []string

// Len is part of sort.Interface
func (paths *blockTarPaths) Len() int {
	return len(*paths)
}

func pathNameStartInt(x string) int64 {
	x = filepath.Base(x)
	underscorePos := strings.IndexRune(x, '_')
	if underscorePos == -1 {
		// try converting the whole string, might be a plain block
		v, err := strconv.ParseInt(x, 10, 64)
		if err == nil {
			return v
		}
		return -1
	}
	v, err := strconv.ParseInt(x[:underscorePos], 10, 64)
	if err != nil {
		return -1
	}
	return v
}

// Less is part of sort.Interface
func (paths *blockTarPaths) Less(i, j int) bool {
	return pathNameStartInt((*paths)[i]) < pathNameStartInt((*paths)[j])
}

// Swap is part of sort.Interface
func (paths *blockTarPaths) Swap(i, j int) {
	t := (*paths)[i]
	(*paths)[i] = (*paths)[j]
	(*paths)[j] = t
}
