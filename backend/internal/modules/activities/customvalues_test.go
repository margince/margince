// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// customValueFillers are the calls that put custom-field values on an answer.
var customValueFillers = []string{"attachCustomValues", "withCustomValuesOf", "ListActivitiesTx", "ListActivitiesIn", "ListActivities"}

// answersWithoutCustomValues are the exported methods that answer an activity
// they just created, on a path that writes no custom-field values.
var answersWithoutCustomValues = gatekit.Waive(map[string]string{
	"BookMeeting":    "answers the meeting the booking just wrote, and booking takes no custom-field values",
	"SendEmail":      "answers the message the send just wrote, and a send takes no custom-field values",
	"SendMessage":    "answers the message the send just wrote, and a send takes no custom-field values",
	"SendPreparedTx": "answers the message the send just wrote, and a send takes no custom-field values",
	"ResumeHeldSend": "answers the scheduled message a send wrote, and a send takes no custom-field values",
})

// Every exported store method that answers an activity carries its custom-field
// values, so no door hands back a record that silently lacks them.
func TestEveryReturnedActivityCarriesItsCustomFieldValues(t *testing.T) {
	defer answersWithoutCustomValues.AssertAllMatched(t)
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !fn.Name.IsExported() || !onStore(fn) || !answersActivity(fn) {
				continue
			}
			checked++
			if answersWithoutCustomValues.Waived(t, fn.Name.Name) {
				continue
			}
			if !callsAny(fn, customValueFillers) {
				t.Errorf("%s (%s) answers an activity and fills no custom-field values: call attachCustomValues "+
					"before it returns, or name it in answersWithoutCustomValues with the reason", fn.Name.Name, name)
			}
		}
	}
	if checked < 10 {
		t.Fatalf("found %d store methods answering an activity, and there are at least 10: the reader stopped seeing them", checked)
	}
}

func onStore(fn *ast.FuncDecl) bool {
	if fn.Recv == nil || len(fn.Recv.List) != 1 {
		return false
	}
	star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	ident, ok := star.X.(*ast.Ident)
	return ok && ident.Name == "Store"
}

// answersActivity reports whether a result is crmcontracts.Activity or a slice of it.
func answersActivity(fn *ast.FuncDecl) bool {
	if fn.Type.Results == nil {
		return false
	}
	for _, field := range fn.Type.Results.List {
		expr := field.Type
		if slice, ok := expr.(*ast.ArrayType); ok {
			expr = slice.Elt
		}
		if sel, ok := expr.(*ast.SelectorExpr); ok && sel.Sel.Name == "Activity" {
			return true
		}
	}
	return false
}

func callsAny(fn *ast.FuncDecl, names []string) bool {
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch called := call.Fun.(type) {
		case *ast.Ident:
			found = found || slices.Contains(names, called.Name)
		case *ast.SelectorExpr:
			found = found || slices.Contains(names, called.Sel.Name)
		}
		return !found
	})
	return found
}
