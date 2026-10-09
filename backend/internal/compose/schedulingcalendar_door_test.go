// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// The scheduling seam maps "no connection" in calendarFor alone, so a second
// call to the registry's CalendarFor would leak the raw sentinel again.
func TestOnlyCalendarForCallsTheRegistryForACalendar(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var callers []string
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "CalendarFor" {
				callers = append(callers, fset.Position(call.Pos()).String())
			}
			return true
		})
	}
	if len(callers) != 1 || !strings.HasPrefix(callers[0], "schedulingcalendar.go") {
		t.Fatalf("registry CalendarFor is called from %v, want only schedulingcalendar.go's calendarFor", callers)
	}
}
