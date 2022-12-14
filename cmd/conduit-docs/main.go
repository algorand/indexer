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
	md := ""
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

		md = md + processConfig(structType, typeTok.Name.Name)
	}
	err = os.MkdirAll(outputDir, os.ModePerm)
	if err != nil {
		return err
	}
	docPath := path.Join(outputDir, pf.Name.Name+".md")
	return os.WriteFile(docPath, []byte(md), os.ModePerm)
}

type tableEntry struct {
	key         string
	valueType   string
	description string
}

func (t tableEntry) renderMd() string {
	return "| " + t.key + " | " + t.valueType + " | " + t.description + " |"
}

func renderConfigTable(typeName string, entries []tableEntry) string {
	header := "| key | type | description |\n|---|---|---|"
	table := []string{header}
	for _, entry := range entries {
		table = append(table, entry.renderMd())
	}
	return strings.Join(table, "\n")
}

func processConfig(configStruct *ast.StructType, structName string) string {
	var tableEntries []tableEntry
	for _, field := range configStruct.Fields.List {
		// TODO We don't handle embedded structs
		comment := strings.ReplaceAll(field.Doc.Text(), "\n", " ")
		tag := field.Tag.Value
		var valueType string
		ident, ok := field.Type.(*ast.Ident)
		if ok {
			valueType = ident.Name
		}
		selExpr, ok := field.Type.(*ast.SelectorExpr)
		if ok {
			valueType = selExpr.X.(*ast.Ident).Name + "." + selExpr.Sel.Name
		}
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
