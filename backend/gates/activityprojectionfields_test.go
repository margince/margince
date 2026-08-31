// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every hand-written SELECT over `activity` that projects the row for a client
// must carry the SAME audience columns as the shared projection.
//
// `activities/activityprojection.go` is the one list every reader is supposed
// to go through, and most do. A record page does not: its 360 read assembles
// its own statement, and the timeline it seeds is drawn from that. So the two
// statements are two answers to "what does a client learn about this message",
// and they drift silently — the reader sees a row with no reason and no way to
// tell a held message from an open one.
//
// That is not hypothetical. `audience_reason` shipped in the shared projection
// and was absent from the person 360's copy, so the record timeline — the one
// screen where an owner decides whether to share a thread — never received it.
// Nothing failed: the field is optional on the wire, so its absence reads
// exactly like a row that has no reason.
//
// The corpus is DERIVED from what a file DOES, not from a list: a file that
// both scans into a `crmcontracts.Activity` and selects `a.audience` is
// assembling the client's view of a message, so a new 360 or export written the
// same way joins this census on the commit it is written (AGENTS.md rule 8).
// A file that reads the audience for its own purposes — the rescope consumer
// deciding what to retract — projects nothing to a client and is not a subject.

import (
	"regexp"
	"strings"
	"testing"
)

// Selecting the audience as a COLUMN, rather than filtering on it. RE2 has no
// negative lookahead, so "not a predicate" is spelled by requiring a comma or a
// newline where `= 'workspace'` would otherwise sit.
var audienceSelected = regexp.MustCompile(`\ba\.audience\s*[,\n]`)

// The reason must travel with it: a client handed the audience without the
// reason is told a held message is an ordinary one.
var reasonSelected = regexp.MustCompile(`\ba\.audience_reason\b`)

// What makes a file a CLIENT projection: it fills the contract type the API
// serves. A file reading the audience for its own logic does not.
var buildsClientActivity = regexp.MustCompile(`crmcontracts\.Activity\b`)

func TestEveryActivityProjectionCarriesTheAudienceColumns(t *testing.T) {
	t.Parallel()
	var missing []string
	goFilesUnderTree(t, func(path, body string) {
		// The shared projection builds its column list from a table rather
		// than spelling a SELECT, so it is the authority here, not a subject.
		if strings.HasSuffix(path, "activityprojection.go") {
			return
		}
		if !buildsClientActivity.MatchString(body) {
			return
		}
		if audienceSelected.MatchString(body) && !reasonSelected.MatchString(body) {
			missing = append(missing,
				path+" selects a.audience into a crmcontracts.Activity without a.audience_reason")
		}
	})
	if len(missing) > 0 {
		t.Fatalf("an activity projection that omits an audience column tells the "+
			"reader a held message is an ordinary one, and nothing downstream "+
			"fails because the field is optional on the wire:\n  %s",
			strings.Join(missing, "\n  "))
	}
}
