// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
//gate:kind claim H1

package backendarch

// "How many days of silence" is spelled once.
//
// It was spelled twice, and the two disagreed ON SCREEN: the deal card's move
// said "They wrote 96 days ago" while the coverage chip beside it said "95
// days", about the same mail, in the same second. One counted calendar days,
// the other elapsed hours over 24, so they drifted apart for most of every day
// and agreed only around midnight.
//
// Neither was obviously wrong to its own author, which is the point — this is
// not a bug a reviewer catches by reading one file. What catches it is a gate
// that fails when the second spelling appears.
//
// WHAT THIS GATE CAN AND CANNOT SEE. It greps for the arithmetic that divides
// a duration by 24, which is the shape both old spellings had. It cannot see a
// count written some other way — a day-granularity SQL date_part, say — so it
// is a net under the known failure rather than a proof of uniqueness.
//
// AND IT IS SCOPED, with a cost worth naming precisely. The trees carry other
// sites with the same arithmetic, and they are not all the same rule:
// deals/health.go and people/leadscore.go return a FLOAT because they feed
// decay curves, where a fractional day is the quantity and rounding it to a
// calendar boundary would change what the product scores. finance/summary.go
// counts invoice lateness against a due DATE, and search/graphtrust.go decays
// a score. Those are duration questions, and converting them blind is a
// different change with its own risk.
//
// So this gate governs the packages whose day count a person READS in a
// sentence, and every one of those has been converted. The first version of
// the comment here said "nine more sites" when there were twenty-three, and
// justified the scope by pointing only at the float curves — while
// meetingbrief/sections.go was printing "Last touch was %d days ago" with the
// very arithmetic this change exists to remove. Both are fixed: the count is
// measured rather than remembered, and meetingbrief, person360 and org360 are
// governed here.
//
// The cost that remains: a NEW reader-facing count in an ungoverned package
// still passes. governedSurfaces is the list to extend when one appears.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The shape both old spellings had: a duration in hours, divided by 24. Written
// as a pattern rather than a literal so `Hours()/24`, `Hours() / 24` and
// `Hours()/hoursPerDay` all match — the last is what one of them actually said,
// and a check that missed it would have passed the day the bug shipped.
var durationOverADay = regexp.MustCompile(`\.Hours\(\)\s*/\s*(24|hoursPerDay)\b`)

// governedSurfaces are the files whose day count a person reads on a screen,
// where two spellings disagreeing is a defect they can SEE. Named rather than
// derived because "does a reader see this number?" is not a question a
// syntactic walk can answer, and a gate over every site would have to waive
// the float-valued scoring maths one by one.
var governedSurfaces = []string{
	"internal/compose/dealstatus",
	"internal/compose/network",
	"internal/compose/meetingbrief",
	"internal/compose/person360",
	"internal/compose/org360",
}

func TestOnlyElapsedCountsDaysOfSilence(t *testing.T) {
	found := offenders(t)
	for _, where := range found {
		t.Errorf("%s divides a duration by 24 to count days. That is the second spelling of a rule "+
			"shared/kernel/elapsed owns, and the two last disagreed by a day on one screen. "+
			"Call elapsed.Days(from, to) instead", where)
	}
}

// TestTheGateCanStillSeeItsOwnSubject is the vacuity check.
//
// The gate passes by finding nothing, which is also what it does if it walks
// the wrong tree, compiles a pattern that matches nothing, or is handed a
// regexp somebody loosened into uselessness. So it asserts that the pattern
// still recognises the shape it was written for — the exact line the coverage
// rule used to carry — rather than only that today's tree is clean.
func TestTheGateCanStillSeeItsOwnSubject(t *testing.T) {
	wasReal := `	days := int(now.Sub(c.LastTouchAt).Hours() / hoursPerDay)`
	if !durationOverADay.MatchString(wasReal) {
		t.Error("the pattern no longer matches the line this gate was written to catch, so it is guarding nothing")
	}
	if !durationOverADay.MatchString("d.Hours()/24") {
		t.Error("the pattern no longer matches a bare Hours()/24")
	}
	// The walk must reach real files. A governedSurfaces entry pointing at a
	// directory that no longer exists makes the gate pass by reading nothing,
	// which is the one way a census fails: it reports the same word, PASS,
	// over a smaller tree, and no assertion notices.
	for _, tree := range governedSurfaces {
		entries, err := os.ReadDir(tree)
		if err != nil {
			t.Errorf("governedSurfaces names %s, which cannot be read: %v", tree, err)
			continue
		}
		if !carriesGo(entries) {
			t.Errorf("governedSurfaces names %s, which holds no Go files: this gate is walking nothing", tree)
		}
	}
}

// carriesGo reports whether a directory holds any Go source at all.
func carriesGo(entries []os.DirEntry) bool {
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
			return true
		}
	}
	return false
}

// offenders walks the governed trees and names every file carrying the shape.
func offenders(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, tree := range governedSurfaces {
		err := filepath.WalkDir(tree, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			// Test files may legitimately spell the arithmetic to check it.
			// elapsed's own file needs no exclusion: it lives in
			// shared/kernel, which no governed package walk reaches.
			if d.IsDir() || !strings.HasSuffix(path, ".go") ||
				strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if durationOverADay.Match(raw) {
				out = append(out, path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", tree, err)
		}
	}
	return out
}
