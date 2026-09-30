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
// lines, so a comparison that happens to sit beside an unrelated clause's
// `IS NULL` passes on proximity rather than on intent. lead_read.go's ORDER BY
// arm does exactly that today. It is the safe direction — a false PASS on a
// line that drops nothing, never a false pass on a filter — but it is the shape
// to widen if this is ever tightened.
//
// NOT a claim that NULL means booked everywhere. It means "nothing said it was
// off", and each surface adds its own time bound — which is why a past meeting
// with NULL reads as held in the brief and a future one reads as booked here.

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// meetingStatusIs matches a positive comparison against the column, and
// admitsNull matches the clause that lets an unrecorded meeting through. Both
// read the SQL as text because these are string literals inside Go, which is
// how every one of them is written.
var (
	meetingStatusIs = regexp.MustCompile(
		`meeting_status\s*(?:=\s*'(?:booked|held)'|IN\s*\([^)]*'(?:booked|held)'[^)]*\))`)
	admitsNull = regexp.MustCompile(`meeting_status\s+IS\s+NULL`)
)

// meetingStatusReadersAdmitted ratifies the two places a positive test is meant
// to drop an unrecorded meeting. Keyed by FILE AND the line's own text, so a
// waiver covers one reading and not every reading added to that file after it.
var meetingStatusReadersAdmitted = gatekit.Waive(map[string]string{
	"internal/compose/reportmeetingconversion.go: whereHeldMeeting = \"t.kind = 'meeting' AND t.meeting_status = 'held' AND t.archived_at IS NULL\"":               "the conversion report's subject is what the RECORD SAYS, which its own description states: \"meetings the record says were HELD (a booked one is a plan, and a no-show or a cancellation is a plan that failed)\". It carries no time bound of its own, so admitting NULL would count a meeting still in the future as one that happened — the opposite of the reading everywhere else, where NULL is paired with a past window",
	"internal/modules/contacts/lead_read.go: (SELECT jsonb_build_object('trigger', CASE WHEN a.kind = 'meeting' AND a.meeting_status = 'held' THEN 'meeting_held'": "a CASE arm that CLASSIFIES rather than filters: it names the trigger `meeting_held` for a meeting the record says was held and `meeting_booked` for every other meeting, which is where an unrecorded one lands. Dropping nothing, so there is nothing for an IS NULL arm to admit. The FILTER beside it is a different question and does admit NULL",
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
