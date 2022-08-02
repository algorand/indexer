package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
)

// usage:
// go run cmd/texttosource/main.go packagename constantname inputfile outputfile
func main() {
	packageName := os.Args[1]
	constantName := os.Args[2]
	inputFilepath := os.Args[3]
	outputFilepath := os.Args[4]

	data, err := ioutil.ReadFile(inputFilepath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot read file %s: %v\n", inputFilepath, err)
		os.Exit(1)
	}

	body := "`" + strings.ReplaceAll(string(data), "`", "\\u0060") + "`"

	fout, err := os.Create(outputFilepath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot open %s for writing: %v\n", outputFilepath, err)
		os.Exit(1)
	}

	format := `// Code generated from source %s via go generate. DO NOT EDIT.

package %s

const %s = %s
`
	_, err = fmt.Fprintf(fout, format, inputFilepath, packageName, constantName, body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to write to %s: %v\n", outputFilepath, err)
		os.Exit(1)
	}

	fout.Close()
}
