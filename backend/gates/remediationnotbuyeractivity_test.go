// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Remediation work must never read as buyer engagement.
//
// Forecast assurance files review tasks against a deal. They are activities,
// and the recency clocks the product reads — last_activity_at on deal, person,
// organization and project — are folded by four SQL functions over the activity
// table. If a review task counted, the system asking why a deal went quiet
// would refresh that deal's clock and the staleness rule that raised the
// question would stop firing. The engine would switch itself off, one deal at
// a time, and nothing would fail.
//
// This is a census over the SQL that computes those clocks, and it fails SHORT
// by construction if it finds nothing to check — which is the failure a census
// cannot notice about itself. So it asserts the corpus size too: four functions
// exist, and every one of them carries the exclusion. Adding a fifth recency
// function without the clause fails here rather than in a forecast six months
// later.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// recencyFunctions are the SQL functions that answer "when did the other side
// last engage". The list is asserted, not just iterated: a new one that skips
// the exclusion is exactly the defect this gate exists to catch.
var recencyFunctions = []string{
	"last_activity_of_deal",
	"last_activity_of_person",
	"last_activity_of_organization",
	"last_activity_of_project",
}

const remediationOrigin = "system_remediation"

// workspaceAudienceClause is the other clause these bodies carry. This gate
// guards it because it lives in the same statements and is lost the same way.
const workspaceAudienceClause = "a.audience = 'workspace'"

// TestRecencyFunctionsExcludeRemediationWork reads the migration that owns each
// function's CURRENT body — the newest CREATE OR REPLACE wins, the same way
// Postgres resolves it — and requires the exclusion in every arm.
func TestRecencyFunctionsExcludeRemediationWork(t *testing.T) {
	t.Parallel()

	bodies := currentRecencyBodies(t)

	if len(bodies) != len(recencyFunctions) {
		t.Fatalf("found %d recency functions, expected %d (%v) — a census that "+
			"reads a smaller tree reports PASS and there is no failing assertion "+
			"to notice", len(bodies), len(recencyFunctions), recencyFunctions)
	}

	for _, name := range recencyFunctions {
		body, ok := bodies[name]
		if !ok {
			t.Errorf("%s: no definition found in migrations", name)
			continue
		}
		// Every arm, not the function: last_activity_of_organization unions
		// three populations, and an exclusion on two of them still lets a
		// review task filed against a deal refresh its account.
		arms := strings.Count(body, "activity a ON a.id = l.activity_id")
		clauses := strings.Count(body, remediationOrigin)
		if arms == 0 {
			t.Errorf("%s: no activity join found — the census cannot see this "+
				"function's shape, so it cannot vouch for it", name)
			continue
		}
		if clauses < arms {
			t.Errorf("%s: %d activity arm(s) but only %d %q exclusion(s) — an "+
				"unexcluded arm lets remediation work refresh this clock",
				name, arms, clauses, remediationOrigin)
		}
		// CREATE OR REPLACE rewrites the WHOLE body, so a definition copied
		// from an older migration silently drops whatever a newer one added.
		// The audience filter is the live example: it was added because a
		// participants-only message was moving a date every colleague could
		// see, and a careless replace here would re-open that. Asserting it
		// arm-for-arm means this gate fails rather than the privacy test
		// failing somewhere else.
		if audience := strings.Count(body, workspaceAudienceClause); audience < arms {
			t.Errorf("%s: %d activity arm(s) but only %d audience filter(s) — a "+
				"replacement copied from a stale definition dropped one",
				name, arms, audience)
		}
	}
}

// currentRecencyBodies returns each function's newest definition, keyed by name.
func currentRecencyBodies(t *testing.T) map[string]string {
	t.Helper()
	dir := filepath.Join("migrations", "core")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			names = append(names, e.Name())
		}
	}
	// Filenames lead with the unix second they were written, so lexical order
	// is apply order and the last definition read is the live one.
	sortStrings(names)

	bodies := map[string]string{}
	for _, fn := range names {
		raw, err := os.ReadFile(filepath.Join(dir, fn))
		if err != nil {
			t.Fatalf("reading %s: %v", fn, err)
		}
		for _, want := range recencyFunctions {
			if body, ok := extractFunctionBody(string(raw), want); ok {
				bodies[want] = body
			}
		}
	}
	return bodies
}

var funcHeadRe = regexp.MustCompile(`CREATE (?:OR REPLACE )?FUNCTION\s+(\w+)`)

// extractFunctionBody returns the text between the function's $$ delimiters.
func extractFunctionBody(sql, name string) (string, bool) {
	for _, m := range funcHeadRe.FindAllStringSubmatchIndex(sql, -1) {
		if sql[m[2]:m[3]] != name {
			continue
		}
		rest := sql[m[1]:]
		open := strings.Index(rest, "$$")
		if open < 0 {
			continue
		}
		end := strings.Index(rest[open+2:], "$$")
		if end < 0 {
			continue
		}
		return rest[open+2 : open+2+end], true
	}
	return "", false
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// recencyReadersOutsideTheHelpers are the Go queries that compute "when was
// this last touched" WITHOUT going through the four SQL helpers. Each one is a
// second spelling of the same question, so each carries the same exclusion.
//
// This half of the census exists because the SQL half cannot see them: a
// reader that runs its own max(occurred_at) is invisible to a gate that only
// reads migrations, and it was four such readers — not the helpers — that
// still counted remediation work when this column first landed.
// engagementFragment is the shared exclusion these readers call instead of
// spelling an origin themselves (internal/platform/auth/engagementorigin.go).
const engagementFragment = "auth.OriginIsEngagement"

// gatekit:fixture the reader files this census covers and what each computes —
// expected data naming the subjects, not waivers excusing them.
var recencyReadersOutsideTheHelpers = map[string]string{
	"internal/modules/activities/lasttouch.go":       "genuine engagement for the quiet-record scan",
	"internal/modules/deals/health.go":               "the record behind deal.last_activity_at",
	"internal/modules/activities/projectcoverage.go": "Project 360's last activity",
	"internal/modules/people/lead_read.go":           "the lead's last-touch clock",
}

// TestRecencyReadersOutsideTheHelpersExcludeRemediation holds the Go side.
func TestRecencyReadersOutsideTheHelpersExcludeRemediation(t *testing.T) {
	t.Parallel()

	for file, what := range recencyReadersOutsideTheHelpers {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Errorf("%s (%s): %v — a reader named here must exist, or this "+
				"census is guarding a file that moved", file, what, err)
			continue
		}
		// EITHER spelling satisfies this. A reader may name the origin itself,
		// or call the shared fragment that names every system origin — which is
		// what all four do now, so that a THIRD system origin reaches them
		// without anyone having to remember these four files.
		//
		// The fragment is the stronger form and is held separately
		// (TestEveryRecencyReadingExcludesTheSystemOrigins asserts no reader
		// spells the exclusion by hand, and TestTheOneSpellingExcludesEvery-
		// SystemOrigin holds the fragment to the vocabulary). This census stays
		// because it names WHICH files ask the question at all — the fact a
		// pattern-matching gate cannot recover.
		body := string(raw)
		if !strings.Contains(body, remediationOrigin) && !strings.Contains(body, engagementFragment) {
			t.Errorf("%s computes %s and neither mentions %q nor calls %s — a second spelling "+
				"of last_activity_at that still counts work the product filed "+
				"about the record", file, what, remediationOrigin, engagementFragment)
		}
	}
}
