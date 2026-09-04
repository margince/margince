// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The outbound category vocabulary is spelled TWICE — once in Go, once as a
// CHECK constraint on communication_decision — and the two must agree.
//
// The failure this exists to stop is silent and one-directional. A category
// added to the Go vocabulary and forgotten in the constraint passes every
// compile, every unit test and every review: the engine resolves it happily,
// and the INSERT that records the decision then fails at runtime, inside the
// send transaction, on the message that used it. A resolution the database
// refuses to store is a send nobody can explain afterwards, which is the whole
// property communication_decision exists to provide.
//
// The reverse — a value in the constraint the Go vocabulary does not know — is
// the milder half and is checked too, because it means the constraint is
// admitting something no code can produce, and the next reader takes that
// permission for a specification.
//
// It reads the SHIPPED migration rather than a live schema. The constraint is
// additive-only under the migration rule, so the file that created it is the
// file that still defines it, and a gate needing a database would not run in
// the unit lane where a vocabulary edit is actually made.
//
// What this gate deliberately does NOT assert: that every category has a
// validator arm. Nine of the fourteen resolve to `default:` in
// consent/authorizevalidators.go and stay unsupported on purpose — they have no
// record evidence to read, so they fall through to the legacy verdict. That is
// a documented design choice, and a gate demanding otherwise would fail against
// correct code.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// decisionMigration is the migration that created communication_decision and
// its CHECK constraints. Named rather than globbed: a glob that matched nothing
// would report PASS, which is the shape of under-recognition this repository
// treats as a failure in itself.
const decisionMigration = "1788407500_a_send_records_why_it_was_allowed.up.sql"

// categoryConstraint captures the ARRAY body of the resolved_category CHECK.
var categoryConstraint = regexp.MustCompile(
	`(?s)CONSTRAINT communication_decision_category\s*CHECK \(resolved_category = ANY \(ARRAY\[(.*?)\]\)\)`)

// quotedLiteral picks each 'value'::text out of a constraint body.
var quotedLiteral = regexp.MustCompile(`'([a-z_]+)'::text`)

func TestTheCategoryVocabularyAgreesWithItsCheckConstraint(t *testing.T) {
	t.Parallel()

	sql := readDecisionMigration(t)
	body := categoryConstraint.FindStringSubmatch(sql)
	if body == nil {
		// A renamed constraint is not a reason to pass. If this fires after a
		// deliberate rename, repoint the pattern — never delete the assertion.
		t.Fatalf("no communication_decision_category CHECK found in %s: the constraint this gate "+
			"holds against the Go vocabulary has moved or been renamed", decisionMigration)
	}

	inSQL := map[string]bool{}
	for _, m := range quotedLiteral.FindAllStringSubmatch(body[1], -1) {
		inSQL[m[1]] = true
	}
	if len(inSQL) == 0 {
		t.Fatalf("the %s constraint body parsed to no values — the gate is reading the wrong text",
			decisionMigration)
	}

	inGo := map[string]bool{}
	for _, c := range commsauthz.Categories() {
		inGo[string(c)] = true
	}

	for _, missing := range difference(inGo, inSQL) {
		t.Errorf("category %q is in the Go vocabulary but not in the CHECK constraint: "+
			"the engine can resolve it and the INSERT recording that decision will fail "+
			"inside the send transaction", missing)
	}
	for _, extra := range difference(inSQL, inGo) {
		t.Errorf("category %q is admitted by the CHECK constraint but is not a Category: "+
			"the constraint permits a value nothing can produce", extra)
	}
}

// TestEveryCategoryCarriesADistinctWireValue holds the property the constraint
// comparison above silently depends on.
//
// Categories() is built from a map keyed by the Category values themselves, so
// two constants sharing a string would collapse into ONE member — and the
// comparison against the constraint would still pass, having compared a
// vocabulary that had already lost a member. A census that can fail short has
// already failed, so the count is asserted against the declared constants
// rather than against itself.
func TestEveryCategoryCarriesADistinctWireValue(t *testing.T) {
	t.Parallel()

	seen := map[commsauthz.Category]bool{}
	for _, c := range commsauthz.Categories() {
		if seen[c] {
			t.Errorf("category %q appears twice in the vocabulary", c)
		}
		seen[c] = true
		if strings.TrimSpace(string(c)) == "" {
			t.Error("a category carries an empty wire value")
		}
		if !c.Valid() {
			t.Errorf("category %q is listed by Categories() but Valid() denies it", c)
		}
	}
	// The five that serve the subject are the ones a hard suppression may not
	// stop. Asserting the COUNT here means a sixth cannot be added silently:
	// ServesTheSubject widens what reaches somebody who asked us to stop, so it
	// is the one predicate in this vocabulary that must never grow by accident.
	serving := 0
	for _, c := range commsauthz.Categories() {
		if c.ServesTheSubject() {
			serving++
		}
	}
	if want := 5; serving != want {
		t.Errorf("%d categories serve the subject, want %d: this predicate decides what passes a "+
			"hard suppression, so a change here reaches somebody who asked us to stop", serving, want)
	}
}

// readDecisionMigration returns the shipped migration's text.
func readDecisionMigration(t *testing.T) string {
	t.Helper()
	path := filepath.Join("migrations", "core", decisionMigration)
	sql, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(sql)
}

// difference lists the keys of a that b lacks, ordered so a failure reads the
// same on every run.
func difference(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
