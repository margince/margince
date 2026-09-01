// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// CurrentPrimarySlotSQL mirrors uq_rel_current_primary_employer, and this is
// what holds it to the index rather than to somebody's memory of the index.
//
// It lives beside the helper and not with the census in gates because
// the root package may not import a module (ADR-0054): the census reads the
// tree as text and needs no import, while a mirror has to CALL the function.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// slotPredicateName is what the failure report calls the function it is about.
const slotPredicateName = "CurrentPrimarySlotSQL"

// headCatalog is the generated shape of a freshly migrated database — the
// index's own text, and therefore the only statement of what the slot
// predicate IS. Deriving the expectation from it rather than restating the
// predicate here is what keeps this gate from becoming a second copy of its
// own subject: a migration that narrows the index fails this test instead of
// leaving the helper quietly wrong.
const headCatalog = "../../../migrations/testdata/head_catalog.txt"

// slotIndex is the unique index the helper exists to satisfy.
const slotIndex = "uq_rel_current_primary_employer"

// indexPredicate lifts the WHERE clause off a catalog line.
var indexPredicate = regexp.MustCompile(`\sWHERE\s+(.*)$`)

// printedSQLNoise is what Postgres prints that a hand-written predicate does
// not: the parentheses it re-adds around every conjunct and the cast it
// resolves a literal to. Removing it compares the two as PREDICATES rather than
// as text — a text comparison loses to an equivalent spelling, and this one has
// two.
//
// Dropping parentheses is only sound while the predicate is a flat conjunction,
// which is why the comparison below refuses to run over an OR rather than
// quietly answering: `A AND (B OR C)` and `(A AND B) OR C` reduce to the same
// text once the brackets are gone, so a mirror that kept going could pass a
// helper that asks a different question.
var printedSQLNoise = regexp.MustCompile(`::text|[()]`)

// disjunction is the shape this comparison cannot judge.
var disjunction = regexp.MustCompile(`(?i)\bOR\b`)

func TestTheCurrentPrimarySlotPredicateMirrorsItsIndex(t *testing.T) {
	catalog, err := os.ReadFile(headCatalog)
	if err != nil {
		t.Fatalf("reading %s: %v", headCatalog, err)
	}
	var predicate string
	for _, line := range strings.Split(string(catalog), "\n") {
		if !strings.Contains(line, slotIndex) || !strings.Contains(line, "CREATE UNIQUE INDEX") {
			continue
		}
		match := indexPredicate.FindStringSubmatch(line)
		if match == nil {
			t.Fatalf("%s carries no WHERE clause in %s, so it is no longer a partial index and this "+
				"helper has nothing to mirror:\n%s", slotIndex, headCatalog, line)
		}
		predicate = match[1]
	}
	if predicate == "" {
		t.Fatalf("%s names no %s, so either the index is gone or the catalog is not what it was: "+
			"this helper mirrors a predicate that has to exist", headCatalog, slotIndex)
	}
	if disjunction.MatchString(predicate) {
		t.Fatalf("%s now carries an OR (%s), and this comparison strips parentheses — it cannot tell "+
			"`A AND (B OR C)` from `(A AND B) OR C`. Compare the predicates structurally before trusting it again",
			slotIndex, predicate)
	}
	if want, got := normalizedPredicate(predicate), normalizedPredicate(CurrentPrimarySlotSQL("")); want != got {
		t.Errorf("%s renders %q, but %s is %q.\n\n"+
			"The helper IS that index's predicate. A guard that asks a narrower question skips a write "+
			"the index then refuses with a 409; a wider one skips a write the index would have accepted.",
			slotPredicateName, got, slotIndex, want)
	}
	// The aliased form is the same predicate with every column qualified, and
	// nothing else — the shape four of the six call sites need.
	if want, got := "b.", CurrentPrimarySlotSQL("b"); strings.Count(got, want) != 3 {
		t.Errorf("%s(%q) = %q, want every one of the three columns qualified", slotPredicateName, "b", got)
	}
}

// normalizedPredicate reduces a predicate to what it asks: no Postgres
// re-parenthesisation, no resolved cast, and one space between tokens.
//
// Case is NOT folded, deliberately. The catalog and the helper both spell SQL
// keywords upper-case, and folding here would hide a helper that stopped doing
// so — the mirror is also what a reader greps for when they want the predicate.
func normalizedPredicate(sql string) string {
	return strings.Join(strings.Fields(printedSQLNoise.ReplaceAllString(sql, "")), " ")
}

// liveEmploymentIndex is the OTHER unique index a plant has to satisfy: one
// live employment per person per company, whatever the primary slot says.
const liveEmploymentIndex = "uq_rel_employment"

// LiveEmploymentSlotSQL mirrors that index, and this holds it there for the
// reason the primary-slot mirror exists: a guard that asks a NARROWER question
// than the index offers work the insert then drops on conflict, which is how a
// sweep comes to return the same rows on every pass for ever.
func TestTheLiveEmploymentSlotPredicateMirrorsItsIndex(t *testing.T) {
	catalog, err := os.ReadFile(headCatalog)
	if err != nil {
		t.Fatalf("reading %s: %v", headCatalog, err)
	}
	var predicate string
	for _, line := range strings.Split(string(catalog), "\n") {
		if !strings.Contains(line, liveEmploymentIndex) || !strings.Contains(line, "CREATE UNIQUE INDEX") {
			continue
		}
		match := indexPredicate.FindStringSubmatch(line)
		if match == nil {
			t.Fatalf("%s carries no WHERE clause in %s, so it is no longer a partial index and this "+
				"helper has nothing to mirror:\n%s", liveEmploymentIndex, headCatalog, line)
		}
		predicate = match[1]
	}
	if predicate == "" {
		t.Fatalf("%s names no %s, so either the index is gone or the catalog is not what it was: "+
			"this helper mirrors a predicate that has to exist", headCatalog, liveEmploymentIndex)
	}
	if disjunction.MatchString(predicate) {
		t.Fatalf("%s now carries an OR (%s), and this comparison strips parentheses — compare the "+
			"predicates structurally before trusting it again", liveEmploymentIndex, predicate)
	}
	if want, got := normalizedPredicate(predicate), normalizedPredicate(LiveEmploymentSlotSQL("")); want != got {
		t.Errorf("LiveEmploymentSlotSQL renders %q, but %s is %q.\n\n"+
			"The helper IS that index's predicate. A guard narrower than the index offers a write the "+
			"index refuses, and the caller that keeps offering it never drains.", got, liveEmploymentIndex, want)
	}
	if want, got := "held.", LiveEmploymentSlotSQL("held"); strings.Count(got, want) != 3 {
		t.Errorf("LiveEmploymentSlotSQL(%q) = %q, want every one of the three columns qualified", "held", got)
	}
}
