// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The contract's refusal kinds are exactly the kinds the engine constructs.
//
// A client branches on `details.kind`. A kind the engine builds and the enum
// lacks reaches a client that cannot name it; a kind the enum carries and
// nothing builds is a branch the client writes for a case that never comes. The
// Go side declares two kinds nothing builds, so the set is read from the
// RefusalError literals themselves rather than from the declarations.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func TestTheRefusalKindEnumIsTheKindsTheEngineConstructs(t *testing.T) {
	t.Parallel()
	values := gatekit.PackageStringConstants(t, "internal/compose/analyticsquery")
	built := map[string]string{}
	err := filepath.WalkDir("internal", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if lit, ok := n.(*ast.CompositeLit); ok && isRefusalErrorType(lit.Type) {
				built[refusalKindOf(t, path, lit, values)] = path
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walking internal: %v", err)
	}
	if len(built) == 0 {
		t.Fatal("no RefusalError literal found — this census reads nothing and agrees with everything")
	}

	constructed := slices.Sorted(maps.Keys(built))
	if declared := crmYAMLEnum(t, "AnalyticsRefusalDetails", "kind"); !slices.Equal(declared, constructed) {
		t.Errorf("AnalyticsRefusalDetails.kind declares %v but the engine constructs %v — "+
			"a kind on one side only is one a client cannot name, or a branch for a case that never comes",
			declared, constructed)
	}
}

func isRefusalErrorType(expr ast.Expr) bool {
	switch typ := expr.(type) {
	case *ast.Ident:
		return typ.Name == "RefusalError"
	case *ast.SelectorExpr:
		return typ.Sel.Name == "RefusalError"
	}
	return false
}

// refusalKindOf resolves a literal's Kind to its wire value. A literal it cannot
// read fails rather than being skipped, or the census would come up short.
func refusalKindOf(t *testing.T, path string, lit *ast.CompositeLit, values map[string]string) string {
	t.Helper()
	for _, elt := range lit.Elts {
		field, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if key, ok := field.Key.(*ast.Ident); !ok || key.Name != "Kind" {
			continue
		}
		name := ""
		switch value := field.Value.(type) {
		case *ast.Ident:
			name = value.Name
		case *ast.SelectorExpr:
			name = value.Sel.Name
		}
		if kind, ok := values[name]; ok {
			return kind
		}
	}
	t.Fatalf("%s: a RefusalError literal whose Kind is not one of analyticsquery's named kinds — "+
		"name it by its constant so this census can read it", path)
	return ""
}

// AnalyticsRefusal restates Problem's envelope because the breaking-change
// check cannot read an allOf; the restatement must name the same members.
func TestTheRefusalEnvelopeIsTheProblemEnvelope(t *testing.T) {
	t.Parallel()
	schemas := crmYAMLSchemas(t)
	refusal := slices.Sorted(maps.Keys(schemas["AnalyticsRefusal"].Properties))
	problem := slices.Sorted(maps.Keys(schemas["Problem"].Properties))
	if len(problem) == 0 || !slices.Equal(refusal, problem) {
		t.Errorf("AnalyticsRefusal has %v and Problem has %v — one writer renders both", refusal, problem)
	}
}
