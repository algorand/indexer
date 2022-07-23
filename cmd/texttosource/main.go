package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"text/template"

	"github.com/algorand/indexer/idb/postgres"
)

type Interpolators struct {
	AppBox string
}

// usage:
// go run cmd/texttosource/main.go packagename constantname inputfile outputfile
func main() {
	interpolators := Interpolators{
		AppBox: postgres.AppBoxMigration,
	}

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
	tmpl, err := template.New("setup_postgres_sql").Parse(body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot parse template for %s: %v\n", outputFilepath, err)
		os.Exit(1)
	}

	var bodyBuff bytes.Buffer
	err = tmpl.Execute(&bodyBuff, interpolators)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot interpolate template with %+v for %s: %v\n", interpolators, outputFilepath, err)
		os.Exit(1)
	}
	body = bodyBuff.String()

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
