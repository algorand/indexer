package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
	"path"
	"strings"
)

const namePrefix = "Name: "
const headerPrefix = "Header\n"
const footerPrefix = "Footer\n"

// generateMd takes the path of a file containing a struct definition named "Config", and an output directory,
// and writes a markdown file to the outputDir containing mkdocs-style documentation for the Config struct
func generateMd(configPath string, outputDir string) error {
	// parse the config file
	bytes, err := ioutil.ReadFile(configPath)
	if err != nil {
		return err
	}
	fset := token.NewFileSet()
	pf, err := parser.ParseFile(fset, configPath, bytes, parser.ParseComments)
	// _ = ast.Print(fset, pf)
	if err != nil {
		return err
	}
	var body string
	var title string
	var examples string
	var fileName string
	// Process freestanding comments into docs sections
	for _, comm := range pf.Comments {
		if strings.HasPrefix(comm.Text(), headerPrefix) {
			title = strings.TrimPrefix(comm.Text(), headerPrefix)
		}
		if strings.HasPrefix(comm.Text(), footerPrefix) {
			examples = strings.TrimPrefix(comm.Text(), footerPrefix)
		}
		if strings.HasPrefix(comm.Text(), namePrefix) {
			fileName = strings.TrimSuffix(strings.TrimPrefix(comm.Text(), namePrefix), "\n")
			fmt.Println(fileName)
		}
	}
	// Process struct decls into tables describing their fields
	for _, decl := range pf.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		typeTok, ok := genDecl.Specs[0].(*ast.TypeSpec)
		if !ok {
			continue
		}
		structType, ok := typeTok.Type.(*ast.StructType)
		if !ok {
			continue
		}

		body = body + processConfig(structType, typeTok.Name.Name, bytes)
	}
	err = os.MkdirAll(outputDir, os.ModePerm)
	if err != nil {
		return err
	}
	if fileName == "" {
		fileName = pf.Name.Name
	}
	docPath := path.Join(outputDir, fileName+".md")
	return os.WriteFile(docPath, []byte(title+body+examples), os.ModePerm)
}

type tableEntry struct {
	key         string
	valueType   string
	description string
}

func (t tableEntry) renderMd() string {
	return "<tr><td>" + t.key + "</td><td>" + t.valueType + "</td><td>" + t.description + "</td></tr>\n"
}

func renderConfigTable(typeName string, entries []tableEntry) string {
	header := "\n### " + typeName + "\n<table>\n<tr>\n<th>key</th><th>type</th><th>description</th>\n"
	table := []string{header}
	for _, entry := range entries {
		table = append(table, entry.renderMd())
	}
	return strings.Join(table, "\n") + "</table>\n\n"
}

func processConfig(configStruct *ast.StructType, structName string, fileBytes []byte) string {
	var tableEntries []tableEntry
	for _, field := range configStruct.Fields.List {
		// TODO We don't handle embedded structs
		comment := field.Doc.Text()
		// Strip `yaml:"foobar"` down to foobar
		tag := field.Tag.Value
		tag = tag[7:]
		tag = tag[:len(tag)-2]
		var valueType string
		valueType = string(fileBytes[field.Type.Pos()-1 : field.Type.End()-1])
		valueType = strings.ReplaceAll(valueType, "*", `\*`)
		tableEntries = append(tableEntries, tableEntry{
			key:         tag,
			valueType:   valueType,
			description: comment,
		})
	}
	return renderConfigTable(structName, tableEntries)
}

func main() {
	usage := "USAGE: //go:generate conduit-docs <path-to-output-dir>"
	// go:generate conduit-docs [path]
	if len(os.Args) == 2 {
		err := generateMd(os.Getenv("GOFILE"), os.Args[1])
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	fmt.Println(usage)
	os.Exit(1)
}
