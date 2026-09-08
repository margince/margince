// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// "This meeting is that rep's" is spelled once.
//
// Two panels ask it — the headline count and the lead funnel — and they sit on
// one page. Spelled separately they drift, and a meeting credited to different
// people by two panels is the page contradicting itself about the same week,
// which is the defect the shared predicate was extracted to end.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Held by: this test, named in meetingIsTheirsSQL's own doc comment.
func TestTheMeetingAttributionHasOneSpelling(t *testing.T) {
	t.Parallel()
	// The shape a second, hand-written spelling takes: the host column tested
	// against the recorder column in one predicate.
	handWritten := regexp.MustCompile(`host_user_id[\s\S]{0,120}captured_by`)

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
		if name == "weeklycounts.go" {
			// The one permitted spelling, in the helper itself.
			continue
		}
		if hit := handWritten.FindString(string(body)); hit != "" {
			t.Errorf("%s writes the meeting attribution by hand (%q) — call "+
				"meetingIsTheirsSQL so both panels credit one meeting to one person",
				name, strings.Join(strings.Fields(hit), " "))
		}
	}
	// A census that reads nothing reports PASS with nothing to notice.
	if scanned < 8 {
		t.Fatalf("scanned %d files of this package, below its size — the walk lost its source", scanned)
	}
}
