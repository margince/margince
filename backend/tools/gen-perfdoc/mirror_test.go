// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The recorder lives in the backend module, which this module cannot import,
// so the mirror is checked by parsing its source.
var integrationDir = filepath.Join("..", "..", "internal", "compose", "integration")

func TestTheMeasurementMirrorsTheRecordersJSONFields(t *testing.T) {
	recorder := structTags(t, filepath.Join(integrationDir, "perfrecord.go"), "BudgetMeasurement")
	var renderer []string
	typ := reflect.TypeFor[measurement]()
	for i := range typ.NumField() {
		renderer = append(renderer, typ.Field(i).Tag.Get("json"))
	}
	slices.Sort(recorder)
	slices.Sort(renderer)
	if len(recorder) < 8 || !slices.Equal(recorder, renderer) {
		t.Errorf("BudgetMeasurement tags %v\nmeasurement tags      %v\nthe two sides of the record must match", recorder, renderer)
	}
}

func TestTheStoredVerdictsMirrorTheBenchsVerdicts(t *testing.T) {
	bench := typedStringConsts(t, filepath.Join(integrationDir, "daily_budgets_bench_test.go"), "DailyVerdict")
	renderer := []string{storedWithin, storedOverKnown, storedOver, storedNoData, storedNotGated}
	slices.Sort(bench)
	slices.Sort(renderer)
	if !slices.Equal(bench, renderer) {
		t.Errorf("the bench writes %v, the renderer reads %v", bench, renderer)
	}
}

func parseFile(t *testing.T, path string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return file
}

func structTags(t *testing.T, path, name string) []string {
	t.Helper()
	var tags []string
	ast.Inspect(parseFile(t, path), func(n ast.Node) bool {
		spec, ok := n.(*ast.TypeSpec)
		if !ok || spec.Name.Name != name {
			return true
		}
		fields, ok := spec.Type.(*ast.StructType)
		if !ok {
			t.Fatalf("%s in %s is not a struct", name, path)
		}
		for _, f := range fields.Fields.List {
			raw, err := strconv.Unquote(f.Tag.Value)
			if err != nil {
				t.Fatalf("%s: tag %s: %v", path, f.Tag.Value, err)
			}
			tags = append(tags, reflect.StructTag(raw).Get("json"))
		}
		return false
	})
	if len(tags) == 0 {
		t.Fatalf("no struct %s with fields in %s", name, path)
	}
	return tags
}

// typedStringConsts reads the values of every constant declared with the given
// type, including those that inherit it inside one const block.
func typedStringConsts(t *testing.T, path, typeName string) []string {
	t.Helper()
	var values []string
	for _, decl := range parseFile(t, path).Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			v, ok := spec.(*ast.ValueSpec)
			if !ok || len(v.Values) != 1 {
				continue
			}
			if id, ok := v.Type.(*ast.Ident); !ok || id.Name != typeName {
				continue
			}
			lit, ok := v.Values[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				t.Fatalf("%s: a %s constant is not a string literal", path, typeName)
			}
			values = append(values, strings.Trim(lit.Value, "\""))
		}
	}
	if len(values) == 0 {
		t.Fatalf("no %s constants in %s", typeName, path)
	}
	return values
}
