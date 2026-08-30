// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// activities.RetractDerivedForActivityTx documents that it is not atomic with
// the narrowing it follows, and the sentence is only true while every caller is
// an async consumer reacting to a COMMITTED audience change.
//
// A caller added inside the narrowing's own transaction would make the comment
// false in the safe direction — atomic after all — but the write-side re-checks
// it justifies (the embedding upsert's FOR SHARE, SetCaptureLabel's audience
// clause) are shaped for the async case and would then be defending a window
// that no longer exists, with nothing saying so. Either way the next reader is
// owed the truth, and a comment nothing holds is what stops them checking.

import (
	"go/ast"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// retractionCallers ratifies each function allowed to call the retraction, with
// the event it reacts to. A waiver rather than a plain map so a caller that goes
// away is reported instead of sitting here describing nothing.
var retractionCallers = gatekit.Waive(map[string]string{
	"AudienceRescopeGen.rescope": "the cg:audience-rescope consumer, which reads activity.updated off the outbox — the narrowing committed before the event was published",
})

func TestTheRetractionRunsOnlyBehindACommittedNarrowing(t *testing.T) {
	t.Parallel()
	defer retractionCallers.AssertAllMatched(t)
	scope := gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: func(_ string, file *ast.File) bool { return callsTheRetraction(file) },
		Exempt:  gatekit.Waive(map[string]string{}),
	}
	found := 0
	for _, src := range scope.Files(t) {
		for _, caller := range retractionCallersIn(src.File) {
			found++
			if !retractionCallers.Waived(t, caller) {
				t.Errorf("%s: %s calls RetractDerivedForActivityTx, and the function's doc says every caller is an "+
					"async consumer behind a committed narrowing. Either it is one — add it to retractionCallers with "+
					"the event it reacts to — or the doc and the write-side re-checks it justifies are now wrong",
					src.Path, caller)
			}
		}
	}
	if found == 0 {
		t.Error("no caller of RetractDerivedForActivityTx was found — a census that sees nothing reports PASS, " +
			"so either the call moved to a spelling this walk cannot see, or the retraction is dead code")
	}
}

func callsTheRetraction(file *ast.File) bool {
	return len(retractionCallersIn(file)) > 0
}

// retractionCallersIn names each function in the file that calls the retraction,
// as receiver.name so a method reads the way the allow-list spells it.
func retractionCallersIn(file *ast.File) []string {
	var callers []string
	for _, decl := range file.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc {
			continue
		}
		// Both spellings. An importer calls `activities.Retract…` and reads as
		// a SelectorExpr; a caller inside the activities package calls it bare
		// and reads as an Ident. A walk that saw only the first would report
		// PASS over a caller added in the retraction's own package, which is
		// exactly where the next one is most likely to appear.
		// Its own declaration is not a call of itself: the Ident arm below sees
		// the name in `func RetractDerivedForActivityTx`, and reporting that
		// would make the census fail on the very function it is about.
		if fn.Name.Name == retractionName {
			continue
		}
		ast.Inspect(fn, func(node ast.Node) bool {
			if !namesTheRetraction(node) {
				return true
			}
			callers = append(callers, qualifiedFuncName(fn))
			return false
		})
	}
	return callers
}

// namesTheRetraction answers whether this node is a reference to the retraction,
// qualified or bare.
func namesTheRetraction(node ast.Node) bool {
	switch named := node.(type) {
	case *ast.SelectorExpr:
		return named.Sel.Name == retractionName
	case *ast.Ident:
		return named.Name == retractionName
	default:
		return false
	}
}

// retractionName is the function this census is about.
const retractionName = "RetractDerivedForActivityTx"

// qualifiedFuncName is `Receiver.Name` for a method and `Name` for a function.
func qualifiedFuncName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	receiver := fn.Recv.List[0].Type
	if star, isStar := receiver.(*ast.StarExpr); isStar {
		receiver = star.X
	}
	ident, isIdent := receiver.(*ast.Ident)
	if !isIdent {
		return fn.Name.Name
	}
	return strings.TrimSpace(ident.Name) + "." + fn.Name.Name
}
