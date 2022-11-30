package importer

import (
	"archive/tar"
	"compress/bzip2"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/algorand/indexer/conduit/plugins/processors/blockprocessor"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/util"

	"github.com/algorand/go-algorand-sdk/client/v2/algod"
	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/protocol"
	"github.com/algorand/go-algorand/rpcs"
)

// NewImportHelper builds an ImportHelper
func NewImportHelper(genesisJSONPath string, blockFileLimite int, l *log.Logger) *ImportHelper {
	return &ImportHelper{
		GenesisJSONPath: genesisJSONPath,
		BlockFileLimit:  blockFileLimite,
		Log:             l,
	}
}

// ImportHelper glues together a directory full of block files and an Importer objects.
type ImportHelper struct {
	// GenesisJSONPath is the location of the genesis file
	GenesisJSONPath string

	// BlockFileLimit is the number of block files to process.
	BlockFileLimit int

	Log *log.Logger
}

// Import is the main ImportHelper function that glues together a directory full of block files and an Importer objects.
func (h *ImportHelper) Import(db idb.IndexerDb, args []string) {
	// Initial import if needed.
	genesisReader := GetGenesisFile(h.GenesisJSONPath, nil, h.Log)
	genesis, err := util.ReadGenesis(genesisReader)
	maybeFail(err, h.Log, "readGenesis() error")

	_, err = EnsureInitialImport(db, genesis)
	maybeFail(err, h.Log, "EnsureInitialImport() error")

	imp := NewImporter(db)
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
				fb, ft := importFile(gfname, imp, h.Log, h.GenesisJSONPath)
				blocks += fb
				txCount += ft
			}
		} else {
			// try without passing throug glob
			fb, ft := importFile(fname, imp, h.Log, h.GenesisJSONPath)
			blocks += fb
			txCount += ft
		}
	}
	blockdone := time.Now()
	if blocks > 0 {
		dt := blockdone.Sub(start)
		h.Log.Infof("%d blocks in %s, %.0f/s, %d txn, %.0f/s", blocks, dt.String(), float64(time.Second)*float64(blocks)/float64(dt), txCount, float64(time.Second)*float64(txCount)/float64(dt))
	}
}

func maybeFail(err error, l *log.Logger, errfmt string, params ...interface{}) {
	if err == nil {
		return
	}
	l.WithError(err).Errorf(errfmt, params...)
	os.Exit(1)
}

func importTar(imp Importer, tarfile io.Reader, logger *log.Logger, genesisReader io.Reader) (blockCount, txCount int, err error) {
	tf := tar.NewReader(tarfile)
	var header *tar.Header
	header, err = tf.Next()
	txCount = 0
	blocks := make([]rpcs.EncodedBlockCert, 0)
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
		var blockContainer rpcs.EncodedBlockCert
		err = protocol.Decode(blockbytes, &blockContainer)
		if err != nil {
			err = fmt.Errorf("error decoding blockbytes, %w", err)
			return
		}
		txCount += len(blockContainer.Block.Payset)
		blocks = append(blocks, blockContainer)
		header, err = tf.Next()
	}
	if err == io.EOF {
		err = nil
	}

	less := func(i int, j int) bool {
		return blocks[i].Block.Round() < blocks[j].Block.Round()
	}
	sort.Slice(blocks, less)

	var genesis bookkeeping.Genesis
	gbytes, err := ioutil.ReadAll(genesisReader)
	if err != nil {
		maybeFail(err, logger, "error reading genesis, %v", err)
	}
	err = protocol.DecodeJSON(gbytes, &genesis)
	if err != nil {
		maybeFail(err, logger, "error decoding genesis, %v", err)
	}

	dir, err := os.MkdirTemp(os.TempDir(), "indexer_import_tempdir")
	maybeFail(err, logger, "Failed to make temp dir")

	ld, err := util.MakeLedger(logger, false, &genesis, dir)
	maybeFail(err, logger, "Cannot open ledger")

	proc, err := blockprocessor.MakeBlockProcessorWithLedger(logger, ld, imp.ImportBlock)
	maybeFail(err, logger, "Error creating processor")

	f := blockprocessor.MakeBlockProcessorHandlerAdapter(&proc, imp.ImportBlock)

	for _, blockContainer := range blocks[1:] {
		err = f(&blockContainer)
		if err != nil {
			return
		}
	}

	return
}

func importFile(fname string, imp Importer, l *log.Logger, genesisPath string) (blocks, txCount int) {
	blocks = 0
	txCount = 0
	l.Infof("importing %s ...", fname)
	genesisReader := GetGenesisFile(genesisPath, nil, l)
	if strings.HasSuffix(fname, ".tar") {
		fin, err := os.Open(fname)
		maybeFail(err, l, "%s: %v", fname, err)
		defer fin.Close()
		tblocks, btxns, err := importTar(imp, fin, l, genesisReader)
		maybeFail(err, l, "%s: %v", fname, err)
		blocks += tblocks
		txCount += btxns
	} else if strings.HasSuffix(fname, ".tar.bz2") {
		fin, err := os.Open(fname)
		maybeFail(err, l, "%s: %v", fname, err)
		defer fin.Close()
		bzin := bzip2.NewReader(fin)
		tblocks, btxns, err := importTar(imp, bzin, l, genesisReader)
		maybeFail(err, l, "%s: %v", fname, err)
		blocks += tblocks
		txCount += btxns
	} else {
		//assume a standalone block msgpack blob
		maybeFail(errors.New("cannot import a standalone block"), l, "not supported")
	}
	return
}

// EnsureInitialImport imports the genesis block if needed. Returns true if the initial import occurred.
func EnsureInitialImport(db idb.IndexerDb, genesis bookkeeping.Genesis) (bool, error) {
	_, err := db.GetNextRoundToAccount()
	// Exit immediately or crash if we don't see ErrorNotInitialized.
	if err != idb.ErrorNotInitialized {
		if err != nil {
			return false, fmt.Errorf("getting import state, %v", err)
		}
		err = checkGenesisHash(db, genesis)
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

// GetGenesisFile creates a reader from the given genesis file
func GetGenesisFile(genesisJSONPath string, client *algod.Client, l *log.Logger) io.Reader {
	var genesisReader io.Reader
	var err error
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

	return genesisReader
}

func checkGenesisHash(db idb.IndexerDb, genesis bookkeeping.Genesis) error {
	network, err := db.GetNetworkState()
	if errors.Is(err, idb.ErrorNotInitialized) {
		err = db.SetNetworkState(genesis)
		if err != nil {
			return fmt.Errorf("error setting network state %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("unable to fetch network state from db %w", err)
	}
	if network.GenesisHash != crypto.HashObj(genesis) {
		return fmt.Errorf("genesis hash not matching")
	}
	return nil
}
