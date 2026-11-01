// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

import (
	"go/ast"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func TestSourceAssessmentVocabularyMatchesTheDutyDecision(t *testing.T) {
	t.Parallel()
	root := gatekit.ModuleRoot(t)
	file := parseModuleFile(t, filepath.Join(root, "internal/modules/consent/noticerule.go"))
	functions := map[string]*ast.FuncDecl{}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			functions[fn.Name.Name] = fn
		}
	}
	duty := functions["DutyFor"]
	if duty == nil {
		t.Fatal("DutyFor not found")
	}
	eligible := map[string]bool{}
	decided := map[string]bool{}
	ast.Inspect(duty.Body, func(node ast.Node) bool {
		clause, ok := node.(*ast.CaseClause)
		if !ok || len(clause.List) == 0 {
			return true
		}
		var returned *ast.ReturnStmt
		for _, stmt := range clause.Body {
			if ret, ok := stmt.(*ast.ReturnStmt); ok {
				returned = ret
			}
		}
		if returned == nil || len(returned.Results) != 2 {
			t.Fatal("unrecognized duty decision")
		}
		state := assessmentDutyState(t, returned.Results[0], functions, 0)
		for _, expr := range clause.List {
			kind, ok := literalString(expr)
			if !ok {
				t.Fatal("acquisition switch kind is not a literal")
			}
			decided[kind] = true
			if state != "NoticeSourceReview" {
				eligible[kind] = true
			}
		}
		return false
	})
	kinds := acquisitionKinds(t)
	if len(kinds) < 9 || len(decided) != len(kinds) {
		t.Fatal("duty census missed acquisition kinds")
	}
	for _, kind := range kinds {
		if !decided[kind] {
			t.Errorf("missing duty decision for %s", kind)
		}
	}
	contract := parseModuleFile(t, filepath.Join(root, "internal/contracts/api_gen.go"))
	apiKinds := map[string]bool{}
	ast.Inspect(contract, func(node ast.Node) bool {
		spec, ok := node.(*ast.ValueSpec)
		if !ok || len(spec.Names) != 1 || len(spec.Values) != 1 {
			return true
		}
		typ, ok := spec.Type.(*ast.Ident)
		if !ok || typ.Name != "AssessNoticeSourceKind" {
			return true
		}
		value, ok := literalString(spec.Values[0])
		if !ok {
			t.Fatal("source assessment enum is not a literal")
		}
		apiKinds[value] = true
		return false
	})
	if !reflect.DeepEqual(apiKinds, eligible) {
		t.Errorf("API assessment kinds %v differ from duty decisions %v", apiKinds, eligible)
	}
	source, err := os.ReadFile(filepath.Join(root, "../frontend/src/features/privacy/noticesource.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	filter := regexp.MustCompile(`(?s)const SOURCES = ACQUISITION_KINDS.filter\((.*?)\);`).FindSubmatch(source)
	if len(filter) != 2 {
		t.Fatal("assessment choices no longer derive from acquisition vocabulary")
	}
	excluded := regexp.MustCompile(`kind !== "([^"]+)"`).FindAllSubmatch(filter[1], -1)
	if len(excluded) == 0 {
		t.Fatal("assessment filter was not recognized")
	}
	uiKinds := map[string]bool{}
	for _, kind := range kinds {
		uiKinds[kind] = true
	}
	for _, match := range excluded {
		delete(uiKinds, string(match[1]))
	}
	if !reflect.DeepEqual(uiKinds, eligible) {
		t.Errorf("UI assessment choices %v differ from duty decisions %v", uiKinds, eligible)
	}
}

func assessmentDutyState(t *testing.T, expr ast.Expr, functions map[string]*ast.FuncDecl, depth int) string {
	t.Helper()
	if depth > len(functions) {
		t.Fatal("recursive duty factory")
	}
	switch value := expr.(type) {
	case *ast.CompositeLit:
		for _, elt := range value.Elts {
			field, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				t.Fatal("unrecognized duty field")
			}
			key, ok := field.Key.(*ast.Ident)
			if ok && key.Name == "State" {
				state, ok := field.Value.(*ast.Ident)
				if !ok {
					t.Fatal("unrecognized initial duty state")
				}
				return state.Name
			}
		}
		return ""
	case *ast.CallExpr:
		name, ok := value.Fun.(*ast.Ident)
		if !ok || len(value.Args) != 0 {
			t.Fatal("unrecognized duty factory")
		}
		fn := functions[name.Name]
		if fn == nil {
			t.Fatal("duty factory not found")
		}
		for _, stmt := range fn.Body.List {
			if ret, ok := stmt.(*ast.ReturnStmt); ok && len(ret.Results) == 1 {
				return assessmentDutyState(t, ret.Results[0], functions, depth+1)
			}
		}
	}
	t.Fatal("duty state could not be derived")
	return ""
}
