// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// When a meeting ENDS is spelled once, in occurredday.EndSQL.
//
// Over means ENDED: start plus duration for an hour, the end of its date in the
// installation zone for a day. A copy of the end once called a day meeting over
// at its stored noon. So a literal outside the owner that adds a duration to a
// start is a finding.

import (
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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
	// meetingEndSpelling is start plus duration, in any alias's spelling.
	meetingEndSpelling = regexp.MustCompile(`make_interval\(\s*secs\s*=>\s*coalesce\([^)]*duration_seconds`)
)

// meetingEndLiterals counts the string literals in file that spell a meeting's end.
func meetingEndLiterals(file *ast.File) int {
	found := 0
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		text, err := strconv.Unquote(lit.Value)
		if err == nil && meetingEndSpelling.MatchString(text) {
			found++
		}
		return true
	})
	return found
}

func TestAMeetingsEndIsSpelledOnce(t *testing.T) {
	t.Parallel()
	scope := gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: func(_ string, file *ast.File) bool { return meetingEndLiterals(file) > 0 },
		Exempt:  gatekit.Waive(map[string]string{}),
	}
	owned := false
	for _, src := range scope.Files(t) {
		if filepath.Clean(src.Path) == meetingEndOwner {
			owned = true
			continue
		}
		t.Errorf("%s spells a meeting's end itself. Ask occurredday.EndSQL, or "+
			"storekit.MeetingIsOverSQL for \"is it over\": a copy misses the day "+
			"meeting that ends with its date, not at its stored noon", src.Path)
	}
	if !owned {
		t.Errorf("%s no longer spells a meeting's end where this census looks, so it "+
			"would report a pass having seen nothing. Re-derive the pattern from the owner", meetingEndOwner)
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
