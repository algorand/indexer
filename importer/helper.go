package importer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/algorand/go-algorand-sdk/client/v2/algod"
	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/protocol"
	log "github.com/sirupsen/logrus"

	"github.com/algorand/indexer/idb"
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

func maybeFail(err error, l *log.Logger, errfmt string, params ...interface{}) {
	if err == nil {
		return
	}
	l.WithError(err).Errorf(errfmt, params...)
	os.Exit(1)
}

func loadGenesis(db idb.IndexerDb, in io.Reader) (err error) {
	var genesis bookkeeping.Genesis
	gbytes, err := ioutil.ReadAll(in)
	if err != nil {
		return fmt.Errorf("error reading genesis, %v", err)
	}
	err = protocol.DecodeJSON(gbytes, &genesis)
	if err != nil {
		return fmt.Errorf("error decoding genesis, %v", err)
	}

	return db.LoadGenesis(genesis)
}

// EnsureInitialImport imports the genesis block if needed. Returns true if the initial import occurred.
func EnsureInitialImport(db idb.IndexerDb, genesisReader io.Reader, l *log.Logger) (bool, error) {
	_, err := db.GetNextRoundToAccount()
	// Exit immediately or crash if we don't see ErrorNotInitialized.
	if err != idb.ErrorNotInitialized {
		if err != nil {
			return false, fmt.Errorf("getting import state, %v", err)
		}
		err = checkGenesisHash(db, genesisReader)
		if err != nil {
			return false, err
		}
		return false, nil
	}

	// Import genesis file from file or algod.
	err = loadGenesis(db, genesisReader)
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

func checkGenesisHash(db idb.IndexerDb, genesisReader io.Reader) error {
	var genesis bookkeeping.Genesis
	gbytes, err := ioutil.ReadAll(genesisReader)
	if err != nil {
		return fmt.Errorf("error reading genesis, %w", err)
	}
	err = protocol.DecodeJSON(gbytes, &genesis)
	if err != nil {
		return fmt.Errorf("error decoding genesis, %w", err)
	}
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
