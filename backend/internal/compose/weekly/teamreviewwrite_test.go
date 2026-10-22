// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// A table hanging off team_weekly_review, in the committed catalog.
var teamReviewChildFK = regexp.MustCompile(
	`^public\.(\w+)\.\w+ FOREIGN KEY \(team_weekly_review_id\) REFERENCES team_weekly_review\(id\)`)

// TestTheReplacementEmptiesEveryChildOfTheTeamReview derives the children from
// the catalog. A child table added later and left off the replacement list
// fails here instead of surviving into the corrected snapshot.
func TestTheReplacementEmptiesEveryChildOfTheTeamReview(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../../migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatal(err)
	}
	var children []string
	for line := range strings.SplitSeq(string(raw), "\n") {
		if m := teamReviewChildFK.FindStringSubmatch(line); m != nil {
			children = append(children, m[1])
		}
	}
	if len(children) < 4 {
		t.Fatalf("found %d children of team_weekly_review in the catalog: the scan stopped seeing them", len(children))
	}
	slices.Sort(children)
	listed := slices.Sorted(slices.Values(teamReviewChildTables))
	if !slices.Equal(children, listed) {
		t.Errorf("the catalog's children of team_weekly_review are %v and the replacement empties %v", children, listed)
	}
}
