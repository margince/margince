// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// declaredValues are the values of every constant of the named type, read from
// this package's source. A new one then cannot hide from its list.
func declaredValues(t *testing.T, typeName string) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			out = append(out, constValuesOf(t, decl, typeName)...)
		}
	}
	if len(out) == 0 {
		t.Fatalf("no constant of type %s found: the census read nothing", typeName)
	}
	slices.Sort(out)
	return out
}

func constValuesOf(t *testing.T, decl ast.Decl, typeName string) []string {
	t.Helper()
	gen, ok := decl.(*ast.GenDecl)
	if !ok || gen.Tok != token.CONST {
		return nil
	}
	var out []string
	for _, spec := range gen.Specs {
		value, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		if ident, typed := value.Type.(*ast.Ident); !typed || ident.Name != typeName {
			continue
		}
		for _, v := range value.Values {
			lit, ok := v.(*ast.BasicLit)
			if !ok {
				t.Fatalf("a %s constant is not a string literal: %#v", typeName, v)
			}
			text, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, text)
		}
	}
	return out
}

func TestEveryDeclaredFileReasonIsListedWithItsKind(t *testing.T) {
	var listed []string
	for _, reason := range FileFaultReasons() {
		listed = append(listed, string(reason))
		if kind := FileReasonKind(reason); kind == nil {
			t.Errorf("%s refines no sentinel", reason)
		}
	}
	if declared := declaredValues(t, "FileReason"); !slices.Equal(declared, listed) {
		t.Errorf("declared reasons %v, listed %v: list every FileReason in fileReasonKinds", declared, listed)
	}
}

func TestEveryDeclaredRefusalParamIsListed(t *testing.T) {
	var listed []string
	for _, param := range RefusalParams() {
		listed = append(listed, string(param))
	}
	if declared := declaredValues(t, "RefusalParam"); !slices.Equal(declared, listed) {
		t.Errorf("declared params %v, listed %v: list every RefusalParam in refusalParams", declared, listed)
	}
}
