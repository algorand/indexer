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
	"fmt"
	"io"
	"os"
	"runtime/pprof"

	"github.com/spf13/cobra"
	//"github.com/spf13/cobra/doc" // TODO: enable cobra doc generation

	"github.com/algorand/indexer/idb"
)

var rootCmd = &cobra.Command{
	Use:   "indexer",
	Short: "Algorand Indexer",
	Long:  `indexer imports blocks from an algod node or from local files into an SQL database for querying. indexer is a daemon that can serve queries from that database.`,
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		//If no arguments passed, we should fallback to help
		cmd.HelpFunc()(cmd, args)
	},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if cpuProfile != "" {
			var err error
			profFile, err = os.Create(cpuProfile)
			maybeFail(err, "%s: create, %v", cpuProfile, err)
			err = pprof.StartCPUProfile(profFile)
			maybeFail(err, "%s: start pprof, %v", cpuProfile, err)
		}
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		if cpuProfile != "" {
			pprof.StopCPUProfile()
			profFile.Close()
		}
	},
}

var (
	postgresAddr   string
	dummyIndexerDb bool
	cpuProfile     string
	db             idb.IndexerDb
	profFile       io.WriteCloser
)

func globalIndexerDb() idb.IndexerDb {
	if db == nil {
		if postgresAddr != "" {
			var err error
			db, err = idb.IndexerDbByName("postgres", postgresAddr)
			maybeFail(err, "could not init db, %v", err)
		} else if dummyIndexerDb {
			db = idb.DummyIndexerDb()
		} else {
			fmt.Fprintf(os.Stderr, "no import db set")
			os.Exit(1)
		}
	}
	return db
}

func init() {
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(daemonCmd)

	rootCmd.PersistentFlags().StringVarP(&postgresAddr, "postgres", "P", "", "connection string for postgres database")
	rootCmd.PersistentFlags().BoolVarP(&dummyIndexerDb, "dummydb", "n", false, "use dummy indexer db")
	rootCmd.PersistentFlags().StringVarP(&cpuProfile, "cpuprofile", "", "", "file to record cpu profile to")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
