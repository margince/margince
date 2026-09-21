// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind claim H2

package gates

// "This meeting is over" is spelled twice, and the two must say the same thing.
//
// Over means ENDED — start plus duration, not start — and a meeting that will
// never happen (archived, cancelled, a no-show) counts as over. Three readers
// depend on that reading: the waiting queue's snooze-lift, the brief's, and the
// deal sweeps that decide whether a meeting is evidence of a touch yet.
//
// Why a second spelling is allowed here at all, when the tree's usual answer is
// one helper: `activities` owns the rule, and a module never imports a sibling.
// `deals` cannot reach it. So the copy is deliberate and this gate is the price
// of it — the thing that makes "the two agree" a fact rather than a hope.
//
// It fails in BOTH directions: reword either side and the comparison fails, so
// neither can drift silently. The needles are DERIVED from each owner rather
// than written here, so a rewritten clause cannot leave this gate hunting for a
// string that no longer exists and reporting a pass having compared nothing —
// under-recognition is the one way a gate must not break.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	// meetingOverActivitiesOwner holds the rule as the waiting queue asks it.
	meetingOverActivitiesOwner = filepath.Join(
		repoRoot, "backend", "internal", "modules", "activities", "waitingsql.go")
	// meetingOverDealsOwner holds the copy the deal sweeps ask it with.
	meetingOverDealsOwner = filepath.Join(
		repoRoot, "backend", "internal", "modules", "deals", "nextstep.go")
)

// meetingOverShape reduces one spelling to the facts it asserts, so the
// comparison survives the differences that are NOT drift: the alias each side
// binds (`m.` here, `%[1]s.` there), its clock placeholder, and its whitespace.
// What survives is the sequence of predicates, which is the rule itself.
func meetingOverShape(clause string) string {
	// The alias is whatever precedes the columns; normalize every form of it.
	clause = regexp.MustCompile(`%\[\d\]s\.|\bm\.`).ReplaceAllString(clause, "X.")
	// The clock is a placeholder on one side and a format verb on the other.
	clause = regexp.MustCompile(`%\[\d\]s|\$\d+`).ReplaceAllString(clause, "CLOCK")
	return strings.Join(strings.Fields(clause), " ")
}

// meetingOverClause reads the distinctive run of predicates out of an owner,
// starting at the archived-or-cancelled test and ending at the clause's closing
// parenthesis.
//
// The end is found by BALANCING parentheses from the opening one, not by
// stopping at a marker inside the text. An earlier version cut the needle at
// the duration add, which left the comparison OPERATOR outside what was
// compared: flipping the deals copy's `<=` to `>` — inverting the rule — kept
// the gate green. A census that can fail short has already failed, and this one
// did.
func meetingOverClause(t *testing.T, name, body string) string {
	t.Helper()
	const opens = "archived_at IS NOT NULL"
	at := strings.Index(body, opens)
	if at < 0 {
		t.Fatalf("%s no longer contains %q, so this gate would compare nothing and "+
			"report a pass. Re-derive the marker from the clause's current text.",
			name, opens)
	}
	// Walk back to the parenthesis this predicate opens under, then forward to
	// its match: everything between is the clause, operators included.
	start := strings.LastIndex(body[:at], "(")
	if start < 0 {
		t.Fatalf("%s opens the meeting-over clause with no enclosing parenthesis", name)
	}
	depth := 0
	for i := start; i < len(body); i++ {
		switch body[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return body[start : i+1]
			}
		}
	}
	t.Fatalf("%s opens the meeting-over clause and never closes it", name)
	return ""
}

func TestTheMeetingIsOverRuleIsSpelledTheSameOnBothSides(t *testing.T) {
	t.Parallel()

	activities, err := os.ReadFile(meetingOverActivitiesOwner)
	if err != nil {
		t.Fatalf("reading the activities spelling: %v", err)
	}
	dealsBody, err := os.ReadFile(meetingOverDealsOwner)
	if err != nil {
		t.Fatalf("reading the deals spelling: %v", err)
	}

	theirs := meetingOverShape(meetingOverClause(
		t, filepath.Base(meetingOverActivitiesOwner), string(activities)))
	ours := meetingOverShape(meetingOverClause(
		t, filepath.Base(meetingOverDealsOwner), string(dealsBody)))

	if theirs != ours {
		t.Errorf("the two spellings of \"this meeting is over\" have drifted.\n"+
			"  %s: %s\n  %s: %s\n"+
			"They are a deliberate copy — deals may not import activities — so they "+
			"only stay one rule while they stay identical. Reconcile them; do not "+
			"relax this gate.",
			filepath.Base(meetingOverActivitiesOwner), theirs,
			filepath.Base(meetingOverDealsOwner), ours)
	}
}

// A future meeting is admitted by asking about CANCELLATION alone, and that
// question must never be written as `meeting_status NOT IN (...)`.
//
// The column is nullable and most captured meetings carry no status — a
// calendar entry nobody has marked. `NOT IN` is NULL-false, so that spelling
// silently calls every unmarked meeting cancelled and excludes nearly every
// meeting there is. The bug reads as "the feature does nothing", which is
// exactly the shape that survives review.
func TestAFutureMeetingTestAdmitsAnUnmarkedStatus(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(meetingOverDealsOwner)
	if err != nil {
		t.Fatalf("reading the deals spelling: %v", err)
	}
	text := string(body)

	const admits = "meeting_status IS NULL OR"
	if !strings.Contains(text, admits) {
		t.Errorf("%s no longer admits an unmarked meeting_status. A nullable column "+
			"tested with NOT IN excludes every row that has no status, which is most "+
			"of them.", filepath.Base(meetingOverDealsOwner))
	}
}
