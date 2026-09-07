// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Two paths can give a connection a live backfill, and they take the connection
// row before either of them looks at capture_backfill.
//
// `uq_capture_backfill_live` admits one live run per connection, and a
// predicate alone does not hold that: a human pressing Import while a sync
// commits passes its own NOT EXISTS at the same instant, and one of the two
// takes the unique violation. StartBackfill can answer that — it returns
// ErrBackfillRunning — and the revival cannot, because the statement that fails
// is inside somebody's successful sync transaction, which then rolls back and
// takes a working mailbox's sync down with it.
//
// So the obligation is an ORDER, and an order is exactly what a later edit
// loses without anything failing: the race is rare, the tests pass, and the
// mailbox that breaks is somebody else's. This reads the package's own source
// and requires the lock to come first.
//
// Why not a contention test instead: what a running database would prove is
// that FOR UPDATE blocks, which is Postgres's contract and not ours. What can
// regress here is forgetting to take it, and that is a property of the text.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// liveRunWrites are the SQL fragments that give a connection a live run. Both
// are matched as substrings of a statement literal, because the two writers
// spell the same act differently — one inserts a queued row, the other moves an
// errored one back to queued.
var liveRunWrites = []string{
	"INSERT INTO capture_backfill (",
	"UPDATE capture_backfill SET status = 'queued'",
}

const connectionLock = "lockConnectionTx"

func TestEveryWriterOfALiveBackfillLocksTheConnectionFirst(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package directory: %v", err)
	}
	judged := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, name, nil, 0)
		if parseErr != nil {
			t.Fatalf("parsing %s: %v", name, parseErr)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			writeAt, ok := firstLiveRunWrite(fn)
			if !ok {
				continue
			}
			judged++
			lockAt, locked := firstCallTo(fn, connectionLock)
			switch {
			case !locked:
				t.Errorf("%s: %s gives a connection a live backfill and never takes %s. "+
					"uq_capture_backfill_live is the only thing stopping a second live run, and the "+
					"violation it raises is answerable on one of these paths and not the other",
					filepath.Join("internal/modules/capture", name), fn.Name.Name, connectionLock)
			case lockAt > writeAt:
				t.Errorf("%s: %s takes %s AFTER it has already read or written capture_backfill. "+
					"A lock taken after the decision serializes nothing — both transactions have "+
					"already seen a connection with no live run",
					filepath.Join("internal/modules/capture", name), fn.Name.Name, connectionLock)
			}
		}
	}
	// A census that can fail short has already failed: a rewritten statement
	// leaves this walking no subjects and reporting a clean package.
	if judged < len(liveRunWrites) {
		t.Fatalf("only %d writer(s) of a live backfill found, want at least %d — the statements were "+
			"rewritten and this gate is judging nothing. Teach liveRunWrites their new spelling",
			judged, len(liveRunWrites))
	}
}

// firstLiveRunWrite is the position of the earliest statement in this function
// that gives a connection a live run.
func firstLiveRunWrite(fn *ast.FuncDecl) (token.Pos, bool) {
	first, found := token.Pos(0), false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		// The literal's STRING, never its source text: a statement written in
		// double quotes reaches a source-text reader as its escapes, and the
		// fragments below would then match nothing on the very shape this is
		// looking for.
		expr, isExpr := node.(ast.Expr)
		if !isExpr {
			return true
		}
		text, isString := gatekit.LiteralText(expr)
		if !isString {
			return true
		}
		for _, fragment := range liveRunWrites {
			if !strings.Contains(text, fragment) {
				continue
			}
			if !found || node.Pos() < first {
				first, found = node.Pos(), true
			}
		}
		return true
	})
	return first, found
}

// firstCallTo is the position of the earliest call to the named function.
func firstCallTo(fn *ast.FuncDecl, name string) (token.Pos, bool) {
	first, found := token.Pos(0), false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}
		ident, isIdent := call.Fun.(*ast.Ident)
		if !isIdent || ident.Name != name {
			return true
		}
		if !found || call.Pos() < first {
			first, found = call.Pos(), true
		}
		return true
	})
	return first, found
}
