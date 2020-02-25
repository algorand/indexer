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
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "run indexer daemon",
	Long:  "run indexer daemon. Serve api on HTTP. (TODO: follow blocks from algod)",
	//Args:
	Run: func(cmd *cobra.Command, args []string) {
		// TODO: -p/--port
		// TODO: -d/$ALGORAND_DATA/--no-algod algod to follow
		if algodDataDir == "" {
			algodDataDir = os.Getenv("ALGORAND_DATA")
		}
		db := globalIndexerDb()
		if len(protoJsonPath) > 0 {
			importProto(db, protoJsonPath)
		}
		ctx, cf := context.WithCancel(context.Background())
		defer cf()
		var bot algobot.Algobot
		var err error
		if algodAddr != "" && algodToken != "" {
			bot, err = algobot.ForNetAndToken(algodAddr, algodToken)
			maybeFail(err, "algobot setup, %v", err)
		} else if algodDataDir != "" {
			bot, err = algobot.ForDataDir(algodDataDir)
			maybeFail(err, "algobot setup, %v", err)
		}
		if bot != nil {
			maxRound, err := db.GetMaxRound()
			if err == nil {
				bot.SetNextRound(maxRound + 1)
			}
			bih := blockImporterHandler{
				imp: importer.NewDBImporter(db),
				db:  db,
			}
			bot.AddBlockHandler(&bih)
			bot.SetContext(ctx)
			go func() {
				bot.Run()
				cf()
			}()
		}
		api.IndexerDb = db
		api.Serve(ctx, daemonServerAddr)
	},
}

func init() {
	daemonCmd.Flags().StringVarP(&algodDataDir, "algod", "d", "", "path to algod data dir, or $ALGORAND_DATA")
	daemonCmd.Flags().StringVarP(&algodAddr, "algod-net", "", "", "host:port of algod")
	daemonCmd.Flags().StringVarP(&algodToken, "algod-token", "", "", "api access token for algod")
	daemonCmd.Flags().StringVarP(&genesisJsonPath, "genesis", "g", "", "path to genesis.json")
	daemonCmd.Flags().StringVarP(&protoJsonPath, "proto", "p", "", "path to proto.json")
	daemonCmd.Flags().StringVarP(&daemonServerAddr, "server", "S", ":8980", "host:port to serve API on (default :8980)")
}

type blockImporterHandler struct {
	imp importer.Importer
	db  idb.IndexerDb
}

func (bih blockImporterHandler) HandleBlock(block *types.EncodedBlockCert) {
	start := time.Now()
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
}
