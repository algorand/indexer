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

	"github.com/algorand/indexer/accounting"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/types"
)

type ImportHelper struct {
	GenesisJsonPath string

	NumRoundsLimit int

	BlockFileLimit int
}

type ImportState struct {
	AccountRound int64 `codec:"account_round"`
}

func ParseImportState(js string) (istate ImportState, err error) {
	err = json.Decode([]byte(js), &istate)
	return
}

func (h *ImportHelper) Import(db idb.IndexerDb, args []string) {
	err := ImportProto(db)
	maybeFail(err, "import proto, %v", err)

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
				fb, ft := importFile(db, imp, gfname)
				blocks += fb
				txCount += ft
			}
		} else {
			// try without passing throug glob
			fb, ft := importFile(db, imp, fname)
			blocks += fb
			txCount += ft
		}
	}
	blockdone := time.Now()
	if blocks > 0 {
		dt := blockdone.Sub(start)
		fmt.Printf("%d blocks in %s, %.0f/s, %d txn, %.0f/s\n", blocks, dt.String(), float64(time.Second)*float64(blocks)/float64(dt), txCount, float64(time.Second)*float64(txCount)/float64(dt))
	}

	accountingRounds, txnCount := updateAccounting(db, h.GenesisJsonPath, h.NumRoundsLimit)

	accountingdone := time.Now()
	if accountingRounds > 0 {
		dt := accountingdone.Sub(blockdone)
		fmt.Printf("%d rounds accounting in %s, %.1f/s (%d txns, %.1f/s)\n", accountingRounds, dt.String(), float64(time.Second)*float64(accountingRounds)/float64(dt), txnCount, float64(time.Second)*float64(txnCount)/float64(dt))
	}

	dt := accountingdone.Sub(start)
	fmt.Printf(
		"%d blocks loaded (%.1f/s) and %d rounds accounting in %s, %.1f/s (%d txns, %.1f/s)\n",
		blocks,
		float64(time.Second)*float64(blocks)/float64(dt),
		accountingRounds,
		dt.String(),
		float64(time.Second)*float64(accountingRounds)/float64(dt),
		txnCount,
		float64(time.Second)*float64(txnCount)/float64(dt),
	)
}

func maybeFail(err error, errfmt string, params ...interface{}) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, errfmt, params...)
	os.Exit(1)
}

func importTar(imp Importer, tarfile io.Reader) (blocks, txCount int, err error) {
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
			fmt.Printf("loaded from tar %v, %.1f/s\n", header.Name, ((float64(dblocks) * float64(time.Second)) / float64(dt)))
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

func importFile(db idb.IndexerDb, imp Importer, fname string) (blocks, txCount int) {
	blocks = 0
	txCount = 0
	var btxns int
	imported, err := db.AlreadyImported(fname)
	maybeFail(err, "%s: %v\n", fname, err)
	if imported {
		return
	}
	fmt.Printf("importing %s ...\n", fname)
	if strings.HasSuffix(fname, ".tar") {
		fin, err := os.Open(fname)
		maybeFail(err, "%s: %v\n", fname, err)
		defer fin.Close()
		tblocks, btxns, err := importTar(imp, fin)
		maybeFail(err, "%s: %v\n", fname, err)
		blocks += tblocks
		txCount += btxns
	} else if strings.HasSuffix(fname, ".tar.bz2") {
		fin, err := os.Open(fname)
		maybeFail(err, "%s: %v\n", fname, err)
		defer fin.Close()
		bzin := bzip2.NewReader(fin)
		tblocks, btxns, err := importTar(imp, bzin)
		maybeFail(err, "%s: %v\n", fname, err)
		blocks += tblocks
		txCount += btxns
	} else {
		// assume a standalone block msgpack blob
		blockbytes, err := ioutil.ReadFile(fname)
		maybeFail(err, "%s: could not read, %v\n", fname, err)
		btxns, err = imp.ImportBlock(blockbytes)
		maybeFail(err, "%s: could not import, %v\n", fname, err)
		blocks++
		txCount += btxns
	}
	err = db.MarkImported(fname)
	maybeFail(err, "%s: %v\n", fname, err)
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

func UpdateAccounting(db idb.IndexerDb, genesisJsonPath string) (rounds, txnCount int) {
	return updateAccounting(db, genesisJsonPath, 0)
}

func updateAccounting(db idb.IndexerDb, genesisJsonPath string, numRoundsLimit int) (rounds, txnCount int) {
	rounds = 0
	txnCount = 0
	stateJsonStr, err := db.GetMetastate("state")
	maybeFail(err, "getting import state, %v\n", err)
	var state ImportState
	if stateJsonStr == "" {
		if genesisJsonPath != "" {
			fmt.Printf("loading genesis %s\n", genesisJsonPath)
			// if we're given no previous state and we're given a genesis file, import it as initial account state
			gf, err := os.Open(genesisJsonPath)
			maybeFail(err, "%s: %v\n", genesisJsonPath, err)
			err = loadGenesis(db, gf)
			maybeFail(err, "%s: could not load genesis json, %v\n", genesisJsonPath, err)
			rounds++
			state.AccountRound = -1
		} else {
			fmt.Fprintf(os.Stderr, "no import state recorded; need --genesis genesis.json file to get started\n")
			os.Exit(1)
			return
		}
	} else {
		state, err = ParseImportState(stateJsonStr)
		maybeFail(err, "parsing import state, %v\n", err)
		fmt.Printf("will start from round >%d\n", state.AccountRound)
	}

	lastlog := time.Now()
	act := accounting.New(db)
	txns := db.YieldTxns(context.Background(), state.AccountRound)
	currentRound := uint64(0)
	roundsSeen := 0
	lastRoundsSeen := roundsSeen
	for txn := range txns {
		maybeFail(txn.Error, "updateAccounting txn fetch, %v", txn.Error)
		if txn.Round != currentRound {
			prevRound := currentRound
			roundsSeen++
			currentRound = txn.Round
			if (numRoundsLimit != 0) && (roundsSeen > numRoundsLimit) {
				fmt.Printf("hit rounds limit %d > %d\n", roundsSeen, numRoundsLimit)
				break
			}
			now := time.Now()
			dt := now.Sub(lastlog)
			if dt > (5 * time.Second) {
				drounds := roundsSeen - lastRoundsSeen
				fmt.Printf("accounting through %d, %.1f/s\n", prevRound, ((float64(drounds) * float64(time.Second)) / float64(dt)))
				lastlog = now
				lastRoundsSeen = roundsSeen
			}
		}
		err = act.AddTransaction(&txn)
		maybeFail(err, "txn accounting r=%d i=%d, %v\n", txn.Round, txn.Intra, err)
		txnCount++
	}
	err = act.Close()
	maybeFail(err, "accounting close %v\n", err)
	rounds += roundsSeen
	if rounds > 0 {
		fmt.Printf("accounting updated through round %d\n", currentRound)
	}
	return
}

type blockTarPaths []string

// sort.Interface
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

// sort.Interface
func (paths *blockTarPaths) Less(i, j int) bool {
	return pathNameStartInt((*paths)[i]) < pathNameStartInt((*paths)[j])
}

// sort.Interface
func (paths *blockTarPaths) Swap(i, j int) {
	t := (*paths)[i]
	(*paths)[i] = (*paths)[j]
	(*paths)[j] = t
}
