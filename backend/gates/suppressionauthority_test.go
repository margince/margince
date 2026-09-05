// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every kind of suppression says who decided it.
//
// communication_suppression.decided_by_level answers "who may lift this", and
// the answer is derived from `kind` by the migration that added the column. The
// two live in different files and different languages, so nothing stops the
// `kind` CHECK growing a fifth value while the classification keeps naming four.
//
// That failure is silent in the worst direction. A kind nobody classified takes
// the CASE's ELSE arm and becomes 'subject' — the tier nothing can lift — so a
// new operational suppression would quietly become permanent, and the first
// symptom is a customer nobody can email with no visible reason.
//
// The corpus is read from the catalog rather than restated here, because a gate
// that hard-codes part of its subject has become a second copy of it.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// suppressionKindCheck reads the CHECK the catalog records for the column that
// names a suppression's kind.
var suppressionKindCheck = regexp.MustCompile(
	`public\.communication_suppression\.communication_suppression_kind CHECK .*`)

// suppressionKindLiteral pulls each 'value'::text out of a CHECK body.
var suppressionKindLiteral = regexp.MustCompile(`'([a-z_]+)'::text`)

// TestEverySuppressionKindIsClassifiedByTheMigrationThatAddedTheLevel fails when
// the kind vocabulary grows past what the classification names.
func TestEverySuppressionKindIsClassifiedByTheMigrationThatAddedTheLevel(t *testing.T) {
	t.Parallel()

	catalog, err := os.ReadFile("migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatalf("reading the schema catalog: %v", err)
	}
	check := suppressionKindCheck.Find(catalog)
	if check == nil {
		t.Fatal("the catalog records no kind CHECK for communication_suppression: " +
			"the gate has stopped seeing its subject, which reports PASS over an unchecked vocabulary")
	}

	kinds := suppressionKindLiteral.FindAllStringSubmatch(string(check), -1)
	// Under-recognition is the one way this must not fail. A regex that matched
	// nothing would pass an empty loop and prove nothing at all.
	if len(kinds) < 4 {
		t.Fatalf("read %d suppression kinds from the catalog, want at least the four "+
			"the column shipped with: the pattern has stopped matching", len(kinds))
	}

	classifier, err := os.ReadFile(levelClassifyingMigration)
	if err != nil {
		t.Fatalf("reading the migration that classifies each kind: %v", err)
	}

	for _, kind := range kinds {
		if !strings.Contains(string(classifier), "'"+kind[1]+"'") {
			t.Errorf("suppression kind %q is not named by %s, so it falls to that file's "+
				"ELSE arm and becomes 'subject' — a suppression nobody can lift. Add an arm "+
				"saying who decides a %[1]q, in a NEW migration rather than by editing a shipped one",
				kind[1], levelClassifyingMigration)
		}
	}
}

// levelClassifyingMigration is the file deriving decided_by_level from kind.
//
// A later migration that reclassifies a kind must be named here instead, which
// is the point: the gate asks which file is authoritative TODAY, and a stale
// answer fails rather than silently checking a superseded rule.
const levelClassifyingMigration = "migrations/core/" +
	"1788572167_a_suppression_records_who_decided_it.up.sql"
