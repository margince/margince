// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// Two lists say which tables a wipe must leave alone, and this fails when they
// disagree.
//
// One is testdb.PreservedReferenceTables, which the integration harness spares
// between tests. The other is the sweep exemption map in compose/datasweep.go,
// which the PRODUCT's own workspace reset spares. They are different mechanisms
// answering the same question — is this table an installation's reference data
// or a workspace's records — and they have to answer it the same way.
//
// Registering in only one is the failure this exists for, and it is invisible
// where it is cheap to see. A table spared by the harness and swept by the
// product passes every test run on its own and fails only once the shard runs
// the reset test before the tests that need the rows: the rows vanish
// mid-shard, every later test reports something unrelated, and nothing points
// at the list that was short. That is exactly what a new reference table did
// here, and it reached main.
//
// WHAT THIS DOES NOT SAY: that either list is CORRECT. A table wrongly spared
// by both is spared by both, and this reports agreement. The lists are a
// decision about what a reset means, and a gate cannot make that decision —
// it can only stop the two halves of it drifting apart.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	harnessList = "internal/platform/testdb/reset.go"
	sweepList   = "internal/compose/datasweep.go"
)

// harnessOnly names tables the TEST harness spares that the product sweep does
// not, with the reason each is not a product concern. Every entry is a
// deliberate asymmetry, written down so the next one has to be argued for
// rather than assumed.
var harnessOnly = gatekit.Waive(map[string]string{
	"field_mask": "role-derived read masks. The product reset SWEEPS it on purpose — a " +
		"workspace's masks are that workspace's configuration, and a reset clears configuration " +
		"back to its default — while the harness spares it so a test's masks do not vanish from " +
		"under it between cases. The asymmetry is the right one in both directions",
})

func TestTheTwoPreservedTableListsAgree(t *testing.T) {
	t.Parallel()

	harness := preservedInHarness(t)
	sweep := preservedInSweep(t)

	// A census that read nothing agrees with everything. Both lists hold a
	// dozen-odd entries, so a handful means the parse broke rather than the
	// lists shrank.
	if len(harness) < 8 {
		t.Fatalf("read %d tables out of %s — the parse is not reaching the list, and a green "+
			"result over a list it could not read means nothing", len(harness), harnessList)
	}
	if len(sweep) < 8 {
		t.Fatalf("read %d tables out of %s — same problem, other side", len(sweep), sweepList)
	}

	var missingFromSweep []string
	for table := range harness {
		if sweep[table] {
			continue
		}
		if harnessOnly.Waived(t, table) {
			continue
		}
		missingFromSweep = append(missingFromSweep, table)
	}
	// Swept for stale entries AFTER the loop that consults it: an entry that
	// matched nothing reads as ratification of a table that no longer exists,
	// and the sweep can only tell that once every subject has been offered.
	harnessOnly.AssertAllMatched(t)

	sort.Strings(missingFromSweep)
	for _, table := range missingFromSweep {
		t.Errorf("%s is spared by the test harness and swept by the product reset.\n"+
			"Add it to the sweep exemption map in %s, or record it in harnessOnly with the reason "+
			"it is not a product concern. Left as it is, the rows survive every test run alone and "+
			"vanish mid-shard the moment the reset test runs first — and every later failure points "+
			"somewhere else.", table, sweepList)
	}
}

// THE OTHER DIRECTION IS NOT CHECKED, and that is a decision rather than an
// omission.
//
// The product sweep spares a great deal the harness must not: app_user,
// role_assignment, passport, audit_log, event_outbox and a dozen more. A reset
// wipes a workspace's RECORDS and leaves the installation standing, so the
// seats and the permissions survive it. A test clone has the opposite
// obligation — one case's users and audit rows must not reach the next — so the
// harness empties exactly those.
//
// The asymmetry is therefore the normal case in that direction and would be a
// list of a dozen permanent waivers saying "yes, still normal", which is a list
// nobody reads. What this gate holds is the direction where asymmetry is
// almost always a mistake: a table the HARNESS keeps and the product empties is
// seeded reference data somebody registered once and forgot to register twice.

// preservedInHarness reads the table names out of the harness's SQL literal.
func preservedInHarness(t *testing.T) map[string]bool {
	t.Helper()
	src := string(readGate(t, harnessList))
	const marker = "PreservedReferenceTables = "
	at := strings.Index(src, marker)
	if at < 0 {
		t.Fatalf("%s no longer declares PreservedReferenceTables — this gate reads that name", harnessList)
	}
	// The literal is a parenthesised SQL list built from concatenated backtick
	// strings. Everything between the first '(' and the matching ')' is the
	// list; the quoting inside is SQL's, so the names come out between single
	// quotes.
	rest := src[at:]
	open := strings.Index(rest, "(")
	closes := strings.Index(rest, ")")
	if open < 0 || closes < open {
		t.Fatalf("cannot find the preserved-table list's parentheses in %s", harnessList)
	}
	// SQL single quotes delimit the names, so every ODD field of the split is
	// one. Taking every field instead would collect the separators too, and a
	// stray "(" would then read as a table this gate reports on.
	out := map[string]bool{}
	fields := strings.Split(rest[open:closes], "'")
	for i := 1; i < len(fields); i += 2 {
		if name := strings.TrimSpace(fields[i]); name != "" {
			out[name] = true
		}
	}
	return out
}

// preservedInSweep reads the keys of the sweep's exemption map out of the AST.
func preservedInSweep(t *testing.T) map[string]bool {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), sweepList, readGate(t, sweepList),
		parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s: %v", sweepList, err)
	}
	out := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		literal, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		mapType, ok := literal.Type.(*ast.MapType)
		if !ok {
			return true
		}
		// The one map[string]bool in this file is the exemption set.
		if key, ok := mapType.Key.(*ast.Ident); !ok || key.Name != "string" {
			return true
		}
		if value, ok := mapType.Value.(*ast.Ident); !ok || value.Name != "bool" {
			return true
		}
		for _, element := range literal.Elts {
			pair, ok := element.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			name, ok := pair.Key.(*ast.BasicLit)
			if !ok || name.Kind != token.STRING {
				continue
			}
			unquoted, err := strconv.Unquote(name.Value)
			if err != nil {
				continue
			}
			// The VALUE decides, not the key's presence. `"t": false` is an
			// entry saying the table is NOT spared, and reading it as spared
			// would report agreement over a table the sweep actually empties.
			spared, ok := pair.Value.(*ast.Ident)
			if !ok || spared.Name != "true" {
				continue
			}
			out[unquoted] = true
		}
		return true
	})
	return out
}

// readGate reads one tracked source file, failing loudly rather than letting a
// missing file read as an empty list.
func readGate(t *testing.T, path string) []byte {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return src
}
