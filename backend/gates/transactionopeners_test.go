// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates_test

// One function in the database package turns a pool into a transaction, and
// every seam the package publishes routes through it. That is what makes "a
// domain row commits with its audit row and its outbox row" a property held in
// a single place instead of a habit each store repeats.
//
// A second opener is the hardest kind of drift to see: it begins a transaction,
// it commits, and it reads correctly in review.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// databasePackage is the seam this gate is about — where a pool becomes a
// transaction for domain work.
const databasePackage = "internal/platform/database"

// transactionOpener names the function in that package permitted to call
// pool.Begin. Every seam the package publishes — WithWorkspaceTx, WithInfraTx,
// DB.Tx, DB.TxIsolated — routes through it, which is what makes "a domain row
// commits with its audit row and its outbox row" a property held in a single
// place.
//
// runTx is NOT a second opener: it supplies the default options and calls this,
// so the rollback discipline still lives in exactly one function. What made a
// second name necessary is the isolation LEVEL, which Postgres will only take
// at BEGIN — a caller cannot set it afterwards, and a read whose answer is
// composed from several statements has to open at REPEATABLE READ or the
// statements each answer from their own instant.
const transactionOpener = "runTxWith"

// TestTheDatabasePackageOpensATransactionInOneFunction is what holds runTx's
// doc comment: no other function in the package turns a pool into a
// transaction.
//
// A second opener does not look wrong at the call site: it begins a transaction,
// it commits, it reads correctly in review — and it is a second place where the
// rollback discipline can drift, invisibly, from the one above it.
func TestTheDatabasePackageOpensATransactionInOneFunction(t *testing.T) {
	entries, err := os.ReadDir(databasePackage)
	if err != nil {
		t.Fatalf("reading %s: %v", databasePackage, err)
	}

	// The corpus is asserted before the verdict: a scan that read no files
	// would report PASS having checked nothing, which is the one way a census
	// must not fail.
	var scanned int
	var offenders []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(databasePackage, name)
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		scanned++
		offenders = append(offenders, beginCallersOutside(file, name)...)
	}

	if scanned == 0 {
		t.Fatalf("scanned no files under %s — the gate read a smaller tree than it protects", databasePackage)
	}
	if len(offenders) > 0 {
		t.Fatalf("%d function(s) besides %s open a transaction on the pool:\n\t%s\n\n"+
			"Route each through %s. A second opener is a second place the rollback "+
			"discipline can drift, and the drift is invisible at the call site.",
			len(offenders), transactionOpener, strings.Join(offenders, "\n\t"), transactionOpener)
	}
}

// TestBeginCallersOutsideCatchesAMethodValue proves the walk still finds a
// second opener when the call target is a captured method VALUE
// (begin := pool.Begin; begin(ctx)) rather than a selector expression at the
// call site — the shape a walk that only inspected *ast.SelectorExpr call
// targets would miss and report PASS on.
func TestBeginCallersOutsideCatchesAMethodValue(t *testing.T) {
	const source = `package database

func other(ctx context.Context, pool *pgxpool.Pool) error {
	begin := pool.Begin
	tx, err := begin(ctx)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
`
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, 0)
	if err != nil {
		t.Fatalf("parsing fixture source: %v", err)
	}
	offenders := beginCallersOutside(file, "fixture.go")
	if len(offenders) != 1 || offenders[0] != "fixture.go: other" {
		t.Fatalf("beginCallersOutside(fixture) = %v, want exactly [\"fixture.go: other\"] — "+
			"a transaction opener reached through a captured method value must be reported "+
			"the same as one reached through pool.Begin(ctx) directly", offenders)
	}
}

// TestBeginCallersOutsideCatchesAVarDeclaredAlias is the sibling gap in the
// same alias tracking: `begin := pool.Begin` is an *ast.AssignStmt, but
// `var begin = pool.Begin` is an *ast.GenDecl holding an *ast.ValueSpec — a
// different node entirely. A walk that only matched AssignStmt would read a
// tree containing this spelling as one with no second opener and report PASS.
func TestBeginCallersOutsideCatchesAVarDeclaredAlias(t *testing.T) {
	const source = `package database

func other(ctx context.Context, pool *pgxpool.Pool) error {
	var begin = pool.Begin
	tx, err := begin(ctx)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
`
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, 0)
	if err != nil {
		t.Fatalf("parsing fixture source: %v", err)
	}
	offenders := beginCallersOutside(file, "fixture.go")
	if len(offenders) != 1 || offenders[0] != "fixture.go: other" {
		t.Fatalf("beginCallersOutside(fixture) = %v, want exactly [\"fixture.go: other\"] — "+
			"a transaction opener reached through a var-declared alias must be reported "+
			"the same as one reached through pool.Begin(ctx) directly", offenders)
	}
}

// TestBeginCallersOutsideCatchesAParenthesizedSelectorCall is the gap a walk
// that switches on call.Fun's exact node type leaves open: `(pool.Begin)(ctx)`
// parenthesizes the call target, so it parses as *ast.ParenExpr rather than
// the *ast.SelectorExpr the switch's Begin* case matches — and a walk that did
// not unwrap it first would read this call as neither a selector nor a tracked
// alias and report PASS.
func TestBeginCallersOutsideCatchesAParenthesizedSelectorCall(t *testing.T) {
	const source = `package database

func other(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := (pool.Begin)(ctx)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
`
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, 0)
	if err != nil {
		t.Fatalf("parsing fixture source: %v", err)
	}
	offenders := beginCallersOutside(file, "fixture.go")
	if len(offenders) != 1 || offenders[0] != "fixture.go: other" {
		t.Fatalf("beginCallersOutside(fixture) = %v, want exactly [\"fixture.go: other\"] — "+
			"a transaction opener called through a parenthesized selector must be reported "+
			"the same as one reached through pool.Begin(ctx) directly", offenders)
	}
}

// TestBeginCallersOutsideCatchesAParenthesizedAliasCall is the same gap on the
// alias path: `(begin)(ctx)` parenthesizes the call target around a tracked
// alias identifier, so it parses as *ast.ParenExpr rather than the *ast.Ident
// the switch's alias case matches — and a walk that did not unwrap it first
// would read this call as neither a selector nor a tracked alias and report
// PASS.
func TestBeginCallersOutsideCatchesAParenthesizedAliasCall(t *testing.T) {
	const source = `package database

func other(ctx context.Context, pool *pgxpool.Pool) error {
	begin := pool.Begin
	tx, err := (begin)(ctx)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
`
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, 0)
	if err != nil {
		t.Fatalf("parsing fixture source: %v", err)
	}
	offenders := beginCallersOutside(file, "fixture.go")
	if len(offenders) != 1 || offenders[0] != "fixture.go: other" {
		t.Fatalf("beginCallersOutside(fixture) = %v, want exactly [\"fixture.go: other\"] — "+
			"a transaction opener called through a parenthesized alias must be reported "+
			"the same as one reached through pool.Begin(ctx) directly", offenders)
	}
}

// beginCallersOutside names every function in file that calls a .Begin( other
// than transactionOpener itself.
func beginCallersOutside(file *ast.File, filename string) []string {
	var offenders []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name == transactionOpener || fn.Body == nil {
			continue
		}
		// A Begin* method is an opener whether it is called directly
		// (pool.Begin(ctx)) or captured first as a method VALUE
		// (begin := pool.Begin; begin(ctx)): the second form still turns the
		// pool into a transaction, so every identifier assigned from a
		// Begin* selector is tracked as an opener alias before the calls are
		// walked.
		openerIdents := beginAliases(fn.Body)
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch fun := unwrapParen(call.Fun).(type) {
			case *ast.SelectorExpr:
				// Every spelling pgx offers, not the one this package happens
				// to use today: BeginTx and BeginFunc open exactly the same
				// transaction, so a gate that matched only Begin would report
				// PASS on the two ways around it — and under-recognition is
				// the one way a census must not fail, because it reads a
				// smaller subject and leaves no failing assertion to notice.
				if !strings.HasPrefix(fun.Sel.Name, "Begin") {
					return true
				}
			case *ast.Ident:
				if !openerIdents[fun.Name] {
					return true
				}
			default:
				return true
			}
			offenders = append(offenders, filename+": "+fn.Name.Name)
			return false
		})
	}
	return offenders
}

// beginAliases names every identifier body assigns a Begin* method value to —
// the name `begin := pool.Begin` binds, so a later call through that name is
// recognized as the same opener a direct `pool.Begin(ctx)` would be.
func beginAliases(body *ast.BlockStmt) map[string]bool {
	aliases := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.AssignStmt:
			// begin := pool.Begin (or begin = pool.Begin, reassigning one
			// declared earlier): both are AssignStmt regardless of token.
			for i, rhs := range stmt.Rhs {
				if i >= len(stmt.Lhs) {
					continue
				}
				if ident, ok := stmt.Lhs[i].(*ast.Ident); ok && isBeginSelector(rhs) {
					aliases[ident.Name] = true
				}
			}
		case *ast.ValueSpec:
			// var begin = pool.Begin. This is a DIFFERENT node than the
			// AssignStmt above — a `var` declaration is a GenDecl holding
			// ValueSpecs, never an AssignStmt — so a walk that matched only
			// AssignStmt would miss this spelling of the identical alias and
			// report PASS on a second opener reached through it.
			for i, rhs := range stmt.Values {
				if i >= len(stmt.Names) {
					continue
				}
				if isBeginSelector(rhs) {
					aliases[stmt.Names[i].Name] = true
				}
			}
		}
		return true
	})
	return aliases
}

// unwrapParen strips every enclosing parenthesization from expr —
// `(pool.Begin)` and `((begin))` name the same expression `pool.Begin` and
// `begin` do. A call target normalized through this before the walk matches
// it is what keeps `(pool.Begin)(ctx)` and `(begin)(ctx)` recognized the same
// as their unparenthesized spellings, rather than surviving as *ast.ParenExpr
// and slipping past both the *ast.SelectorExpr and *ast.Ident cases.
func unwrapParen(expr ast.Expr) ast.Expr {
	for {
		paren, ok := expr.(*ast.ParenExpr)
		if !ok {
			return expr
		}
		expr = paren.X
	}
}

// isBeginSelector reports whether expr is a Begin* method value, looking
// through any parenthesization — `(pool.Begin)` is the same alias source as
// `pool.Begin` and must be recognized the same way.
func isBeginSelector(expr ast.Expr) bool {
	sel, ok := unwrapParen(expr).(*ast.SelectorExpr)
	return ok && strings.HasPrefix(sel.Sel.Name, "Begin")
}
