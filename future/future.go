package future

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"
)

type missing struct {
	shouldMiss bool
	msg        string
}

func (hm *missing) handleMissing(isMissing bool) error {
	if hm.shouldMiss != isMissing {
		return fmt.Errorf(hm.msg)
	}
	return nil
}

type expectation struct {
	File        string
	MissingFile missing

	Name          string
	MissingStruct missing

	Fields        []string
	MissingFields missing
}

func getProblematicExpectations(t *testing.T, expectations []expectation) (problems [][]error) {
	problems = [][]error{}

	for i, expected := range expectations {
		fmt.Printf(`
____________________________________________
%d. Expectations for file %s
Expecting: %#v`, i+1, expected.File, expected)

		prob := []error{}

		fset := token.NewFileSet()
		root, err := parser.ParseFile(fset, expected.File, nil, parser.ParseComments)

		missingFile := false
		if err != nil {
			fmt.Printf("\nHeads up! ParseFile failed on [%v]", err)
			missingFile = true
		}

		fileExpectationError := expected.MissingFile.handleMissing(missingFile)
		if fileExpectationError != nil {
			prob = append(prob, fileExpectationError)
		}

		if missingFile {
			problems = append(problems, prob)
			continue
		}

		fileStructs := getFileStructs(root)
		fmt.Printf("\nStructs found in %s: %s", expected.File, fileStructs)

		foundStruct, ok := fileStructs[expected.Name]
		structExpectationError := expected.MissingStruct.handleMissing(!ok)
		if structExpectationError != nil {
			prob = append(prob, structExpectationError)
		}

		if !ok {
			problems = append(problems, prob)
			continue
		}

		missingSomeField := false
		for _, field := range expected.Fields {
			_, ok := foundStruct[field]
			if !ok {
				missingSomeField = true
			}
		}
		fieldsExpectationError := expected.MissingFields.handleMissing(missingSomeField)
		if fieldsExpectationError != nil {
			prob = append(prob, fieldsExpectationError)
		}
		problems = append(problems, prob)
	}

	fmt.Printf("\nAll Problems: %+v", problems)

	return
}

func getTopLevelTypeNodes(root *ast.File) (typeSpecs []*ast.TypeSpec) {
	for _, decl := range root.Decls {
		if decl == nil {
			continue
		}
		gDecl, ok := decl.(*ast.GenDecl)
		if !ok || gDecl == nil {
			continue
		}
		for _, spec := range gDecl.Specs {
			tSpec, ok := spec.(*ast.TypeSpec)
			if !ok || tSpec == nil {
				continue
			}
			typeSpecs = append(typeSpecs, tSpec)
		}
	}
	return
}

// first elmt of stct is the struct's name, the rest are fields
func getStructFieldNames(tSpec *ast.TypeSpec) (stct []string) {
	if tSpec == nil || tSpec.Type == nil {
		return
	}
	stct = append(stct, tSpec.Name.Name)
	sType, ok := tSpec.Type.(*ast.StructType)
	if !ok {
		return
	}
	if sType.Fields == nil || sType.Fields.List == nil {
		return
	}
	for _, field := range sType.Fields.List {
		if field == nil || field.Names == nil {
			continue
		}
		for _, name := range field.Names {
			stct = append(stct, name.Name)
		}
	}
	return
}

type fStructs map[string]map[string]bool

func (fs fStructs) String() string {
	parts := make([]string, len(fs))
	for stct, fset := range fs {
		fields := []string{}
		for field := range fset {
			fields = append(fields, field)
		}
		sort.Strings(fields)
		parts = append(parts, fmt.Sprintf("%s: %s", stct, fields))
	}
	return strings.Join(parts, "\n")
}

// returns map from struct's type name, to its set of fields
func getFileStructs(root *ast.File) fStructs {
	fileStructs := map[string]map[string]bool{}
	for _, tSpec := range getTopLevelTypeNodes(root) {
		structInfo := getStructFieldNames(tSpec)
		if len(structInfo) > 0 {
			fields := map[string]bool{}
			for _, field := range structInfo[1:] {
				fields[field] = true
			}
			fileStructs[structInfo[0]] = fields
		}
	}
	return fileStructs
}
