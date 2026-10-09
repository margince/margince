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
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(info os.FileInfo) bool { return !strings.HasSuffix(info.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatal(err)
	}
	var callers []string
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
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
	}
	if len(callers) != 1 || !strings.Contains(callers[0], "schedulingcalendar.go") {
		t.Fatalf("registry CalendarFor is called from %v, want only schedulingcalendar.go's calendarFor", callers)
	}
}
