// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H1

package gates

// A stored original is found by the reference, never by a key two writers spell
// two ways.
//
// `source_id` is what ONE provider called a message, and raw_capture's copy of
// it is written by two lanes that mean different things by it: the mail sink
// stores the domain natural key, the channel poll stores the provider's
// redelivery counter. A predicate correlating the two tables on that column is
// therefore true for mail and false for every channel — and false SILENTLY,
// because a join that matches nothing returns the same empty result as a query
// with nothing to find. Each reader was wrong in its own direction: the Art. 15
// disclosure gate withheld an open channel original from its own subject, the
// redaction purge left one standing and reported success.
//
// `activity.raw_capture_id` is the one answer, and a wrong one cannot be
// silent: the FK refuses an id no raw_capture row carries.
//
// WHAT IT READS: every statement in every hand-written Go file of the licensed
// trees, tests included — a test that re-derives the pair to check a purge is
// asserting against a correlation production no longer makes — plus the shipped
// migrations, which are the other place SQL is written. Statements come from
// gatekit.SQLStatementsIn, which decodes and flattens a `+` chain, so a
// predicate concatenated onto a fragment is read as part of the statement it
// joins rather than slipping past as a fragment nothing recognises.
//
// WHAT IT CANNOT SEE, so nothing more is read into a green run: a correlation
// whose column name arrives at runtime, and one spelled through a view or a
// function body in the database rather than in this tree.

import (
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// correlationSubject is one offending predicate, named by the file that spells
// it. The vocabulary is its own type so a waiver cannot be keyed by anything
// else the gates package happens to call a string.
type correlationSubject string

// rawCaptureStatementFloor pins the census. The prohibition is over every
// statement, but the mentions of raw_capture are the population this rule
// exists for, and a reader that stopped recognising them would report a clean
// tree in the same words as a clean tree.
//
// The tree holds 164. The floor sits close rather than at a token value,
// because the failure it guards against is a reader that goes quiet — and a
// reader that lost three quarters of its reach would clear any slack number.
const rawCaptureStatementFloor = 120

var pairCorrelationWaived = gatekit.Waive(map[correlationSubject]string{
	"internal/compose/rawcapturelinkbackfill.go:r.source_id = a.source_id": "the one-off pass that converts the correlation INTO the reference has to " +
		"spell the correlation once, and this is it — the mail arm, pairing an activity captured before the column existed with the " +
		"original its own natural key names. Sound only because it runs over rows already written and writes raw_capture_id rather " +
		"than reading through the pair: nothing downstream reconstructs a key again. It goes stale when the backfill does",
})

// pairCorrelation matches a correlation between two tables' source_id columns:
// both sides QUALIFIED, which is what separates a join predicate from the
// `source_id = $2` of a lookup by a key the caller already holds.
//
// The alias is unanchored on the left because a predicate can arrive mid-line
// (`... AND r.source_id = a.source_id`), and the trailing boundary keeps
// `a.source_id = b.source_identity` from reading as one.
var pairCorrelation = regexp.MustCompile(`\b[a-z_]+\.source_id\s*=\s*[a-z_]+\.source_id\b`)

// naturalJoinCorrelation matches the same correlation spelled as a join's own
// column list. Postgres's `DELETE … USING <table>` takes no parentheses and is
// not this, which is why the paren is required rather than the word alone.
var naturalJoinCorrelation = regexp.MustCompile(`(?i)\busing\s*\([^)]*\bsource_id\b`)

func TestNothingCorrelatesTwoTablesOnAProvidersOwnKey(t *testing.T) {
	t.Parallel()
	defer pairCorrelationWaived.AssertAllMatched(t)

	var findings []string
	mentions := 0
	for _, tree := range licensedTrees {
		checked := walkHandWrittenGoFiles(t, tree.root, func(path, text string) {
			// This file plants the spellings it bans, so reading it would fail
			// on its own source. The exact path, not the basename: a second
			// file of this name elsewhere is read like any other.
			if path == "gates/rawcapturereference_test.go" {
				return
			}
			for _, statement := range gatekit.SQLStatementsIn(t, path, text) {
				mentions += strings.Count(statement, "raw_capture")
				findings = append(findings, correlationsIn(t, path, statement)...)
			}
		})
		if tree.mustHaveFiles && checked == 0 {
			t.Fatalf("%s yielded no hand-written Go file — a root that scans nothing passes exactly like a clean one", tree.root)
		}
	}
	shipped := 0
	walkTextFiles(t, "migrations", func(path, text string) {
		if strings.HasSuffix(path, ".sql") {
			shipped++
			mentions += strings.Count(text, "raw_capture")
			findings = append(findings, correlationsIn(t, path, text)...)
		}
	})
	// Counted on its own: the Go mentions alone clear the floor below, so a
	// migrations root that stopped yielding files would leave the other half of
	// the corpus unread behind a census that still passes.
	if shipped == 0 {
		t.Fatal("the sweep read no migration, so the SQL this tree ships is outside the corpus this gate claims to hold")
	}

	if mentions < rawCaptureStatementFloor {
		t.Fatalf("the sweep read %d mention(s) of raw_capture and the tree holds at least %d: the reader has "+
			"stopped recognising this tree's SQL, and a census of nothing reports the clean tree it never read",
			mentions, rawCaptureStatementFloor)
	}
	if len(findings) > 0 {
		t.Errorf("%d correlation(s) on a provider's own key:\n\t%s\n\n"+
			"raw_capture.source_id is the mail sink's natural key in one lane and the channel poll's redelivery "+
			"counter in another, so this predicate is true for mail and silently false for every channel. Join on "+
			"activity.raw_capture_id, the reference every lane writes; a row captured before that column existed is "+
			"named by compose/rawcapturelinkbackfill.go rather than by reconstructing the pair here.",
			len(findings), strings.Join(findings, "\n\t"))
	}
}

// correlationsIn reports the offending predicates one statement spells, each
// asked of the waivers as the offender it is.
func correlationsIn(t *testing.T, path, statement string) []string {
	t.Helper()
	var out []string
	for _, spelling := range [][]string{
		pairCorrelation.FindAllString(statement, -1),
		naturalJoinCorrelation.FindAllString(statement, -1),
	} {
		for _, predicate := range spelling {
			predicate = strings.Join(strings.Fields(predicate), " ")
			if pairCorrelationWaived.Waived(t, correlationSubject(path+":"+predicate)) {
				continue
			}
			out = append(out, path+": "+predicate)
		}
	}
	return out
}

// TestTheCorrelationReaderSeesEverySpellingItClaims plants the shapes the rule
// is about, because a prohibition anchored on the obvious spelling passes in
// exactly the words of a clean tree.
//
// The fragment case is the one that caught a real miss: capture's noise sweep
// builds its predicate as `withinVerdictReach() + "AND …"`, which no reader
// that judges only statement-shaped strings would ever have read.
func TestTheCorrelationReaderSeesEverySpellingItClaims(t *testing.T) {
	t.Parallel()
	for _, planted := range []struct {
		name, statement string
		correlates      bool
	}{
		{"the join as written", `SELECT 1 FROM raw_capture r JOIN activity a ON r.source_system = a.source_system AND r.source_id = a.source_id`, true},
		{"the operands the other way round", `SELECT 1 FROM activity a JOIN raw_capture r ON a.source_id = r.source_id`, true},
		{"a predicate broken over lines", "SELECT 1 FROM raw_capture r, activity a\n\t\tWHERE r.source_id =\n\t\t      a.source_id", true},
		{"a fragment concatenated onto a call's text", `AND EXISTS (SELECT 1 FROM raw_capture r WHERE r.source_id = a.source_id)`, true},
		{"the natural-join spelling", `SELECT 1 FROM activity a JOIN raw_capture r USING (source_system, source_id)`, true},
		{"the reference", `SELECT 1 FROM raw_capture r JOIN activity a ON r.id = a.raw_capture_id`, false},
		{"a lookup by a key the caller holds", `SELECT id FROM raw_capture WHERE source_system = $1 AND source_id = $2`, false},
		{"a delete correlated on the reference", `DELETE FROM raw_capture r USING activity a WHERE a.id = ANY($1) AND r.id = a.raw_capture_id`, false},
		{"an attachment keyed on the pair spelled as one string", `SELECT 1 FROM attachment at JOIN raw_capture rc ON at.external_source_id = rc.source_system || ':' || rc.source_id`, false},
	} {
		t.Run(planted.name, func(t *testing.T) {
			t.Parallel()
			found := correlationsIn(t, "planted.go", planted.statement)
			if planted.correlates && len(found) == 0 {
				t.Errorf("the reader does not see %s, so the rule is unenforced for that spelling:\n\t%s",
					planted.name, planted.statement)
			}
			if !planted.correlates && len(found) > 0 {
				t.Errorf("the reader reports %s, which correlates nothing:\n\t%s", planted.name, found)
			}
		})
	}
}
