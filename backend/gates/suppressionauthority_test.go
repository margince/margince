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

	// The CASE arms only, never the whole file. The migration also carries a
	// CHECK listing the authority levels — 'machine', 'user', 'admin',
	// 'subject' — so a bare substring search for a kind named `user` would find
	// that constraint's literal and report a kind classified when it actually
	// falls to the ELSE arm. A gate that matches lines instead of statements
	// passes over the defect it exists to catch.
	// Comments stripped FIRST. The pattern below cannot tell a live CASE arm
	// from one somebody commented out while debugging, and a gate that reads a
	// commented `WHEN kind = 'x'` as a classification reports PASS over a kind
	// production actually sends through the ELSE arm — unliftable, silently.
	// Under-recognition is the failure this file exists to prevent, so the
	// corpus is the executable statement and nothing else.
	arms := classifyingArm.FindAllStringSubmatch(sqlWithoutComments(string(classifier)), -1)
	classified := make(map[string]bool, len(arms))
	for _, arm := range arms {
		for _, lit := range caseArmKindLiteral.FindAllStringSubmatch(arm[1], -1) {
			classified[lit[1]] = true
		}
	}
	if len(classified) == 0 {
		t.Fatal("no WHEN arm in " + levelClassifyingMigration + " names a kind: the pattern " +
			"has stopped matching, and an empty set would report every kind unclassified " +
			"or none — neither is a reading of the file")
	}

	for _, kind := range kinds {
		if !classified[kind[1]] {
			t.Errorf("suppression kind %q is not named by %s, so it falls to that file's "+
				"ELSE arm and becomes 'subject' — a suppression nobody can lift. Add an arm "+
				"saying who decides a %[1]q, in a NEW migration rather than by editing a shipped one",
				kind[1], levelClassifyingMigration)
		}
	}
}

// sqlWithoutComments drops `-- ...` to end of line. Enough for this gate: the
// migration it reads carries no string literal containing a double dash, and a
// block-comment form would need the same treatment if one ever appeared.
func sqlWithoutComments(sql string) string {
	var b strings.Builder
	for line := range strings.SplitSeq(sql, "\n") {
		if i := strings.Index(line, "--"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

// classifyingArm pulls the body of each `WHEN ... THEN` arm of the backfill's
// CASE. Matching the arm rather than the file is what keeps the CHECK
// constraint's own level literals out of the answer.
var classifyingArm = regexp.MustCompile(`(?i)WHEN\s+(kind\s+(?:=|IN)[^\n]*?)\s+THEN`)

// caseArmKindLiteral pulls a bare 'value' out of a CASE arm, where the
// literals carry no ::text cast the catalog's CHECK bodies have.
var caseArmKindLiteral = regexp.MustCompile(`'([a-z_]+)'`)

// levelClassifyingMigration is the file deriving decided_by_level from kind.
//
// A later migration that reclassifies a kind must be named here instead, which
// is the point: the gate asks which file is authoritative TODAY, and a stale
// answer fails rather than silently checking a superseded rule.
const levelClassifyingMigration = "migrations/core/" +
	"1788572167_a_suppression_records_who_decided_it.up.sql"
