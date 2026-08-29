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
// The pin is what makes sharing the transaction mean something, and Postgres
// enforces the rest for us: SET TRANSACTION ISOLATION LEVEL is refused once a
// query has taken a snapshot, so a statement that arrives before it does not
// weaken the guarantee quietly — every admission read fails loudly instead.
// What that leaves to check here is that the pin is FIRST.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
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

	statements := transactionBody(body)
	if len(statements) == 0 {
		t.Fatal("liveUserTx opens no transaction body this test can read")
	}
	if !mentions(statements[0], "SET TRANSACTION ISOLATION LEVEL REPEATABLE READ") {
		t.Errorf("the first statement of the authority transaction is not the isolation pin — at READ COMMITTED the seat, the passport and the grants each answer from their own instant, and the admission decision is composed from values that never existed together")
	}
}

// transactionBody returns the statements of the closure handed to s.db.Tx.
func transactionBody(body *ast.BlockStmt) []ast.Stmt {
	var out []ast.Stmt
	ast.Inspect(body, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if !isCall {
			return true
		}
		sel, isSel := call.Fun.(*ast.SelectorExpr)
		if !isSel || sel.Sel.Name != "Tx" {
			return true
		}
		for _, arg := range call.Args {
			fn, isFunc := arg.(*ast.FuncLit)
			if isFunc && fn.Body != nil {
				out = fn.Body.List
				return false
			}
		}
		return true
	})
	return out
}

// mentions reports whether a statement carries a string LITERAL saying want.
//
// The literal's decoded text, never its source form: a statement written in
// double quotes reaches the source as its escapes, so a reader matching on
// lit.Value answers about a spelling rather than about what Postgres receives.
// gatekit.LiteralText is the one decoding in this tree.
func mentions(stmt ast.Stmt, want string) bool {
	found := false
	ast.Inspect(stmt, func(n ast.Node) bool {
		lit, isLit := n.(*ast.BasicLit)
		if !isLit {
			return !found
		}
		if text, isString := gatekit.LiteralText(lit); isString && strings.Contains(text, want) {
			found = true
		}
		return !found
	})
	return found
}
