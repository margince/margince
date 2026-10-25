// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// When a meeting ENDS is spelled once, in occurredday.EndSQL.
//
// Over means ENDED: start plus duration for an hour, the end of its date in the
// installation zone for a day. A copy of the end once called a day meeting over
// at its stored noon. So a statement outside the owner that adds a duration to
// a start is a finding. A line of SQL that only an instant can reach says why,
// as `-- meeting end: <reason>`.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

var (
	// meetingEndOwner is the one file allowed to add a meeting's duration to its start.
	meetingEndOwner = filepath.Join("internal", "shared", "kernel", "occurredday", "occurredday.go")
	// meetingOverDealsOwner holds the next-step rule, whose standing test the
	// second check below reads.
	meetingOverDealsOwner = filepath.Join(
		repoRoot, "backend", "internal", "modules", "deals", "nextstep.go")
	// meetingEndSpelling is start plus duration, in any alias's spelling: as an
	// interval made from seconds, or seconds multiplied by one.
	meetingEndSpelling = regexp.MustCompile(`make_interval\(\s*secs\s*=>\s*coalesce\([^)]*duration_seconds|` +
		`duration_seconds[^']*\*\s*interval\s*'1\s*second'`)
	// meetingEndWaiver is the marker a statement carries on the spelling's line.
	meetingEndWaiver = regexp.MustCompile(`--\s*meeting end:\s*\S+(?:\s+\S+){2,}`)
)

// meetingEnds counts the spellings of a meeting's end in file's statements, and
// how many carry no waiver on their own line.
func meetingEnds(file *ast.File) (spelled, unwaived int) {
	for _, statement := range gatekit.SQLStatementsOf(file) {
		for _, at := range meetingEndSpelling.FindAllStringIndex(statement, -1) {
			spelled++
			lineEnd := strings.IndexByte(statement[at[1]:], '\n')
			if lineEnd < 0 {
				lineEnd = len(statement) - at[1]
			}
			lineStart := strings.LastIndexByte(statement[:at[1]], '\n') + 1
			if !meetingEndWaiver.MatchString(statement[lineStart : at[1]+lineEnd]) {
				unwaived++
			}
		}
	}
	return spelled, unwaived
}

func TestAMeetingsEndIsSpelledOnce(t *testing.T) {
	t.Parallel()
	scope := gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: func(_ string, file *ast.File) bool { spelled, _ := meetingEnds(file); return spelled > 0 },
		Exempt:  gatekit.Waive(map[string]string{}),
	}
	owned, waived := false, 0
	for _, src := range scope.Files(t) {
		if filepath.Clean(src.Path) == meetingEndOwner {
			owned = true
			continue
		}
		spelled, unwaived := meetingEnds(src.File)
		waived += spelled - unwaived
		if unwaived > 0 {
			t.Errorf("%s spells a meeting's end itself %d times. Ask occurredday.EndSQL, or "+
				"storekit.MeetingIsOverSQL for \"is it over\": a copy misses the day meeting that "+
				"ends with its date, not at its stored noon. If only an instant can reach it, say "+
				"why on its line: -- meeting end: <reason>", src.Path, unwaived)
		}
	}
	if !owned {
		t.Errorf("%s no longer spells a meeting's end where this census looks, so it "+
			"would report a pass having seen nothing. Re-derive the pattern from the owner", meetingEndOwner)
	}
	if waived < 3 {
		t.Errorf("found %d waived meeting-end spellings, want the host-slot reads' 3: the census reads less than it should", waived)
	}
}

// The census sees both spellings of an end, so a copy written the other way
// is still a finding.
func TestTheMeetingEndCensusSeesBothSpellings(t *testing.T) {
	t.Parallel()
	for _, literal := range []string{
		"m.occurred_at + make_interval(secs => coalesce(m.duration_seconds, 0))",
		"m.occurred_at + coalesce(m.duration_seconds, 0) * interval '1 second'",
		"occurred_at+(CASE WHEN x THEN duration_seconds ELSE 3600 END)*interval '1 second'",
	} {
		if !meetingEndSpelling.MatchString(literal) {
			t.Errorf("the census does not see %q", literal)
		}
	}
	if meetingEndSpelling.MatchString("m.duration_seconds > 0 AND m.kind = 'meeting'") {
		t.Error("the census reads a bare duration as a meeting's end")
	}
	for statement, want := range map[string]int{
		"SELECT m.occurred_at + make_interval(secs => coalesce(m.duration_seconds, 0)) FROM activity m":                               1,
		"SELECT m.occurred_at + make_interval(secs => coalesce(m.duration_seconds, 0)) -- meeting end: an instant read\n FROM x":      0,
		"SELECT m.occurred_at + make_interval(secs => coalesce(m.duration_seconds, 0))\n -- meeting end: a later line waives nothing": 1,
	} {
		file, err := parser.ParseFile(token.NewFileSet(), "seeded.go", "package p\nconst q = `"+statement+"`\n", 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, unwaived := meetingEnds(file); unwaived != want {
			t.Errorf("%q has %d unwaived spellings, want %d", statement, unwaived, want)
		}
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
