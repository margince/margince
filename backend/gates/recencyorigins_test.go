// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates_test

// Every reading of "when was this record last touched" excludes the origins the
// system wrote itself.
//
// Two activity origins are the product writing rather than a person:
// system_remediation is work filed ABOUT a record (a forecast-assurance review
// task) and system_notice is a message the installation owes somebody (the
// confirm-details link). Neither means a buyer engaged.
//
// The failure this gate exists for is silent and has already happened once in
// shape: a recency query that counts the system's own writing refreshes the very
// record whose silence prompted the write, so the rule that noticed the silence
// switches itself off — one record at a time, with nothing failing. Migration
// 1788386600 made that argument for remediation; adding system_notice to the
// vocabulary without reaching every reader would re-open it for notice mail.
//
// So the obligation is: a Go query that filters on activity origin uses the ONE
// spelling, auth.OriginIsEngagement, rather than naming an origin itself. The
// SQL helpers (last_activity_of_*) carry the same clause and are held by the
// head catalog, which records their bodies verbatim.
//
// A CENSUS rather than a prohibition: the corpus is every file naming an origin
// literal, and the finding is one that spells the exclusion by hand.

import (
	"go/ast"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// systemOrigins are the origins that mean the product wrote the row itself.
var systemOrigins = []string{"system_remediation", "system_notice"}

// ratifiedOriginLiterals are the files allowed to name a system origin in a
// literal, each with the reason it is not a hand-rolled exclusion.
//
// Keyed by file, because a file is what a reader opens. AssertAllMatched reports
// an entry that has gone stale, so a ratification outlives its statement by one
// test run rather than forever.
var ratifiedOriginLiterals = gatekit.Waive(map[string]string{
	"internal/platform/auth/engagementorigin.go": "The one spelling itself. This is the file " +
		"every other reader is required to call instead of naming an origin, so it is the " +
		"single place the literals are correct.",
	"internal/modules/activities/activity.go": "The vocabulary's own declaration — the Origin* " +
		"constants an activity is written with. Naming a value is not filtering on it.",
})

// TestEveryRecencyReadingExcludesTheSystemOrigins reads every Go file in the
// tree for a hand-written exclusion.
func TestEveryRecencyReadingExcludesTheSystemOrigins(t *testing.T) {
	scope := gatekit.Scope{
		Roots: []string{"internal"},
		Subject: func(_ string, file *ast.File) bool {
			return fileNamesASystemOrigin(file)
		},
		Exempt: ratifiedOriginLiterals,
	}
	subjects := scope.Files(t)
	for _, parsed := range subjects {
		if ratifiedOriginLiterals.Waived(t, parsed.Path) {
			continue
		}
		t.Errorf("%s names a system activity origin in a SQL literal. A recency reading that "+
			"spells its own exclusion goes stale the moment a third system origin is added: it "+
			"keeps compiling, keeps passing, and starts counting the product's own writing as "+
			"the other side engaging. Call auth.OriginIsEngagement(alias) instead, which is the "+
			"one spelling all five readers share.", parsed.Path)
	}
	if len(subjects) == 0 {
		// Under-recognition is the one way this gate must not fail. A matcher
		// that stopped recognising an origin literal would read a tree where
		// nothing matches, report PASS, and leave no failing assertion behind.
		// The ratified files ARE subjects, so an empty sweep is the bug.
		t.Fatal("no file in the tree names a system activity origin, not even the two ratified " +
			"ones — the literal matcher has stopped recognising what it is looking for")
	}
	ratifiedOriginLiterals.AssertAllMatched(t)
}

// TestTheOneSpellingExcludesEverySystemOrigin holds the fragment itself to the
// vocabulary, so a new origin cannot be added to the constants and missed here.
//
// Without it the gate above would be satisfied by a single spelling that had
// silently fallen behind the origins it is supposed to exclude — every reader
// correctly calling one function that answers the wrong question.
func TestTheOneSpellingExcludesEverySystemOrigin(t *testing.T) {
	clause := auth.OriginIsEngagement("a")
	for _, origin := range systemOrigins {
		if !strings.Contains(clause, origin) {
			t.Errorf("auth.OriginIsEngagement does not exclude %q, so every recency reading in "+
				"the product counts it as somebody engaging. Add it to the fragment.", origin)
		}
	}
	if !strings.Contains(clause, "a.origin") {
		t.Errorf("auth.OriginIsEngagement(%q) = %q, which does not filter on the alias's origin "+
			"column; a caller concatenating it would silently filter nothing", "a", clause)
	}
}

// fileNamesASystemOrigin reports whether any literal in the file spells one of
// the system origins.
func fileNamesASystemOrigin(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		expr, ok := n.(ast.Expr)
		if !ok {
			return true
		}
		text, ok := gatekit.LiteralText(expr)
		if !ok {
			return true
		}
		for _, origin := range systemOrigins {
			if strings.Contains(text, origin) {
				found = true
			}
		}
		return true
	})
	return found
}
