// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package backendarch

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
// literal, so a statement assembled from fragments that never spell
// "INSERT INTO voice_profile_version" in one piece would escape. Every write
// in this tree spells it whole, and the honest cost is stated rather than
// implied.

import (
	"go/ast"
	"go/token"
	"regexp"
	"strings"
	"testing"

	"github.com/gradionhq/margince/backend/internal/shared/gatekit"
)

// voiceWriteTables are the two tables the version writer owns. Named as data
// so the two censuses below are one walk asked twice rather than two walks
// that could drift.
var voiceWriteTables = []string{"voice_profile_version", "voice_profile_delta"}

func TestVoiceVersionsHaveOneWriter(t *testing.T) {
	for _, table := range voiceWriteTables {
		t.Run(table, func(t *testing.T) {
			scope := gatekit.Scope{
				Roots:   []string{"internal/modules/ai"},
				Subject: insertsInto(table),
				Exempt:  gatekit.Waive(map[string]string{}),
			}
			inside := scope.Files(t)
			if len(inside) > 1 {
				var where []string
				for _, f := range inside {
					where = append(where, f.Path)
				}
				t.Errorf("%s is written from %d files:\n\t%s\n\nOne writer owns the column list, so a column "+
					"added to the table is a field nobody fills rather than a default nobody notices",
					table, len(inside), strings.Join(where, "\n\t"))
			}
		})
	}
}

// insertsInto builds the subject predicate for one table.
func insertsInto(table string) func(string, *ast.File) bool {
	pattern := regexp.MustCompile(`(?is)INSERT\s+INTO\s+` + regexp.QuoteMeta(table) + `\b`)
	return func(_ string, file *ast.File) bool {
		found := false
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING || !pattern.MatchString(lit.Value) {
				return !found
			}
			found = true
			return false
		})
		return found
	}
}

// TestTheWriterCensusStillSeesItsSubject is the vacuity check: a census that
// stops matching passes by finding nothing, which is the same word it prints
// over a clean tree.
func TestTheWriterCensusStillSeesItsSubject(t *testing.T) {
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
	}
}
