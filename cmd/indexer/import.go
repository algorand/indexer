// Copyright (C) 2019-2020 Algorand, Inc.
// This file is part of the Algorand Indexer
//
// Algorand Indexer is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// Algorand Indexer is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with Algorand Indexer.  If not, see <https://www.gnu.org/licenses/>.

package main

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
	"github.com/spf13/cobra"

	"github.com/algorand/indexer/accounting"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/importer"
	"github.com/algorand/indexer/types"
)

func maybeFail(err error, errfmt string, params ...interface{}) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, errfmt, params...)
	os.Exit(1)
}

func importTar(imp importer.Importer, tarfile io.Reader) (blocks int, err error) {
	blocks = 0
	tf := tar.NewReader(tarfile)
	var header *tar.Header
	header, err = tf.Next()
	for err == nil {
		if header.Typeflag != tar.TypeReg {
			return blocks, fmt.Errorf("cannot deal with non-regular-file tar entry %#v", header.Name)
		}
		/*
			round, err := strconv.Atoi(header.Name)
			if err != nil {
				return fmt.Errorf("could not parse round number in tar archive, file %#v", header.Name)
			}
		*/
		blockbytes := make([]byte, header.Size)
		_, err = io.ReadFull(tf, blockbytes)
		if err != nil {
			return blocks, fmt.Errorf("error reading tar entry %#v: %v", header.Name, err)
		}
		err = imp.ImportBlock(blockbytes)
		if err != nil {
			return blocks, fmt.Errorf("error importing tar entry %#v: %v", header.Name, err)
		}
		blocks++
		header, err = tf.Next()
	}
	if err == io.EOF {
		err = nil
	}
	return
}

func importFile(db idb.IndexerDb, imp importer.Importer, fname string) (blocks int) {
	blocks = 0
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
		tblocks, err := importTar(imp, fin)
		maybeFail(err, "%s: %v\n", fname, err)
		blocks += tblocks
	} else if strings.HasSuffix(fname, ".tar.bz2") {
		fin, err := os.Open(fname)
		maybeFail(err, "%s: %v\n", fname, err)
		defer fin.Close()
		bzin := bzip2.NewReader(fin)
		tblocks, err := importTar(imp, bzin)
		maybeFail(err, "%s: %v\n", fname, err)
		blocks += tblocks
	} else {
		// assume a standalone block msgpack blob
		blockbytes, err := ioutil.ReadFile(fname)
		maybeFail(err, "%s: could not read, %v\n", fname, err)
		err = imp.ImportBlock(blockbytes)
		maybeFail(err, "%s: could not import, %v\n", fname, err)
		blocks++
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

/*
type ImportState struct {
	// AccountRound is the last round committed into account state.
	// -1 for after genesis is committed and we need to load round 0
	AccountRound int64
}*/

func updateAccounting(db idb.IndexerDb) (rounds int) {
	rounds = 0
	stateJsonStr, err := db.GetMetastate("state")
	maybeFail(err, "getting import state, %v\n", err)
	var state idb.ImportState
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
		state, err = idb.ParseImportState(stateJsonStr)
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
		err = act.AddTransaction(txn.Round, txn.Intra, txn.TxnBytes)
		maybeFail(err, "txn accounting r=%d i=%d, %v\n", txn.Round, txn.Intra, err)
	}
	err = act.Close()
	maybeFail(err, "accounting close %v\n", err)
	rounds += roundsSeen
	if rounds > 0 {
		fmt.Printf("accounting updated through round %d\n", currentRound)
	}
	return
}

var (
	genesisJsonPath string
	protoJsonPath   string
	numRoundsLimit  int
	blockFileLimit  int
)

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

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "import block file or tar file of blocks",
	Long:  "import block file or tar file of blocks. arguments are interpret as file globs (e.g. *.tar.bz2)",
	//Args:
	Run: func(cmd *cobra.Command, args []string) {
		// TODO: import from block catchup endpoint of a public archival node?
		db := globalIndexerDb()

		err := importer.ImportProto(db)
		maybeFail(err, "import proto, %v", err)

		imp := importer.NewDBImporter(db)
		blocks := 0
		start := time.Now()
		for _, fname := range args {
			matches, err := filepath.Glob(fname)
			if err == nil {
				pathsSorted := blockTarPaths(matches)
				sort.Sort(&pathsSorted)
				if blockFileLimit != 0 && len(pathsSorted) > blockFileLimit {
					pathsSorted = pathsSorted[:blockFileLimit]
				}
				for _, gfname := range pathsSorted {
					//fmt.Printf("%s ...\n", gfname)
					blocks += importFile(db, imp, gfname)
				}
			} else {
				// try without passing throug glob
				blocks += importFile(db, imp, fname)
			}
		}
		blockdone := time.Now()
		if blocks > 0 {
			dt := blockdone.Sub(start)
			fmt.Printf("%d blocks in %s, %.0f/s\n", blocks, dt.String(), float64(time.Second)*float64(blocks)/float64(dt))
		}

		accountingRounds := updateAccounting(db)

		accountingdone := time.Now()
		if accountingRounds > 0 {
			dt := accountingdone.Sub(blockdone)
			fmt.Printf("%d rounds accounting in %s, %.0f/s\n", accountingRounds, dt.String(), float64(time.Second)*float64(accountingRounds)/float64(dt))
		}
	},
}

func init() {
	importCmd.Flags().StringVarP(&genesisJsonPath, "genesis", "g", "", "path to genesis.json")
	importCmd.Flags().IntVarP(&numRoundsLimit, "num-rounds-limit", "", 0, "number of rounds to process")
	importCmd.Flags().IntVarP(&blockFileLimit, "block-file-limit", "", 0, "number of block files to process (for debugging)")
}
