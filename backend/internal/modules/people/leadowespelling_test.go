// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// "This lead still owes a first reply" is spelled once.
//
// Four readers ask it — the SLA state filter, the breach scan, the work
// queue's band, and the list's owed dial — and a fifth writing the predicate by
// hand is how a queue starts disagreeing with the filter that feeds it. The
// figures then differ by rows nobody can account for, which is precisely the
// defect this predicate was extracted to end.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Held by: this test, named in leadOwesAReplySQL's own doc comment.
func TestTheOwesAReplyPredicateHasOneSpelling(t *testing.T) {
	t.Parallel()
	// The shape a hand-written second spelling takes: the two column tests
	// together, in either order, however spaced.
	handWritten := regexp.MustCompile(
		`first_response_at\s+IS\s+NULL[\s\S]{0,80}archived_at\s+IS\s+NULL|` +
			`archived_at\s+IS\s+NULL[\s\S]{0,80}first_response_at\s+IS\s+NULL`)

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package: %v", err)
	}
	var scanned int
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		scanned++
		for _, hit := range handWritten.FindAllString(string(body), -1) {
			// The constant's own declaration is the one permitted spelling.
			if strings.Contains(hit, leadOwesAReplySQL) && name == "leadsla.go" {
				continue
			}
			t.Errorf("%s writes the owes-a-reply predicate by hand (%q) — "+
				"use leadOwesAReplySQL so every reader asks one question",
				name, strings.Join(strings.Fields(hit), " "))
		}
	}
	// A census that reads nothing reports PASS with nothing to notice.
	if scanned < 20 {
		t.Fatalf("scanned %d files of this package, far below its size — the walk lost its source", scanned)
	}
}
