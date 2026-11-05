// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// A read that shows the caller where an activity is filed asks
// auth.LinkTargetReadableClause, which also requires the read grant on the
// linked record's type. auth.LinkTargetVisibleClause is row scope alone, which
// reads every deal for every seat. So each caller of the visible clause says
// here why row scope alone is its question.

import (
	"go/ast"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// visibleClauseCallers ratifies each function that may call the row-scope-only
// clause, keyed package.function, with the reason the grant does not apply.
var visibleClauseCallers = gatekit.Waive(map[string]string{
	"activities.readRequestFiling": "asks where a request is filed for its assignee, as a SeatPrincipal that carries " +
		"no object grant; the filing decides which pages hold the task and discloses nothing to the caller",
	"activities.holdsAHiddenProjectLink": "answers only after auth.ReadGranted(project) has passed, so the grant " +
		"the readable clause would add is already held",
})

// visibleClauseName is the function this census is about.
const visibleClauseName = "LinkTargetVisibleClause"

func TestEveryRowScopeOnlyLinkClauseCallerSaysWhyTheGrantDoesNotApply(t *testing.T) {
	t.Parallel()
	defer visibleClauseCallers.AssertAllMatched(t)
	scope := gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: func(_ string, file *ast.File) bool { return len(visibleClauseCallersIn(file)) > 0 },
		Exempt:  gatekit.Waive(map[string]string{}),
	}
	found := 0
	for _, src := range scope.Files(t) {
		for _, caller := range visibleClauseCallersIn(src.File) {
			found++
			if !visibleClauseCallers.Waived(t, caller) {
				t.Errorf("%s: %s calls auth.%s, which checks row scope and not the read grant on the linked "+
					"record's type. A read that shows the caller a link asks auth.LinkTargetReadableClause; "+
					"anything else is added to visibleClauseCallers with the reason", src.Path, caller, visibleClauseName)
			}
		}
	}
	if found == 0 {
		t.Errorf("no caller of auth.%s was found; a census that sees nothing reports PASS, "+
			"so the walk has stopped seeing the call", visibleClauseName)
	}
}

// visibleClauseCallersIn names each function in the file that calls the
// clause, as package.function or package.Receiver.function. Only the qualified
// spelling counts: the bare one is platform/auth composing its own clause.
func visibleClauseCallersIn(file *ast.File) []string {
	var callers []string
	for _, decl := range file.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc || fn.Name.Name == visibleClauseName {
			continue
		}
		ast.Inspect(fn, func(node ast.Node) bool {
			selector, isSelector := node.(*ast.SelectorExpr)
			if !isSelector || selector.Sel.Name != visibleClauseName {
				return true
			}
			callers = append(callers, file.Name.Name+"."+qualifiedFuncName(fn))
			return false
		})
	}
	return callers
}
