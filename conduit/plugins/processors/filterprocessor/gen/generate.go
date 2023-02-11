package main

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"text/template"

	"github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/transactions"
)

type StructField struct {
	TagPath    string
	FieldPath  string
	CastPrefix string
	CastPost   string
}

func ReturnValue(sf StructField, varName string) string {
	return fmt.Sprintf("%s%s.%s%s", sf.CastPrefix, varName, sf.FieldPath, sf.CastPost)
}

func noCast(t reflect.StructField) bool {
	switch reflect.New(t.Type).Elem().Interface().(type) {
	case uint64:
		return true
	case int64:
		return true
	case string:
		return true
	}
	return false
}

func simpleCast(t reflect.StructField) string {
	switch reflect.New(t.Type).Elem().Interface().(type) {
	// unsigned
	case uint:
		return "uint64"
	case uint8:
		return "uint64"
	case uint16:
		return "uint64"
	case uint32:
		return "uint64"
	// signed
	case int:
		return "int64"
	case int8:
		return "int64"
	case int16:
		return "int64"
	case int32:
		return "int64"
	// alias
	case types.MicroAlgos:
		// SDK microalgo does not need ".Raw"
		return "uint64"

	}
	return ""

}

func CastParts(t reflect.StructField) (prefix, postfix string, err error) {
	if noCast(t) {
		return
	}

	if simple := simpleCast(t); simple != "" {
		prefix = fmt.Sprintf("%s(", simple)
		postfix = ")"
		return
	}

	// all the rest... custom things
	switch reflect.New(t.Type).Elem().Interface().(type) {
	case basics.MicroAlgos:
		prefix = "uint64("
		postfix = ".Raw)"
	default:
		prefix = "NOT "
		postfix = " HANDLED"
	}
	return
}

func getFields(theStruct interface{}) (map[string]StructField, error) {
	output := make(map[string]StructField)
	err := recursiveTagFields(theStruct, output, nil, nil)
	return output, err
}

// recursiveTagFields recursively gets all field names in a struct
// Output will contain a key of the full tag along with the fully qualified struct
func recursiveTagFields(theStruct interface{}, output map[string]StructField, tagLevel []string, fieldLevel []string) error {
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
			sf := StructField{
				TagPath:   fullTag,
				FieldPath: strings.Join(append(fieldLevel, name), "."),
			}
			var err error
			sf.CastPrefix, sf.CastPost, err = CastParts(field)
			if err != nil {
				return fmt.Errorf("problem casting %s: %s", fullTag, err)
			}
			output[fullTag] = sf
		}

		if field.Type.Kind() == reflect.Struct {
			var passedTagLevel []string
			if foundTag {
				passedTagLevel = append(tagLevel, tagValue)
			} else {
				passedTagLevel = tagLevel
			}
			recursiveTagFields(reflect.New(field.Type).Elem().Interface(), output, passedTagLevel, append(fieldLevel, name))
		}
	}
	return nil
}

const templateStr = `// Code generated via go generate. DO NOT EDIT.

package {{ .PackageName }}

import (
	"fmt"

	"github.com/algorand/go-algorand/data/transactions"
)

// LookupFieldByTag takes a tag and associated SignedTxnInBlock and returns the value 
// referenced by the tag.  An error is returned if the tag does not exist
func LookupFieldByTag(tag string, input *transactions.SignedTxnInBlock) (interface{}, error) {
	switch tag {
{{ range .StructFields }}	case "{{ .TagPath }}":
		return {{ ReturnValue . "input" }}
{{ end }}default:
		return nil, fmt.Errorf(\"unknown tag: %s\", tag)
	}
}
`

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

	// Initialize template, no point to continue if there is a problem with it.
	ut, err := template.
		New("LookupFieldByTag").
		Funcs(map[string]interface{}{
			"ReturnValue": ReturnValue,
		}).
		Parse(templateStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse template string: %s", err)
		os.Exit(1)
	}

	// Process fields.
	fields, err := getFields(transactions.SignedTxnInBlock{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to get fields for struct: %s", err)
		os.Exit(1)
	}

	// Setup writer to stdout or file.
	var outputWriter io.Writer
	if outputFilepath == "" {
		outputWriter = os.Stdout
	} else {
		fout, err := os.Create(outputFilepath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cannot open %s for writing: %v\n", outputFilepath, err)
			os.Exit(1)
		}
		defer fout.Close()
		outputWriter = fout
	}

	// Prepare template inputs.
	data := struct {
		StructFields map[string]StructField
		PackageName  string
	}{
		StructFields: fields,
		PackageName:  packageName,
	}

	// Process template and write results.
	err = ut.Execute(outputWriter, data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Template execute failure: %s", err)
		os.Exit(1)
	}
}
