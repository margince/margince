// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// The projection tier's read census. A graph_*_edge row states who talks to
// whom and how often — the exact fact the activity gates protect — so every
// statement reading one must be auth-gated in its own declaration, or be a
// ratified maintenance/privacy site that runs under the system principal
// inside its own transaction.
//
// The corpus is DERIVED from the migrations' DDL, not listed here: the next
// graph_*_edge table joins this census the commit it is created, because a
// census a new table can silently miss has already failed (AGENTS.md rule 8).
// The relationship table has its own, richer census in edgereaders_test.go —
// asserted edges carry a dedicated grant; a projection is gated by the person
// and activity objects it derives from.

import (
	"go/ast"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// projectionEdgeTables reads the migration DDL and names every projection edge
// table. Anchored on CREATE TABLE so a mention in a comment or an index name
// cannot mint a phantom table.
func projectionEdgeTables(t *testing.T) []string {
	t.Helper()
	pattern := regexp.MustCompile(`(?m)^CREATE TABLE (graph_[a-z_]*_edge)\b`)
	entries, err := os.ReadDir(filepath.Join("migrations", "core"))
	if err != nil {
		t.Fatalf("reading the migration directory: %v", err)
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		ddl, err := os.ReadFile(filepath.Join("migrations", "core", entry.Name()))
		if err != nil {
			t.Fatalf("reading %s: %v", entry.Name(), err)
		}
		for _, m := range pattern.FindAllStringSubmatch(string(ddl), -1) {
			seen[m[1]] = true
		}
	}
	var out []string
	for table := range seen {
		out = append(out, table)
	}
	sort.Strings(out)
	return out
}

// projectionGateSpellings are the auth calls that admit a reading site. Any
// one of them in the site's own declaration means the caller was gated or the
// rows were row-scoped before they left the statement.
var projectionGateSpellings = map[string]bool{
	"Require":           true,
	"EnsureVisible":     true,
	"EnsureVisibleLive": true,
	"ScopeClauseFor":    true,
	"EdgeReadScope":     true,
}

// projectionMaintenanceSites are the ratified ungated sites, each with the
// reason it may read the table without a caller gate. Every entry must match a
// real site — a stale one fails the census the day its subject moves.
var projectionMaintenanceSites = gatekit.Waive(map[string]string{
	"internal/modules/search:recomputePairs":                 "the fold runs under the system principal from the cg:graph-edge consumer; it rewrites the projection from the base tables and returns nothing to a caller",
	"internal/modules/search:recomputeContactPairs":          "the contact fold, same contract as recomputePairs: system principal, rewrites from base tables, returns nothing",
	"internal/modules/search:affectedContactPairs":           "resolves which pairs an activity touches for the fold — including stale edges of a relinked participant; the keys never leave the maintenance path",
	"internal/modules/search:contactPairsForPerson":          "resolves a person's pairs for the fold on merge/archive; the keys never leave the maintenance path",
	"internal/compose/org360:contactRoutes":                  "gated by its one caller: contacts.go asks mayReadRoutes (activity read — the projection derives from activity) before calling, and a refusal omits the routes rather than reaching this read",
	"internal/compose/org360:readInContactWith":              "gated by its one caller: readOurSide requires person and activity read before calling, and only already-drawn contact ids reach it",
	"internal/modules/search:RecomputeEdgesForPerson":        "maintenance entry point: re-folds one contact's edges under the system principal after a merge, archive or restore",
	"internal/modules/search:DropEdgesForPerson":             "the erasure drop: deletes, returns nothing",
	"internal/modules/search:RebuildEdges":                   "the corruption remedy: wholesale replace under the system principal",
	"internal/modules/search:rebuildContactEdges":            "RebuildEdges' contact half, same contract",
	"internal/modules/privacy:deleteSubjectInteractionEdges": "the Art. 17 erasure delete inside the erasure transaction; deletes on both endpoint columns and returns nothing",
	"internal/modules/privacy:scrubPersonGraphTraces":        "the nightly retention delete inside the per-record transaction; deletes and returns nothing",
})

func TestEveryProjectionEdgeReaderIsAuthGatedOrRatified(t *testing.T) {
	t.Parallel()
	tables := projectionEdgeTables(t)
	// The floor: the two tables known today. Fewer means the DDL scan rotted,
	// and a census that reads a smaller tree reports PASS with nothing failing.
	if len(tables) < 2 {
		t.Fatalf("the migration scan found %d projection edge tables (%v), want at least graph_contact_edge and graph_interaction_edge — the corpus derivation has rotted", len(tables), tables)
	}

	for _, table := range tables {
		// Wider than gatekit.TableReadPattern on purpose: FROM/JOIN alone
		// misses an UPDATE … RETURNING or INSERT … ON CONFLICT … RETURNING,
		// which is a read on the replay/conflict path — exactly the shape a
		// later author adds without thinking of this census.
		pattern := regexp.MustCompile(`(?i)\b(FROM|JOIN|UPDATE|INTO)\s+` + regexp.QuoteMeta(table) + `(\s|$|[,;)])`)
		scope := gatekit.Scope{Roots: []string{"internal"}, Subject: func(path string, file *ast.File) bool {
			return gatekit.FileReadsTable(path, file, pattern)
		}}
		for _, parsed := range scope.Files(t) {
			for _, decl := range parsed.File.Decls {
				reads := gatekit.DeclReads(decl, pattern)
				if len(reads) == 0 {
					continue
				}
				fn := reads[0].Function
				key := filepath.Dir(parsed.Path) + ":" + fn
				if projectionMaintenanceSites.Waived(t, key) {
					continue
				}
				if declCallsProjectionGate(decl) {
					continue
				}
				t.Errorf("%s: %s reads %s with no auth gate in its own declaration and no ratified maintenance entry — gate the caller (auth.Require / EnsureVisibleLive) or scope the rows (auth.ScopeClauseFor), or ratify it in projectionMaintenanceSites with the reason it needs neither.\n  SQL: %s",
					parsed.Path, fn, table, gatekit.FirstLineOf(reads[0].SQL))
			}
		}
	}
	projectionMaintenanceSites.AssertAllMatched(t)
}

// declCallsProjectionGate reports whether the declaration itself calls one of
// the admitting auth spellings. Deliberately not transitive: a projection read
// is small enough to hold its gate in hand, and a package-wide fixpoint would
// let a gate in a neighbouring function vouch for a reader it never guards.
func declCallsProjectionGate(decl ast.Decl) bool {
	found := false
	ast.Inspect(decl, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if !isCall {
			return true
		}
		selector, isSelector := call.Fun.(*ast.SelectorExpr)
		if !isSelector {
			return true
		}
		pkg, isIdent := selector.X.(*ast.Ident)
		if isIdent && pkg.Name == "auth" && projectionGateSpellings[selector.Sel.Name] {
			found = true
		}
		return !found
	})
	return found
}
