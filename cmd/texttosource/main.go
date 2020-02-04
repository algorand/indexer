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
	"io/ioutil"
	"os"
	"strings"
)

// usage:
// go run cmd/texttosource/main.go packagename text.file.suffix ...
//
// outputs text_file_suffix.go containing constant text_file_suffix
func main() {
	packageName := os.Args[1]
	for _, fname := range os.Args[2:] {
		data, err := ioutil.ReadFile(fname)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", fname, err)
			os.Exit(1)
			return
		}
		outname := strings.ReplaceAll(fname, ".", "_") + ".go"
		fout, err := os.Create(outname)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", outname, err)
			os.Exit(1)
			return
		}
		varname := strings.ReplaceAll(fname, ".", "_")
		bodyConstant := "`" + strings.ReplaceAll(string(data), "`", "\\u0060") + "`"
		_, err = fmt.Fprintf(fout, `// Copyright (C) 2019-2020 Algorand, Inc.
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

// GENERATED CODE from source %s via go generate

package %s

const %s = %s
`, fname, packageName, varname, bodyConstant)
	}
}
