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
	"github.com/spf13/cobra"

	"github.com/algorand/indexer/api"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "run indexer daemon",
	Long:  "run indexer daemon. Serve api on HTTP. (TODO: follow blocks from algod)",
	//Args:
	Run: func(cmd *cobra.Command, args []string) {
		// TODO: -p/--port
		// TODO: -d/$ALGORAND_DATA/--no-algod algod to follow
		api.IndexerDb = globalIndexerDb()
		api.Serve()
	},
}
