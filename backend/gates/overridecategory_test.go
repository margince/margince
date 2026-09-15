// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// A rep's override names a category from the SAME closed vocabulary the send
// engine resolves against — commsauthz.Categories() — and the
// communication_override_category CHECK constraint restates that vocabulary by
// hand, because SQL cannot call the Go package. category.go says so in
// KnownForOverride's comment; this is what makes that claim fail if it ever
// stops being true.
//
// The failure this stops is the same shape as the sibling gate one migration
// over (authzcategories_test.go, for communication_decision): a category added
// to commsauthz and forgotten in this CHECK passes every compile and unit test,
// and the INSERT recording a rep's vouch for it fails at runtime inside the
// override transaction — the one path where a machine refusal was just
// overruled, so failing silently there is the worst place for it. The reverse —
// a value the CHECK admits that commsauthz does not know — is checked too,
// because it means the constraint permits something no code can produce and the
// next reader takes that permission for a specification.
//
// It reads the SHIPPED migration rather than a live schema, for the same reason
// the sibling gate does: the constraint is additive-only under the migration
// rule, so the file that created it is still the file that defines it, and a
// gate needing a database would not run in the unit lane where a vocabulary
// edit is actually made.

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// overrideMigration is the migration that created communication_override and
// its category CHECK. Named rather than globbed: a glob matching nothing would
// report PASS, which is the under-recognition this repository treats as a
// failure in itself.
const overrideMigration = "1789456230_a_rep_may_vouch_for_a_send.up.sql"

// overrideCategoryConstraint captures the ARRAY body of the
// communication_override_category CHECK. Mirrors categoryConstraint's shape in
// authzcategories_test.go, one constraint name and one table over.
var overrideCategoryConstraint = regexp.MustCompile(
	`(?s)CONSTRAINT communication_override_category\s*CHECK \(category = ANY \(ARRAY\[(.*?)\]\)\)`)

func TestOverrideCategoryVocabularyAgreesWithItsCheckConstraint(t *testing.T) {
	t.Parallel()

	path := filepath.Join("migrations", "core", overrideMigration)
	sql, err := os.ReadFile(path) // #nosec G304 -- path is a fixed migration name under the trusted migrations tree
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	body := overrideCategoryConstraint.FindStringSubmatch(string(sql))
	if body == nil {
		// A renamed constraint is not a reason to pass. If this fires after a
		// deliberate rename, repoint the pattern — never delete the assertion.
		t.Fatalf("no communication_override_category CHECK found in %s: the constraint this gate "+
			"holds against the Go vocabulary has moved or been renamed", overrideMigration)
	}

	// quotedLiteral is declared in authzcategories_test.go, in this same
	// package — reused rather than restated, since it already parses the exact
	// 'value'::text shape both CHECK constraints share.
	inSQL := map[string]bool{}
	for _, m := range quotedLiteral.FindAllStringSubmatch(body[1], -1) {
		inSQL[m[1]] = true
	}
	if len(inSQL) == 0 {
		t.Fatalf("the %s constraint body parsed to no values — the gate is reading the wrong text",
			overrideMigration)
	}

	inGo := map[string]bool{}
	for _, c := range commsauthz.Categories() {
		inGo[string(c)] = true
	}

	// difference is declared in authzcategories_test.go, in this same package.
	for _, missing := range difference(inGo, inSQL) {
		t.Errorf("category %q is in the Go vocabulary but not in the communication_override_category "+
			"CHECK: a rep could never name it in a vouch, or — if KnownForOverride ever narrows below "+
			"Valid — the door silently admits every category the engine can resolve", missing)
	}
	for _, extra := range difference(inSQL, inGo) {
		t.Errorf("category %q is admitted by the communication_override_category CHECK but is not a "+
			"commsauthz.Category: the constraint permits a value no rep's vouch can actually produce",
			extra)
	}
}
