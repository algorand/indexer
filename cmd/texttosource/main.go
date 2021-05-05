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
		}
		outname := strings.ReplaceAll(fname, ".", "_") + ".go"
		fout, err := os.Create(outname)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", outname, err)
			os.Exit(1)
		}
		varname := strings.ReplaceAll(fname, ".", "_")
		bodyConstant := "`" + strings.ReplaceAll(string(data), "`", "\\u0060") + "`"
		_, err = fmt.Fprintf(fout, `// Code generated from source %s via go generate. DO NOT EDIT.

package %s

const %s = %s
`, fname, packageName, varname, bodyConstant)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to write to %s: %v\n", outname, err)
			os.Exit(1)
		}
	}
}
