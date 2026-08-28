// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// voice_profile_version and voice_profile_delta each have ONE writer.
//
// They had three apiece — a build, a rollback, and the manual activation of a
// derived artifact — hand-copying a twenty-column INSERT between them. Nothing
// compared the three column lists, so a column added to the table could be
// added to two of them and the package still compiled. One of the three left
// review_reasons out entirely, and because that column is NOT NULL DEFAULT
// '{}' the omission was silent: a manually activated version read back
// indistinguishable from one with genuinely no review reasons.
//
// A single writer is what makes the column census next door meaningful.
// TestTheVoiceVersionWriterCoversEveryColumnACallerChooses compares one
// writer's column list against information_schema; a second writer, anywhere,
// would be outside everything that checks — so uniqueness is the load-bearing
// half and this is where it is held.
//
// WHAT THIS GATE CAN AND CANNOT SEE. It matches the INSERT in a Go string
// literal, and in a `+` chain whose literal parts spell it between them — the
// one that would otherwise escape is exactly the one a second writer reaches
// for, because the first thing an author does to a copied statement is
// parameterise a piece of it. What it still cannot see is a statement whose
// verb and table arrive through a variable at run time. Nothing in this tree
// builds SQL that way, and the honest cost is stated rather than implied.

import (
	"fmt"
	"go/ast"
	"go/token"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// voiceWriteTables are the two tables the version writer owns. Named as data
// so the two censuses below are one walk asked twice rather than two walks
// that could drift.
var voiceWriteTables = []string{"voice_profile_version", "voice_profile_delta"}

func TestVoiceVersionsHaveOneWriter(t *testing.T) {
	t.Parallel()
	for _, table := range voiceWriteTables {
		t.Run(table, func(t *testing.T) {
			scope := gatekit.Scope{
				Roots:   []string{"internal/modules/ai"},
				Subject: insertsInto(table),
				Exempt:  gatekit.Waive(map[string]string{}),
			}
			count := insertCounter(table)
			total := 0
			var where []string
			for _, f := range scope.Files(t) {
				n := count(f.File)
				total += n
				where = append(where, fmt.Sprintf("%s (%d)", f.Path, n))
			}
			// EXACTLY one, not at most one. Zero is the direction a census
			// fails silently in: the writer moved out of this scope, or the
			// statement stopped being spelled whole, and a gate reading "no
			// second writer" prints the same clean word it prints over a tree
			// that is genuinely fine.
			if total != 1 {
				sites := "(no file under internal/modules/ai holds one)"
				if len(where) > 0 {
					sites = strings.Join(where, "\n\t")
				}
				t.Errorf("%s is written by %d INSERTs:\n\t%s\n\nOne writer owns the column list, so a column "+
					"added to the table is a field nobody fills rather than a default nobody notices",
					table, total, sites)
			}
		})
	}
}

// insertsInto builds the subject predicate for one table. It is the counter
// below asked whether the answer is nonzero, so the sweep that locates the
// writers and the count that judges them cannot come to differ about what an
// INSERT is.
func insertsInto(table string) func(string, *ast.File) bool {
	count := insertCounter(table)
	return func(_ string, file *ast.File) bool { return count(file) > 0 }
}

// insertCounter counts the INSERTs into one table in a file.
//
// It counts STATEMENTS, not files and not literals. A file is the wrong unit:
// two INSERTs into the same table in one file are two writers of the column
// list, and a gate that reported the file once would let the second in
// silently. A literal is the wrong unit for the same reason one step down —
// nothing stops a caller putting two statements in one raw string.
func insertCounter(table string) func(*ast.File) int {
	pattern := regexp.MustCompile(`(?is)INSERT\s+INTO\s+` + regexp.QuoteMeta(table) + `\b`)
	return func(file *ast.File) int {
		total := 0
		// A `+` chain is nested: `a + b + c` is a BinaryExpr holding another,
		// and joining both would count one statement twice. Only the OUTERMOST
		// chain is read, and ast.Inspect reaches it first.
		inner := map[ast.Node]bool{}
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.BasicLit:
				if node.Kind == token.STRING {
					total += len(pattern.FindAllString(gatekit.TextOf(node), -1))
				}
			case *ast.BinaryExpr:
				if inner[node] {
					return true
				}
				markNestedChains(node, inner)
				// SQL assembled by concatenation, where no single literal
				// holds the whole verb. Joining the chain's literal parts is
				// what keeps `"INSERT INTO " + table + " (a) VALUES ($1)"`
				// from being a writer the census cannot see. The chain's own
				// literals are visited too, so a statement spelled whole
				// inside one of them would be counted twice — which is why
				// only chains that do NOT already hold the match are joined.
				if node.Op == token.ADD && !holdsWholeMatch(node, pattern) {
					total += len(pattern.FindAllString(literalsOf(node), -1))
				}
			}
			return true
		})
		return total
	}
}

// markNestedChains records every `+` expression BELOW this one, so the chain is
// read once from its top rather than once per level.
func markNestedChains(top *ast.BinaryExpr, inner map[ast.Node]bool) {
	ast.Inspect(top, func(n ast.Node) bool {
		if nested, ok := n.(*ast.BinaryExpr); ok && nested != top {
			inner[nested] = true
		}
		return true
	})
}

// literalsOf concatenates the string literals of an expression, dropping the
// non-literal operands. What is dropped is a table alias or a format verb, and
// the words on either side of it are what the pattern reads.
func literalsOf(expr ast.Expr) string {
	var parts []string
	ast.Inspect(expr, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			parts = append(parts, gatekit.TextOf(lit))
		}
		return true
	})
	return strings.Join(parts, "")
}

// holdsWholeMatch reports whether some single literal in the expression
// already spells the match, in which case the BasicLit arm has counted it.
func holdsWholeMatch(expr ast.Expr, pattern *regexp.Regexp) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if ok && lit.Kind == token.STRING && pattern.MatchString(gatekit.TextOf(lit)) {
			found = true
		}
		return !found
	})
	return found
}

// TestTheWriterCensusStillSeesItsSubject is the vacuity check: a census that
// stops matching passes by finding nothing, which is the same word it prints
// over a clean tree.
func TestTheWriterCensusStillSeesItsSubject(t *testing.T) {
	t.Parallel()
	for _, table := range voiceWriteTables {
		subject := insertsInto(table)
		written := parseGateFixture(t, "package p\nfunc f() { q(`\n\t\tINSERT INTO "+table+
			"\n\t\t  (voice_profile_id)\n\t\tVALUES ($1)`) }")
		if !subject("x.go", written) {
			t.Errorf("the census no longer recognises an INSERT into %s, so it is guarding nothing", table)
		}
		read := parseGateFixture(t, "package p\nfunc f() { q(`SELECT id FROM "+table+" WHERE id = $1`) }")
		if subject("x.go", read) {
			t.Errorf("the census counts a READ of %s as a writer", table)
		}
		// The unit is the statement. One file holding two INSERTs is the shape
		// a file-counting gate reports as one writer and waves through.
		twice := parseGateFixture(t, "package p\nfunc f() { q(`INSERT INTO "+table+" (a) VALUES ($1)`)\n"+
			"\tq(`INSERT INTO "+table+" (b) VALUES ($1)`) }")
		if got := insertCounter(table)(twice); got != 2 {
			t.Errorf("two INSERTs into %s in one file count as %d; the census is counting files, so a second "+
				"writer added beside the first is invisible", table, got)
		}
		// And two statements inside ONE literal are still two.
		joined := parseGateFixture(t, "package p\nfunc f() { q(`INSERT INTO "+table+" (a) VALUES ($1); "+
			"INSERT INTO "+table+" (b) VALUES ($2)`) }")
		if got := insertCounter(table)(joined); got != 2 {
			t.Errorf("two INSERTs into %s in one literal count as %d", table, got)
		}
		// A statement no single literal holds. Parameterising a piece of a
		// copied INSERT is the first thing an author does to it, so this is
		// the shape a second writer most plausibly arrives in.
		assembled := parseGateFixture(t, "package p\nfunc f() { q(`INSERT INTO `+prefix+`"+table+
			"` + ` (a) VALUES ($1)`) }")
		if got := insertCounter(table)(assembled); got != 1 {
			t.Errorf("an INSERT into %s assembled from fragments counts as %d; a second writer would only "+
				"have to break the verb across a `+` to be invisible", table, got)
		}
		// A chain whose own literal already spells the statement is ONE
		// writer, not two: the chain and the literal inside it must not both
		// be counted.
		trailing := parseGateFixture(t, "package p\nfunc f() { q(`INSERT INTO "+table+
			" (a) VALUES ($1)` + suffix) }")
		if got := insertCounter(table)(trailing); got != 1 {
			t.Errorf("one INSERT into %s with a trailing fragment counts as %d", table, got)
		}
	}
}
