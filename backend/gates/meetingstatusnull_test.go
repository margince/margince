// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

//go:build !integration

package gates

// A meeting with no recorded status is one nothing has said is off.
//
// `activity.meeting_status` has no default and the capture sink writes none, so
// every calendar-imported meeting carries NULL — and the column's own backfill
// says the convention out loud: "Every meeting captured before that carries
// `meeting_status` NULL, which every surface reads as booked."
//
// Every surface except two, which is what this refuses. A predicate written
// `meeting_status = 'booked'` drops exactly the meetings a connector imported:
// the week's capacity line read emptier than the rep's actual week, and a deal
// whose next step was a synced meeting raised a "no next step" finding. Neither
// failed anything — the rows were simply not there.
//
// WHICH comparisons. Only the positive ones — `= 'booked'`, `= 'held'` — where
// NULL and the value mean the same thing to the reader. A NEGATIVE test
// (`NOT IN ('canceled', 'no_show')`) already excludes NULL in SQL's own
// three-valued logic, and every site in the tree that writes one pairs it with
// `IS NULL OR` for that reason; a gate reading those too would be asking about
// a different question.
//
// WHAT IT CANNOT SEE, stated rather than discovered later: the window is two
// lines. A comparison beside an unrelated clause's `IS NULL` passes on
// proximity rather than on intent, which is the shape to widen if this is ever
// tightened.
//
// NOT a claim that NULL means booked everywhere. It means "nothing said it was
// off", and each surface adds its own time bound. A future meeting with NULL
// reads as booked. A past one reads as awaiting its outcome, never as held.

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// meetingStatusIs matches a positive comparison against the column. The value
// may be a literal or a joined Booked or Held constant. admitsNull matches the
// clause that lets an unrecorded meeting through. Both read the SQL as text
// inside Go string literals.
var (
	meetingStatusIs = regexp.MustCompile(
		`meeting_status\s*(?:=\s*'(?:booked|held)'|=\s*'"\s*\+\s*(?:\w+\.)?(?:Booked|Held)\b|IN\s*\([^)]*'(?:booked|held)'[^)]*\))`)
	admitsNull = regexp.MustCompile(`meeting_status\s+IS\s+NULL`)
)

// meetingStatusReadersAdmitted ratifies the places a positive test is meant to
// drop an unrecorded meeting. Keyed by the file and the line's own text, so a
// waiver covers one reading and not every reading added to that file after it.
var meetingStatusReadersAdmitted = gatekit.Waive(map[string]string{
	"internal/shared/kernel/meetingoutcome/meetingoutcome.go: return \"(\" + alias + \".meeting_status = '\" + Held + \"')\"": "the shared rule for a meeting that took place, and its subject is what the record says: a meeting nobody reported stays awaiting its outcome. Its callers either count what took place (the weekly held count, the conversion report, metric meetings, Deal Scout, stage evidence, the meeting follow-up lane and reconciliation evidence), where a past meeting nobody reported must be dropped, or classify with it, where the next CASE arm names an unrecorded meeting booked. Being in touch is a different rule: relstrength.HadContactSQL admits a past meeting with no status",
})

func TestAMeetingWithNoStatusIsNotDroppedFromAPositiveTest(t *testing.T) {
	t.Parallel()
	defer meetingStatusReadersAdmitted.AssertAllMatched(t)

	var strict []string
	scanned := 0
	err := filepath.WalkDir("internal", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") || strings.Contains(path, "/contracts/") {
			return err
		}
		raw, readErr := os.ReadFile(path) // #nosec G304 -- a *.go path from walking the trusted tree
		if readErr != nil {
			return readErr
		}
		scanned++
		for _, line := range strictMeetingLines(string(raw)) {
			finding := path + ": " + line
			if meetingStatusReadersAdmitted.Waived(t, finding) {
				continue
			}
			strict = append(strict, finding)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking internal: %v", err)
	}
	// A walk that read nothing would report a clean tree.
	if scanned < 500 {
		t.Fatalf("read %d Go files under internal/ and expected far more — this gate has "+
			"stopped reaching its corpus", scanned)
	}
	if len(strict) > 0 {
		t.Errorf("%d positive meeting_status test(s) drop an unrecorded meeting:\n\t%s\n"+
			"A calendar-imported meeting carries no status at all, so `= 'booked'` alone "+
			"silently excludes every synced meeting. Write "+
			"`(x.meeting_status IS NULL OR x.meeting_status = 'booked')`.",
			len(strict), strings.Join(strict, "\n\t"))
	}
}

// strictMeetingLines reports each positive comparison that does not admit NULL
// within its own clause.
//
// The window is the matched line and its two neighbours, not the whole SQL
// literal. Per literal was the first shape and it fails short: one lenient
// clause anywhere in a long statement made every strict clause in the same
// statement read as covered, which is how a mixed query would have passed. Two
// lines is what the lenient form actually spans when gofmt wraps it.
func strictMeetingLines(src string) []string {
	lines := strings.Split(src, "\n")
	var out []string
	for i, line := range lines {
		if !meetingStatusIs.MatchString(line) {
			continue
		}
		if admitsNullNear(lines, i) {
			continue
		}
		out = append(out, strings.TrimSpace(line))
	}
	return out
}

// admitsNullNear reports whether the comparison's own clause lets an unrecorded
// meeting through, reading one line either side for the wrapped spelling.
func admitsNullNear(lines []string, at int) bool {
	for i := max(0, at-1); i <= min(len(lines)-1, at+1); i++ {
		if admitsNull.MatchString(lines[i]) {
			return true
		}
	}
	return false
}
