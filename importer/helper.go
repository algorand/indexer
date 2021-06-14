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

	"github.com/algorand/go-algorand-sdk/client/v2/algod"
	"github.com/algorand/go-algorand-sdk/encoding/json"
	log "github.com/sirupsen/logrus"

	"github.com/algorand/indexer/accounting"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/types"
)

// NewImportHelper builds an ImportHelper
func NewImportHelper(defaultFrozenCache map[uint64]bool, genesisJSONPath string, numRoundsLimit, blockFileLimite int, l *log.Logger) *ImportHelper {
	return &ImportHelper{
		DefaultFrozenCache: defaultFrozenCache,
		GenesisJSONPath:    genesisJSONPath,
		NumRoundsLimit:     numRoundsLimit,
		BlockFileLimit:     blockFileLimite,
		Log:                l,
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

	// DefaultFrozenCache is a persistent cache of default frozen values.
	DefaultFrozenCache map[uint64]bool

	Log *log.Logger
}

// Import is the main ImportHelper function that glues together a directory full of block files and an Importer objects.
func (h *ImportHelper) Import(db idb.IndexerDb, args []string) {
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

	startRound, err := db.GetNextRoundToAccount()
	if err == idb.ErrorNotInitialized {
		InitialImport(db, h.GenesisJSONPath, nil, h.Log)
		startRound = 0
	} else {
		maybeFail(err, h.Log, "problem getting the import state")
	}

	filter := idb.UpdateFilter{
		StartRound: startRound,
	}
	if h.NumRoundsLimit != 0 {
		filter.RoundLimit = &h.NumRoundsLimit
	}
	accountingRounds, txnCount := updateAccounting(db, h.DefaultFrozenCache, filter, h.Log)

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
	l.WithError(err).Errorf(errfmt, params...)
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

// InitialImport imports the genesis block if needed. Returns true if the initial import occurred.
func InitialImport(db idb.IndexerDb, genesisJSONPath string, client *algod.Client, l *log.Logger) bool {
	_, err := db.GetNextRoundToAccount()

	// Exit immediately or crash if we don't see ErrorNotInitialized.
	if err != idb.ErrorNotInitialized {
		maybeFail(err, l, "getting import state, %v", err)
		return false
	}

	// Import genesis file from file or algod.
	var genesisReader io.Reader

	if genesisJSONPath != "" {
		// Read file if specified.
		l.Infof("loading genesis file %s", genesisJSONPath)
		genesisReader, err = os.Open(genesisJSONPath)
		maybeFail(err, l, "unable to read genesis file %s", genesisJSONPath)
	} else if client != nil {
		// Fallback to asking algod for genesis if file is not specified.
		l.Infof("fetching genesis from algod")
		genesisString, err := client.GetGenesis().Do(context.Background())
		maybeFail(err, l, "unable to fetch genesis from algod")
		genesisReader = strings.NewReader(genesisString)
	} else {
		l.Fatal("Neither genesis file path or algod client provided for initial import.")
	}

	err = loadGenesis(db, genesisReader)
	maybeFail(err, l, "%s: could not load genesis json, %v", genesisJSONPath, err)
	return true
}

// UpdateAccounting triggers an accounting update.
func UpdateAccounting(db idb.IndexerDb, frozenCache map[uint64]bool, filter idb.UpdateFilter, l *log.Logger) (rounds, txnCount int) {
	return updateAccounting(db, frozenCache, filter, l)
}

func updateAccounting(db idb.IndexerDb, frozenCache map[uint64]bool, filter idb.UpdateFilter, l *log.Logger) (rounds, txnCount int) {
	rounds = 0
	txnCount = 0
	lastlog := time.Now()
	act := accounting.New(frozenCache)
	txns := db.YieldTxns(context.Background(), filter.StartRound)
	currentRound := uint64(0)
	roundsSeen := 0
	lastRoundsSeen := roundsSeen
	txnForRound := 0
	var blockHeaderPtr *types.BlockHeader = nil
	for txn := range txns {
		maybeFail(txn.Error, l, "updateAccounting txn fetch, %v", txn.Error)
		if txn.Round != currentRound {
			if blockHeaderPtr != nil && txnForRound > 0 {
				err := db.CommitRoundAccounting(act.RoundUpdates, currentRound, blockHeaderPtr)
				maybeFail(err, l, "failed to commit round accounting")
			}

			// initialize accounting for next round
			txnForRound = 0
			prevRound := currentRound
			roundsSeen++
			currentRound = txn.Round

			// Check to see if the max round has been reached after resetting txnForRound.
			if filter.MaxRound != 0 && currentRound > filter.MaxRound {
				break
			}

			blockHeader, _, err := db.GetBlock(context.Background(), currentRound, idb.GetBlockOptions{})
			maybeFail(err, l, "problem fetching next round (%d)", currentRound)
			blockHeaderPtr = &blockHeader
			act.InitRound(blockHeaderPtr)

			// Log progress
			if (filter.RoundLimit != nil) && (roundsSeen > *filter.RoundLimit) {
				l.Infof("hit rounds limit %d > %d", roundsSeen, filter.RoundLimit)
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
		err := act.AddTransaction(&txn)
		maybeFail(err, l, "txn accounting r=%d i=%d, %v", txn.Round, txn.Intra, err)
		txnCount++
		txnForRound++
	}

	// Commit the final round
	if blockHeaderPtr != nil && txnForRound > 0 {
		err := db.CommitRoundAccounting(act.RoundUpdates, currentRound, blockHeaderPtr)
		maybeFail(err, l, "failed to commit round accounting")
	}

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
