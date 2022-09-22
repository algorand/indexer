package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/algorand/go-algorand/data/transactions"
)

// recursiveTagFields recursively gets all field names in a struct
// Output will contain a key of the full tag along with the fully qualified struct
func recursiveTagFields(theStruct interface{}, Output *map[string]string, tagLevel []string, fieldLevel []string) {
	rStruct := reflect.TypeOf(theStruct)
	numFields := rStruct.NumField()
	for i := 0; i < numFields; i++ {
		field := rStruct.Field(i)
		name := field.Name

		var tagValue string
		var foundTag bool
		// If there is a codec tag...
		if tagValue, foundTag = field.Tag.Lookup("codec"); foundTag {

			vals := strings.Split(tagValue, ",")
			// Get the first value (the one we care about)
			tagValue = vals[0]
			// If it is empty ignore it
			if tagValue == "" {
				continue
			}

			fullTag := strings.Join(append(tagLevel, tagValue), ".")
			(*Output)[fullTag] = strings.Join(append(fieldLevel, name), ".")
		}

		if field.Type.Kind() == reflect.Struct {
			var passedTagLevel []string
			if foundTag {
				passedTagLevel = append(tagLevel, tagValue)
			} else {
				passedTagLevel = tagLevel
			}
			recursiveTagFields(reflect.New(field.Type).Elem().Interface(), Output, passedTagLevel, append(fieldLevel, name))
		}
	}
}

// usage:
// go run generate.go packagename outputfile
func main() {

	var packageName string
	var outputFilepath string

	if len(os.Args) == 3 {
		packageName = os.Args[1]
		outputFilepath = os.Args[2]
	}

	if packageName == "" {
		packageName = "NULL"
	}

	output := make(map[string]string)
	tagLevel := []string{}
	fieldLevel := []string{}

	recursiveTagFields(transactions.SignedTxnInBlock{}, &output, tagLevel, fieldLevel)

	var err error
	var bb bytes.Buffer

	initialStr := `// Code generated via go generate. DO NOT EDIT.

package %s

import "github.com/algorand/go-algorand/data/transactions"

// SignedTxnMap generates a map with the key as the codec tag and the value as a function
// that returns the associated variable
var SignedTxnMap = map[string]func(*transactions.SignedTxnInBlock) interface{}{
`
	_, err = fmt.Fprintf(&bb, initialStr, packageName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to write to %s: %v\n", outputFilepath, err)
		os.Exit(1)
	}

	keys := []string{}

	for k := range output {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, k := range keys {
		fmt.Fprintf(&bb, "\"%s\": func(i *transactions.SignedTxnInBlock)interface{}{return &((*i).%s)},\n", k, output[k])
	}

	_, err = fmt.Fprintf(&bb, "}")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to write to %s: %v\n", outputFilepath, err)
		os.Exit(1)
	}

	bbuf, err := format.Source(bb.Bytes())

	outputStr := string(bbuf)

	if outputFilepath == "" {
		fmt.Printf("%s", outputStr)
	} else {
		fout, err := os.Create(outputFilepath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cannot open %s for writing: %v\n", outputFilepath, err)
			os.Exit(1)
		}
		defer fout.Close()
		fmt.Fprintf(fout, "%s", outputStr)
	}

}
