// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
//gate:kind census H3

package gates

// Every path that stages a delivery records why it was allowed to.
//
// The transmit ticket catches a delivery that reaches a provider with nothing
// on record, but it catches it in a worker — minutes or days later, against a
// parked row, with the person who typed the message long gone. The staging
// decision is what makes a refusal answerable, and this is what stops a new
// send path from quietly not taking one.
//
// Derived rather than listed: the corpus is every caller of a Stage*Tx method
// on the comms store, so a send path added tomorrow is judged the day it is
// written. A hand-kept list would go stale exactly when a new door appeared,
// which is the failure this gate exists to prevent.

import (
	"go/ast"
	"go/token"
	"testing"
)

func TestEveryStagedDeliveryRecordsWhyItWasAllowed(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	staging := map[string]bool{}     // functions that stage a delivery
	authorizing := map[string]bool{} // functions that record a decision

	for _, file := range parsePackageDir(t, fset, composeTier) {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			// Receiver-qualified, because the method NAME is fixed by the
			// interface: every type satisfying activities.DeliveryStager must
			// call its method StageTx. Keyed on the bare name, a wrapper that
			// staged without deciding would land in the same map entry as the
			// one that decides, and the gate would absorb it and report PASS —
			// which is the exact shape a second send door takes.
			name := receiverQualified(fn)
			ast.Inspect(fn, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				switch sel.Sel.Name {
				// The comms store's two delivery-staging entry points, named
				// exactly. A prefix match also catches unrelated staging —
				// StageOrJoinPendingInTx on the site-read transport — which
				// queues no message and owes no decision.
				case "StageTx", "StageChannelTx":
					staging[name] = true
				case "AuthorizeStagingTx":
					authorizing[name] = true
				}
				return true
			})
		}
	}

	// Under-recognition is the one way this gate must not break: a scan that
	// found nothing would report PASS over an empty corpus.
	if len(staging) == 0 {
		t.Fatal("found no delivery staging in compose — the scan is looking in the wrong place")
	}
	for fn := range staging {
		if !authorizing[fn] {
			t.Errorf("%s stages a delivery and records no authorization decision — "+
				"a message queued with nothing on record about why is one nobody can answer for", fn)
		}
	}
}

// receiverQualified names a function the way this gate must count it: the
// receiver type and the method, so two types implementing one interface method
// are two subjects rather than one.
func receiverQualified(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	expr := fn.Recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if id, ok := expr.(*ast.Ident); ok {
		return id.Name + "." + fn.Name.Name
	}
	return fn.Name.Name
}
