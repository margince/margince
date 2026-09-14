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
//   - it refuses what will not change before dispatch (refuseAtStaging) — an
//     absolute denial, and any refusal under a category this installation
//     enforces;
//   - it fails closed when no authority is wired, rather than staging anyway.
//
// The last one is why this gate is not satisfied by a bare call. A census sees
// the CALL and cannot see that a nil authority made it a no-op; commsstager.go
// says so in its own comment and defends with an explicit error. Asserting the
// guard here is what keeps that defence from being deleted as belt-and-braces.
//
// THE CORPUS IS DERIVED FROM THE TREE, not from a file named here. It was one
// hard-coded path — internal/compose/commsstager.go — while this header claimed
// otherwise, and the claim went false the moment the controller mail lane was
// wired: controllermail.go stages a delivery, carries all four obligations, and
// the gate could not see it. That is the shape a census must never take. It
// reads a smaller tree than its subject, finds nothing wrong, and reports PASS
// with no failing assertion to notice.
//
// So the corpus is derived twice over, both times by reusing what the sibling
// staging gate already built: parseTreeUnder walks the whole compose TIER, not
// one directory, because a call site in a subpackage stages just as real a
// message; and stagingMethodNames reads the comms store's own Stage…Tx
// declarations, so a fourth door is a subject on the day it is written rather
// than on the day somebody remembers to list it. A new staging path is a
// subject by existing, and the two staging gates now read one corpus and ask it
// two different questions.

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// stagingObligations are the calls a staging method must pair. Each is the
// literal selector as it appears in source; a rename that broke one would fail
// this gate rather than silently stop matching, because the staging call is
// found the same way and a method matching none of them is not a subject.
const (
	callAuthorize = "AuthorizeStagingTx"
	// callAuthorizeDirected is the SAME obligation for a door that resumes a
	// message the engine refused: it authorizes on the caller's transaction
	// exactly as the ordinary call does, and additionally says where the
	// recorded decision authorizing that refusal lives. Either spelling
	// discharges the obligation; neither may be absent.
	callAuthorizeDirected = "AuthorizeStagingWithDecisionTx"
	callRefuse            = "refuseAtStaging"
	callEnqueue           = "EnqueueTx"
)

// stagingMethod is one method that writes a delivery row: where it lives, so a
// failure names a file a reader can open, and what it called.
type stagingMethod struct {
	name   string
	file   string
	decl   *ast.FuncDecl
	called map[string]bool
}

// stagingMethodsIn answers every method under a tier that stages a delivery.
//
// IT REUSES stagingdecision_test.go's parseTreeUnder and stagingMethodNames
// rather than walking and listing on its own, and that is the whole correction
// this gate needed. It parsed ONE hard-coded file while its header claimed a
// derived corpus; replacing that with a one-level ParseDir and a hand-kept list
// of stager names would have been the same defect twice — blind to a stager in
// a compose subpackage, and blind to a fourth Stage…Tx on the day it is written.
// The sibling already derives both, so the two staging gates now read ONE
// corpus and ask it two different questions.
func stagingMethodsIn(t *testing.T, fset *token.FileSet, root string) []stagingMethod {
	t.Helper()
	stagers := stagingMethodNames(t, fset)
	var out []stagingMethod
	for _, file := range parseTreeUnder(t, fset, root) {
		path := fset.Position(file.Pos()).Filename
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			called := callsIn(fn)
			if !stagesADelivery(called, stagers) {
				continue
			}
			out = append(out, stagingMethod{
				name: fn.Name.Name, file: filepath.Base(path), decl: fn, called: called,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].file != out[j].file {
			return out[i].file < out[j].file
		}
		return out[i].name < out[j].name
	})
	return out
}

// stagingCorpusFloor is the number of staging paths that must be found before
// any assertion below is trusted.
//
// Three today: mail and channel on the user lane, plus the controller lane. It
// is a FLOOR rather than an exact count so a fourth stager does not fail the
// gate for existing — but it must rise when one lands, because a corpus that
// silently shrank to two would otherwise still pass. The mismatch is what a
// reader is meant to notice.
//
// It is not a hard-coded copy of the subject: the tree is walked and the names
// are derived: this only refuses to trust a walk that came back near-empty.
const stagingCorpusFloor = 3

func TestEveryStagedDeliveryCarriesItsAuthorization(t *testing.T) {
	t.Parallel()

	subjects := stagingMethodsIn(t, token.NewFileSet(), composeTier)
	// Under-recognition is the one way this must not fail. A gate that walked
	// the wrong directory, or a rename that made every method stop matching,
	// would find no subjects and report PASS.
	if len(subjects) < stagingCorpusFloor {
		t.Fatalf("found %d staging methods in %s, want at least %d "+
			"(mail, channel, controller): the gate is looking in the wrong place, "+
			"the stagers were renamed, or a staging path was deleted",
			len(subjects), composeTier, stagingCorpusFloor)
	}

	for _, subject := range subjects {
		if !subject.called[callAuthorize] && !subject.called[callAuthorizeDirected] {
			t.Errorf("%s (%s) stages a delivery and never calls %s or %s: the message reaches "+
				"the send queue with nothing on record about why it was allowed",
				subject.name, subject.file, callAuthorize, callAuthorizeDirected)
		}
		if !subject.called[callRefuse] {
			t.Errorf("%s (%s) stages a delivery and never calls %s: the message reaches the "+
				"send queue with nothing on record about why it was allowed",
				subject.name, subject.file, callRefuse)
		}
		if !subject.called[callEnqueue] {
			// Not an obligation in itself — it is what makes the others
			// load-bearing. A stager that does not enqueue is not on the send
			// path, and the gate should say so rather than pass quietly.
			t.Errorf("%s (%s) stages a delivery and never enqueues a send job: "+
				"this gate's corpus has drifted from the send path", subject.name, subject.file)
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

	subjects := stagingMethodsIn(t, token.NewFileSet(), composeTier)
	if len(subjects) < stagingCorpusFloor {
		t.Fatalf("found %d staging methods in %s, want at least %d: this gate would "+
			"otherwise report PASS having checked nothing",
			len(subjects), composeTier, stagingCorpusFloor)
	}
	guarded := 0
	for _, subject := range subjects {
		if !guardsNilAuthority(subject.decl) {
			t.Errorf("%s (%s) does not refuse a missing authorization authority: without the "+
				"guard a dropped field stages every message with no decision and still passes "+
				"a call census", subject.name, subject.file)
			continue
		}
		guarded++
	}
	// Against the corpus, not the floor: a fourth stager with no guard would
	// otherwise leave guarded at 3, clear the floor, and make this final check
	// assert nothing while its message claimed "every one".
	if guarded < len(subjects) {
		t.Fatalf("%d of %d staging methods carry the nil-authority guard, want every one",
			guarded, len(subjects))
	}
}

// stagesADelivery reports whether a method writes a delivery row, against the
// stager names derived from the comms store rather than a list kept here.
func stagesADelivery(called map[string]bool, stagers map[string]bool) bool {
	for name := range stagers {
		if called[name] {
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
