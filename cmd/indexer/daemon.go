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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/algorand/go-algorand-sdk/encoding/json"
	"github.com/spf13/cobra"

	"github.com/algorand/indexer/algobot"
	"github.com/algorand/indexer/api"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/importer"
	"github.com/algorand/indexer/types"
)

var (
	algodDataDir     string
	algodAddr        string
	algodToken       string
	daemonServerAddr string
	noAlgod          bool
	developerMode    bool
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "run indexer daemon",
	Long:  "run indexer daemon. Serve api on HTTP.",
	//Args:
	Run: func(cmd *cobra.Command, args []string) {
		if algodDataDir == "" {
			algodDataDir = os.Getenv("ALGORAND_DATA")
		}
		db := globalIndexerDb()

		ctx, cf := context.WithCancel(context.Background())
		defer cf()
		var bot algobot.Algobot
		var err error
		if noAlgod {
			fmt.Fprint(os.Stderr, "algod block following disabled")
		} else if algodAddr != "" && algodToken != "" {
			bot, err = algobot.ForNetAndToken(algodAddr, algodToken)
			maybeFail(err, "algobot setup, %v", err)
		} else if algodDataDir != "" {
			if genesisJsonPath == "" {
				genesisJsonPath = filepath.Join(algodDataDir, "genesis.json")
			}
			bot, err = algobot.ForDataDir(algodDataDir)
			maybeFail(err, "algobot setup, %v", err)
		} else {
			// no algod was found
			noAlgod = true
		}
		if !noAlgod {
			// Only do this if we're going to be writing
			// to the db, to allow for read-only query
			// servers that hit the db backend.
			err := importer.ImportProto(db)
			maybeFail(err, "import proto, %v", err)
		}
		if bot != nil {
			maxRound, err := db.GetMaxRound()
			if err == nil {
				bot.SetNextRound(maxRound + 1)
			}
			bih := blockImporterHandler{
				imp:   importer.NewDBImporter(db),
				db:    db,
				round: maxRound,
			}
			bot.AddBlockHandler(&bih)
			bot.SetContext(ctx)
			go func() {
				bot.Run()
				cf()
			}()
		}
		fmt.Printf("serving on %s\n", daemonServerAddr)
		api.Serve(ctx, daemonServerAddr, db, developerMode)
	},
}

func init() {
	daemonCmd.Flags().StringVarP(&algodDataDir, "algod", "d", "", "path to algod data dir, or $ALGORAND_DATA")
	daemonCmd.Flags().StringVarP(&algodAddr, "algod-net", "", "", "host:port of algod")
	daemonCmd.Flags().StringVarP(&algodToken, "algod-token", "", "", "api access token for algod")
	daemonCmd.Flags().StringVarP(&genesisJsonPath, "genesis", "g", "", "path to genesis.json (defaults to genesis.json in algod data dir if that was set)")
	daemonCmd.Flags().StringVarP(&daemonServerAddr, "server", "S", ":8980", "host:port to serve API on (default :8980)")
	daemonCmd.Flags().BoolVarP(&noAlgod, "no-algod", "", false, "disable connecting to algod for block following")
	daemonCmd.Flags().BoolVarP(&developerMode, "dev-mode", "", false, "allow performance intensive operations like searching for accounts at a particular round.")
}

type blockImporterHandler struct {
	imp   importer.Importer
	db    idb.IndexerDb
	round uint64
}

func (bih *blockImporterHandler) HandleBlock(block *types.EncodedBlockCert) {
	start := time.Now()
	if uint64(block.Block.Round) != bih.round+1 {
		fmt.Fprintf(os.Stderr, "received block %d when expecting %d\n", block.Block.Round, bih.round+1)
	}
	bih.imp.ImportDecodedBlock(block)
	updateAccounting(bih.db)
	dt := time.Now().Sub(start)
	if len(block.Block.Payset) == 0 {
		// accounting won't have updated the round state, so we do it here
		stateJsonStr, err := db.GetMetastate("state")
		var state idb.ImportState
		if err == nil && stateJsonStr != "" {
			state, err = idb.ParseImportState(stateJsonStr)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error parsing import state, %v\n", err)
				panic("error parsing import state in bih")
			}
		}
		state.AccountRound = int64(block.Block.Round)
		stateJsonStr = string(json.Encode(state))
		err = db.SetMetastate("state", stateJsonStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to save import state, %v\n", err)
		}
	}
	fmt.Printf("round r=%d (%d txn) imported in %s\n", block.Block.Round, len(block.Block.Payset), dt.String())
	bih.round = uint64(block.Block.Round)
}
