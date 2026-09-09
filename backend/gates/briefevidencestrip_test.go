// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// "Never persist an email summary" is spelled once, and every writer that
// caches evidence says it that way.
//
// An email summary is the one reader-scoped field on an otherwise
// reader-independent row: it is assembled out of ONE reader's audience and
// content grants. Two shapes in this tree are both enriched and cached — the
// deal status card and the account scan's settled findings — so a summary that
// rode into either store would be served back to whoever read the cache next,
// out of the first reader's access.
//
// briefevidence.Strip is that rule. The risk this census exists for is not a
// writer that forgets to strip — the persistence tests catch that — but a
// writer that strips BY HAND, walking the evidence itself and setting the field
// to nil. Such a writer is correct on the day it is written and silently
// partial the moment a producer grows a shape it does not know about, which is
// exactly what happened when the deal move turned out to carry its own
// evidence type. Two spellings of one rule is one spelling and one omission.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// cachingWriters are the packages that persist a shape carrying evidence.
// Named rather than derived: adding a third cache of grounded prose is exactly
// the deliberate act this gate wants somebody to make, and a derived corpus
// would admit one silently.
var cachingWriters = []string{
	"internal/compose/dealstatus",
	"internal/compose/companyscan",
}

// Every caching writer reaches the shared strip.
func TestEveryCacheOfGroundedProseStripsThroughTheSharedRule(t *testing.T) {
	t.Parallel()
	for _, pkg := range cachingWriters {
		if !strings.Contains(sourcesOf(t, pkg), "briefevidence.Strip(") {
			t.Errorf("%s persists a shape carrying evidence and never calls "+
				"briefevidence.Strip — a summary stored there serves the writer's own "+
				"mail access to whoever reads the cache next", filepath.Base(pkg))
		}
	}
}

// And nobody spells the rule a second time by hand.
//
// A hand-rolled strip is a second answer to "which fields hold a summary", free
// to disagree with the collectors the moment a producer grows a shape it does
// not know about.
func TestNobodyClearsAnEmailSummaryByHand(t *testing.T) {
	t.Parallel()
	// An assignment of nil to the field, in any of the spellings a writer
	// would reach for. The briefevidence package itself is where the rule
	// lives, so it is the one place allowed to say this.
	byHand := regexp.MustCompile(`\.EmailSummary\s*=\s*nil`)
	for _, pkg := range cachingWriters {
		for _, line := range strings.Split(sourcesOf(t, pkg), "\n") {
			if byHand.MatchString(line) {
				t.Errorf("%s clears an email summary by hand (%q) — call "+
					"briefevidence.Strip, which is derived from the same collectors that "+
					"fill the field and cannot fall out of step with them",
					filepath.Base(pkg), strings.TrimSpace(line))
			}
		}
	}
}

// sourcesOf is a package's non-test Go, concatenated.
func sourcesOf(t *testing.T, pkg string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(pkg, "*.go"))
	if err != nil {
		t.Fatalf("listing %s: %v", pkg, err)
	}
	// A census that can fail short has already failed: a moved package would
	// leave this reading nothing and reporting clean about every writer at once.
	if len(matches) == 0 {
		t.Fatalf("%s holds no Go files — the package moved and this census is judging nothing", pkg)
	}
	var out strings.Builder
	for _, file := range matches {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("reading %s: %v", file, err)
		}
		out.Write(source)
		out.WriteString("\n")
	}
	return out.String()
}
