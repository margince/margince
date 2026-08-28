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
// DB.Tx — routes through it, which is what makes "a domain row commits with its
// audit row and its outbox row" a property held in a single place.
const transactionOpener = "runTx"

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

// beginCallersOutside names every function in file that calls a .Begin( other
// than transactionOpener itself.
func beginCallersOutside(file *ast.File, filename string) []string {
	var offenders []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name == transactionOpener || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			// Every spelling pgx offers, not the one this package happens to
			// use today: BeginTx and BeginFunc open exactly the same
			// transaction, so a gate that matched only Begin would report PASS
			// on the two ways around it — and under-recognition is the one way
			// a census must not fail, because it reads a smaller subject and
			// leaves no failing assertion to notice.
			if !ok || !strings.HasPrefix(sel.Sel.Name, "Begin") {
				return true
			}
			offenders = append(offenders, filename+": "+fn.Name.Name)
			return false
		})
	}
	return offenders
}
