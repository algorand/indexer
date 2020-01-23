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
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	//"github.com/spf13/cobra/doc"

	idb "github.com/algorand/indexer/db"
	"github.com/algorand/indexer/importer"
)

func maybeFail(err error, errfmt string, params ...interface{}) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, errfmt, params...)
	os.Exit(1)
}

func importTar(imp importer.Importer, tarfile io.Reader) (err error) {
	tf := tar.NewReader(tarfile)
	var header *tar.Header
	header, err = tf.Next()
	for err == nil {
		if header.Typeflag != tar.TypeReg {
			return fmt.Errorf("cannot deal with non-regular-file tar entry %#v", header.Name)
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
			return fmt.Errorf("error reading tar entry %#v: %v", header.Name, err)
		}
		err = imp.ImportBlock(blockbytes)
		if err != nil {
			return fmt.Errorf("error importing tar entry %#v: %v", header.Name, err)
		}
		header, err = tf.Next()
	}
	if err == io.EOF {
		err = nil
	}
	return
}

func importFile(db idb.IndexerDb, imp importer.Importer, fname string) {
	imported, err := db.AlreadyImported(fname)
	maybeFail(err, "%s: %v\n", fname, err)
	if imported {
		return
	}
	if strings.HasSuffix(fname, ".tar") {
		fin, err := os.Open(fname)
		maybeFail(err, "%s: %v\n", fname, err)
		defer fin.Close()
		err = importTar(imp, fin)
		maybeFail(err, "%s: %v\n", fname, err)
	} else if strings.HasSuffix(fname, ".tar.bz2") {
		fin, err := os.Open(fname)
		maybeFail(err, "%s: %v\n", fname, err)
		defer fin.Close()
		bzin := bzip2.NewReader(fin)
		err = importTar(imp, bzin)
		maybeFail(err, "%s: %v\n", fname, err)
	} else {
		// assume a standalone block msgpack blob
		blockbytes, err := ioutil.ReadFile(fname)
		maybeFail(err, "%s: could not read, %v\n", fname, err)
		err = imp.ImportBlock(blockbytes)
		maybeFail(err, "%s: could not import, %v\n", fname, err)
	}
	err = db.MarkImported(fname)
	maybeFail(err, "%s: %v\n", fname, err)
}

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "import block file or tar file of blocks",
	Long:  "import block file or tar file of blocks. arguments are interpret as file globs (e.g. *.tar.bz2)",
	//Args:
	Run: func(cmd *cobra.Command, args []string) {
		// TODO: connect to db and instantiate Importer
		//imp := importer.NewPrintImporter()
		db := globalIndexerDb()
		imp := importer.NewDBImporter(db)
		for _, fname := range args {
			matches, err := filepath.Glob(fname)
			if err == nil {
				for _, gfname := range matches {
					fmt.Printf("%s ...\n", gfname)
					importFile(db, imp, gfname)
				}
			} else {
				// try without passing throug glob
				importFile(db, imp, fname)
			}
		}
	},
}
