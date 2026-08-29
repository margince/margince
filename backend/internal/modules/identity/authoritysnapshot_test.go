// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// One transaction is not by itself one snapshot.
//
// The pool begins at READ COMMITTED, where every statement sees its own
// committed view — so the seat read, the passport re-check and the grant load
// can each answer from a different instant and compose an authority nobody ever
// held: permissions from before a role change beside a seat from after it. All
// three ARE the admission decision, so the composed answer is what a caller is
// admitted on.
//
// The pin is what makes sharing the transaction mean something, and it belongs
// at BEGIN rather than as a statement inside the closure: Postgres refuses the
// level once any query has taken a snapshot, and DB.Tx runs one of its own on a
// BOUNDED handle (the statement-timeout set_config). A pin spelled inside would
// therefore work on the handle identity has today and fail on the one compose
// could give it tomorrow — the same code, correct or broken depending on how it
// was wired.
//
// So what is checked is that the authority read OPENS at that level, which is
// where the guarantee now lives.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestTheAuthorityReadPinsItsSnapshotBeforeReadingAnything(t *testing.T) {
	t.Parallel()
	const source = "authority.go"
	file, err := parser.ParseFile(token.NewFileSet(), source, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s: %v", source, err)
	}
	body := funcBody(t, file, "liveUserTx")
	if body == nil {
		t.Fatalf("%s has no liveUserTx — every authority read went through it, so this test is reading for something that has moved", source)
	}
	if !opensAt(body, "TxIsolated", "RepeatableRead") {
		t.Error("the authority read no longer opens its transaction at REPEATABLE READ — at READ COMMITTED the seat, the passport and the grants each answer from their own instant, and the admission decision is composed from values that never existed together")
	}
}

// opensAt reports whether a body calls the named transaction opener with the
// named isolation level.
func opensAt(body *ast.BlockStmt, opener, level string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if !isCall {
			return true
		}
		sel, isSel := call.Fun.(*ast.SelectorExpr)
		if !isSel || sel.Sel.Name != opener {
			return true
		}
		for _, arg := range call.Args {
			if named, isSel := arg.(*ast.SelectorExpr); isSel && named.Sel.Name == level {
				found = true
			}
		}
		return !found
	})
	return found
}
