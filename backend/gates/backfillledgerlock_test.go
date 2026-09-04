// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A transaction that writes the backfill creation ledger locks the run row
// first.
//
// The run's `people_created` / `organizations_created` are a PROJECTION over
// `capture_backfill_creation`, refreshed by counting it. At READ COMMITTED two
// recomputes running at once each count a snapshot without the other's
// uncommitted rows, so the later committer can write a total missing the
// earlier one — a silent undercount that no later creation repairs once the
// run has no creations left to make. The lock taken before the inserts is what
// makes the whole write one queue.
//
// ## Why this is a SOURCE gate and not a concurrency test
//
// `TestConcurrentCreationsAreAllCounted` was written to hold this and does
// not: it passes with `FOR UPDATE` removed, which is what raised the question.
// The reason is in the projection's own arithmetic. It writes
// `greatest(counted, existing)`, so the column only ever grows, and with N
// racing writers whichever runs its UPDATE last sees every row committed
// before it plus its own — so the final total is right whether or not anything
// serialised. Losing it needs exactly two writers whose UPDATEs both execute
// before either commits and no writer after them, which the page walk cannot
// arrange: the connector's `afterMessage` hook fires once `sink.Upsert` has
// returned, which is after the transaction has already committed.
//
// So the property is real, its absence is silent, and no behaviour a test can
// stage distinguishes it. What CAN be held is the statement itself.
//
// The corpus is derived from the TABLE rather than from a function name: any
// transaction inserting into the ledger is subject, so a second writer added
// later is covered by construction rather than by somebody remembering to add
// it here.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	ledgerTable = "capture_backfill_creation"
	runTable    = "capture_backfill"
	rowLock     = "FOR UPDATE"
)

// runRowLock is the lock this gate is about: one taken ON THE RUN. Matching
// bare `FOR UPDATE` would accept a writer that locks some other table and
// still report the run protected — capture takes `FOR UPDATE` on
// `capture_import`, `capture_connection` and the pending queue, so that is a
// reachable pass, not a hypothetical one.
//
// `\b` after the table name excludes the ledger's own `capture_backfill_creation`,
// since `_` is a word character. The gap admits no backtick or semicolon, which
// keeps the match inside ONE SQL literal: a run named in one statement cannot
// borrow a `FOR UPDATE` belonging to the next.
var runRowLock = regexp.MustCompile("(?is)\\b" + runTable + "\\b[^;`]*" + rowLock)

func TestEveryWriterOfTheBackfillLedgerLocksTheRunFirst(t *testing.T) {
	t.Parallel()

	writers := functionsWritingTheBackfillLedger(t)
	if len(writers) == 0 {
		t.Fatal("no function inserts into " + ledgerTable + " — either the ledger " +
			"moved and this gate now reads nothing, or the scan is looking in the " +
			"wrong tree. A census that finds no subject cannot report a pass.")
	}
	for name, body := range writers {
		lock := -1
		if at := runRowLock.FindStringIndex(body); at != nil {
			lock = at[0]
		}
		insert := strings.Index(body, "INSERT INTO "+ledgerTable)
		switch {
		case lock < 0:
			t.Errorf("%s writes the backfill ledger and takes no %s on %s. The run's "+
				"counters are a projection over that ledger, so two recomputes at "+
				"READ COMMITTED each count without the other's uncommitted rows and "+
				"the later one writes a total missing the earlier — a number that is "+
				"wrong and looks fine.", name, rowLock, runTable)
		case lock > insert:
			t.Errorf("%s takes the %s AFTER its first insert, so the count and the "+
				"insert are a race rather than one queue. The lock belongs before "+
				"the writes, not between them and the projection.", name, rowLock)
		}
	}
}

// functionsWritingTheBackfillLedger answers every function whose body contains
// an insert into the ledger, by source text.
//
// By text rather than by parsing SQL: the statement is a Go string literal and
// what this gate holds is the ORDER of two statements inside one function,
// which the text carries and an AST of the SQL would not.
func functionsWritingTheBackfillLedger(t *testing.T) map[string]string {
	t.Helper()
	root := filepath.Join(repoRoot, "backend", "internal")
	found := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		source := string(raw)
		// Cheap skip before parsing: most of the tree never names this table.
		if !strings.Contains(source, ledgerTable) {
			return nil
		}
		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, path, source, 0)
		if parseErr != nil {
			return parseErr
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			body := source[fset.Position(fn.Body.Pos()).Offset:fset.Position(fn.Body.End()).Offset]
			if strings.Contains(body, "INSERT INTO "+ledgerTable) {
				found[filepath.Base(path)+":"+fn.Name.Name] = body
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return found
}
