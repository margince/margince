// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"go/ast"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// An edge annotates its anchor, so writing one is writing that record — and the
// anchor's authority is TWO questions, not one. `person.update` says this seat
// may change people; whether it may change THIS person is the row half, and on
// an anchor object it is the only half that decides anything: person,
// organization, deal and project are identity tables, read by every seat in the
// workspace, so the visibility probe an edge already takes passes for everyone.
//
// A verb that asks only the object grant is therefore not narrowly gated, it is
// ungated — an ordinary seat could demote another team's primary-employer edge
// or staff their project through POST/PATCH/DELETE /v1/relationships, and
// nothing else in the path refuses.
//
// The population is derived from the pairing rather than listed: a function
// that resolves an anchor with relationshipAnchor and then calls auth.Require
// is asking the object half, whatever it is called and wherever it lives. So a
// verb added tomorrow is judged without anyone remembering this file, and a
// verb that stops taking the object grant leaves the census the same way it
// entered it — visibly.

// anchorRowGate is the row half every asker of the object half owes.
const anchorRowGate = "ensureRelationshipAnchorWritable"

// anchorObjectResolver and anchorObjectGate are the pair that identifies an
// asker of the object half: the anchor is resolved from the kind, then required
// as an object grant. Either alone is something else — the audit subject
// resolves an anchor and gates nothing, and most of the module calls
// auth.Require about objects that are not anchors.
const (
	anchorObjectResolver = "relationshipAnchor"
	anchorObjectGate     = "Require"
	anchorObjectPackage  = "auth"
)

// wantMinimumAnchorAskers is the anti-vacuity floor. Seven functions ask the
// object half today, and naming them is the point of stating a number at all:
// the two create shapes, the update, the archive, the archive's own stage-time
// refusal, the edge reversal's stage-time refusal, and the stakeholder seat a
// lead qualification mints. A floor of five leaves room for one to be folded
// away, and still fails loudly if the pairing this gate matches on stops being
// recognisable.
const wantMinimumAnchorAskers = 5

// anchorAskersWithoutTheRowGate ratifies a function that asks the anchor's
// object grant and cannot ask its row authority. Each entry says why the row
// question is unanswerable there, not merely inconvenient.
var anchorAskersWithoutTheRowGate = gatekit.Waive(map[string]string{
	"RefuseEdgeWrite":           "the stage-time half of an edge reversal, which is handed a KIND and no edge — so there is no anchor row to probe, and it says so itself: it answers the object half early to keep a button honest, and the write it precedes takes the row half through UpdateRelationship or the archive. Gating here would need an id this signature does not carry",
	"seatPersonOnQualifiedDeal": "the stakeholder seat a lead qualification mints, whose person AND deal were both created in the very transaction that writes it. No row-scope probe can see either yet, so EnsureWritableLive would refuse every qualification; the authority is the promote's own, taken on the lead before any of the three rows exist",
})

func TestEveryGenericRelationshipVerbTakesTheAnchorRowGate(t *testing.T) {
	gated := functionsReachingTheAnchorRowGate(t)
	askers := 0
	forEachModuleFunc(t, func(parsed moduleFile, fn *ast.FuncDecl) {
		if !asksTheAnchorObjectGrant(fn) {
			return
		}
		askers++
		if callsAnyPackageFunc(fn, gated) || anchorAskersWithoutTheRowGate.Waived(t, fn.Name.Name) {
			return
		}
		t.Errorf("%s in %s resolves an edge's anchor and requires the object grant on it, and never "+
			"reaches %s.\n\n"+
			"`<anchor>.update` says the seat may change that KIND of record, not that it may change "+
			"the one this edge hangs on — and every anchor object is read by the whole workspace, so "+
			"the visibility probe an edge takes admits every seat. Without the row half any seat "+
			"holding the ordinary relationship grants rewrites another team's graph and the write "+
			"succeeds. Take the gate, or ratify the omission in %s with the reason the row question "+
			"cannot be asked here.",
			fn.Name.Name, parsed.name, anchorRowGate, "anchorAskersWithoutTheRowGate")
	})
	if askers < wantMinimumAnchorAskers {
		t.Errorf("only %d function(s) were recognised as asking the anchor's object grant, want at "+
			"least %d — the pairing this gate matches on (%s then %s.%s) no longer describes how the "+
			"module gates an edge, so it is judging a smaller module than it reports",
			askers, wantMinimumAnchorAskers, anchorObjectResolver, anchorObjectPackage, anchorObjectGate)
	}
	anchorAskersWithoutTheRowGate.AssertAllMatched(t)
}

// asksTheAnchorObjectGrant reports whether fn resolves an edge's anchor and
// then requires an object grant — the object half of the anchor gate.
func asksTheAnchorObjectGrant(fn *ast.FuncDecl) bool {
	return callsAnyPackageFunc(fn, map[string]bool{anchorObjectResolver: true}) &&
		callsAny(fn, anchorObjectPackage, map[string]bool{anchorObjectGate: true})
}

// functionsReachingTheAnchorRowGate names the gate plus every module function
// that calls it, so a verb whose gate is taken by a writer it delegates to is
// covered. One hop, not a full call graph: a verb two removes from its own
// authorization check is worth failing over.
func functionsReachingTheAnchorRowGate(t *testing.T) map[string]bool {
	t.Helper()
	gated := map[string]bool{anchorRowGate: true}
	forEachModuleFunc(t, func(_ moduleFile, fn *ast.FuncDecl) {
		if callsAnyPackageFunc(fn, map[string]bool{anchorRowGate: true}) {
			gated[fn.Name.Name] = true
		}
	})
	return gated
}

// callsAnyPackageFunc reports whether fn calls any of this package's named
// functions — a bare identifier call, which is how the module's own writers and
// gates are reached, or a call on fn's own receiver.
//
// Closures count. Both create shapes hand their write to s.tx as a function
// literal, so a walk that stopped at the first FuncLit would find neither of
// them reaching the gate their one writer takes.
func callsAnyPackageFunc(fn *ast.FuncDecl, names map[string]bool) bool {
	if fn.Body == nil {
		return false
	}
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if !isCall {
			return true
		}
		switch callee := call.Fun.(type) {
		case *ast.Ident:
			found = found || names[callee.Name]
		case *ast.SelectorExpr:
			// A method on this store's own receiver reads as a call to the
			// same declaration; anything qualified by a package does not, and
			// this gate's names are all declared here.
			if on, isIdent := callee.X.(*ast.Ident); isIdent && on.Name == receiverName(fn) {
				found = found || names[callee.Sel.Name]
			}
		}
		return !found
	})
	return found
}
