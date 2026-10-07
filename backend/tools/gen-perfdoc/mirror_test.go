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
	assertMirrors(t, "BudgetMeasurement", reflect.TypeFor[measurement](), 8)
}

func TestTheRecordAndItsCorpusMirrorTheRecorders(t *testing.T) {
	assertMirrors(t, "PerfRecord", reflect.TypeFor[record](), 6)
	assertMirrors(t, "CorpusFacts", reflect.TypeFor[corpus](), 9)
}

// assertMirrors holds one recorder struct and its renderer twin to the same
// JSON tags; atLeast fails a parse that found fewer fields than exist.
func assertMirrors(t *testing.T, recorderType string, twin reflect.Type, atLeast int) {
	t.Helper()
	recorder := structTags(t, filepath.Join(integrationDir, "perfrecord.go"), recorderType)
	var renderer []string
	for i := range twin.NumField() {
		renderer = append(renderer, twin.Field(i).Tag.Get("json"))
	}
	slices.Sort(recorder)
	slices.Sort(renderer)
	if len(recorder) < atLeast || !slices.Equal(recorder, renderer) {
		t.Errorf("%s tags %v\n%s tags %v\nthe two sides of the record must match", recorderType, recorder, twin.Name(), renderer)
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
