// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A message that reaches the send queue carries a decision saying why, written
// in the transaction that staged it.
//
// The property is per-staging-method rather than per-file, and that is the
// point: mail and channel are two implementations of one act, and the recurring
// defect in this area is a fix landing on one of them. A method that stages a
// delivery and enqueues the job WITHOUT authorizing it produces a delivery the
// worker will send with nothing on record about why it exists — and it looks,
// in review, exactly like the method next to it that does.
//
// Four obligations, all of which must appear in the same method body:
//
//   - it stages a row (comms.Store.Stage*Tx);
//   - it authorizes (AuthorizeStagingTx) on the SAME transaction;
//   - it refuses an absolute denial (refuseAbsoluteDenial) — the four refusals
//     no rollout mode may soften;
//   - it fails closed when no authority is wired, rather than staging anyway.
//
// The last one is why this gate is not satisfied by a bare call. A census sees
// the CALL and cannot see that a nil authority made it a no-op; commsstager.go
// says so in its own comment and defends with an explicit error. Asserting the
// guard here is what keeps that defence from being deleted as belt-and-braces.
//
// NOT in the corpus: comms.Store.StageControllerTx. It has no production caller
// on main — the controller mail lane it belongs to was never wired — so it
// stages nothing today and an obligation on it would be an assertion about code
// that does not run. It joins the corpus the moment a caller appears, because
// the corpus is derived from who calls the stagers rather than listed here.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// stagingObligations are the calls a staging method must pair. Each is the
// literal selector as it appears in source; a rename that broke one would fail
// this gate rather than silently stop matching, because the staging call is
// found the same way and a method matching none of them is not a subject.
const (
	callAuthorize = "AuthorizeStagingTx"
	callRefuse    = "refuseAbsoluteDenial"
	callEnqueue   = "EnqueueTx"
)

// stagerCalls are the delivery-row writers. A method calling one of these is
// staging a message and owes the obligations above.
var stagerCalls = []string{"StageTx", "StageChannelTx", "StageControllerTx"}

func TestEveryStagedDeliveryCarriesItsAuthorization(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset,
		"internal/compose/commsstager.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing the staging seam: %v", err)
	}

	subjects := map[string]map[string]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		called := callsIn(fn)
		if !stagesADelivery(called) {
			continue
		}
		subjects[fn.Name.Name] = called
	}

	// Under-recognition is the one way this must not fail. A gate that parsed
	// the wrong file, or a rename that made every method stop matching, would
	// find no subjects and report PASS.
	if len(subjects) < 2 {
		t.Fatalf("found %d staging methods, want at least the mail and channel pair: "+
			"the gate is looking in the wrong place or the stagers were renamed", len(subjects))
	}

	names := map[string]bool{}
	for name := range subjects {
		names[name] = true
	}
	for _, name := range sortedKeys(names) {
		called := subjects[name]
		for _, want := range []string{callAuthorize, callRefuse} {
			if !called[want] {
				t.Errorf("%s stages a delivery and never calls %s: the message reaches the "+
					"send queue with nothing on record about why it was allowed", name, want)
			}
		}
		if !called[callEnqueue] {
			// Not an obligation in itself — it is what makes the others
			// load-bearing. A stager that does not enqueue is not on the send
			// path, and the gate should say so rather than pass quietly.
			t.Errorf("%s stages a delivery and never enqueues a send job: "+
				"this gate's corpus has drifted from the send path", name)
		}
	}
}

// TestAStagingPathWithNoAuthorityFailsRatherThanStages holds the guard a call
// census cannot see.
//
// AuthorizeStagingTx appearing in the body proves the code was written; it does
// not prove the authority was wired. A refactor dropping the field would
// compile, stage every message with no decision, and satisfy the census above.
// The explicit nil check is the defence, and this is what stops it being
// removed as redundant.
func TestAStagingPathWithNoAuthorityFailsRatherThanStages(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset,
		"internal/compose/commsstager.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing the staging seam: %v", err)
	}

	guarded := 0
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || !stagesADelivery(callsIn(fn)) {
			continue
		}
		if !guardsNilAuthority(fn) {
			t.Errorf("%s does not refuse a missing authorization authority: without the guard a "+
				"dropped field stages every message with no decision and still passes a call census",
				fn.Name.Name)
			continue
		}
		guarded++
	}
	if guarded < 2 {
		t.Fatalf("%d staging methods carry the nil-authority guard, want at least the mail and "+
			"channel pair", guarded)
	}
}

// stagesADelivery reports whether a method writes a delivery row.
func stagesADelivery(called map[string]bool) bool {
	for _, stager := range stagerCalls {
		if called[stager] {
			return true
		}
	}
	return false
}

// callsIn collects every selector called in a function body, by its final name
// (`s.store.StageTx(...)` yields "StageTx"). The receiver is deliberately
// ignored: this gate asks WHAT was called, and matching on the variable holding
// the store would stop working the moment somebody renamed the field.
func callsIn(fn *ast.FuncDecl) map[string]bool {
	out := map[string]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch f := call.Fun.(type) {
		case *ast.SelectorExpr:
			out[f.Sel.Name] = true
		case *ast.Ident:
			out[f.Name] = true
		}
		return true
	})
	return out
}

// guardsNilAuthority reports whether the body refuses when the authority field
// is nil. It looks for a comparison against nil that returns, which is the
// shape both stagers use, rather than the exact text of either.
func guardsNilAuthority(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		stmt, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		cmp, ok := stmt.Cond.(*ast.BinaryExpr)
		if !ok || cmp.Op != token.EQL {
			return true
		}
		if !isNil(cmp.Y) || !mentionsAuthority(cmp.X) {
			return true
		}
		if returnsIn(stmt.Body) {
			found = true
		}
		return true
	})
	return found
}

// isNil reports whether an expression is the nil identifier.
func isNil(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "nil"
}

// mentionsAuthority reports whether an expression names the authority field.
func mentionsAuthority(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	return ok && strings.EqualFold(sel.Sel.Name, "authority")
}

// returnsIn reports whether a block returns, which is what makes a guard a
// refusal rather than a log line.
func returnsIn(block *ast.BlockStmt) bool {
	found := false
	ast.Inspect(block, func(n ast.Node) bool {
		if _, ok := n.(*ast.ReturnStmt); ok {
			found = true
		}
		return true
	})
	return found
}
