// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A reader that COUNTS messages asks the audience, exactly as one that shows
// them does.
//
// A held message must not be visible in the shape of an aggregate either. A
// relationship-strength score counting a founder's correspondence with their
// lawyer tells every colleague the relationship is strong; a last-touch that
// moves when a private message arrives tells them when it arrived. Neither
// shows a word, and both disclose it.
//
// The readers are named rather than discovered, because what makes one of these
// an aggregate is what it MEANS rather than anything a scanner can see: they
// select ids, timestamps and counts, so the content-projection gate beside this
// one correctly passes them. A new aggregate joins this list by hand — and the
// list is short enough that a reviewer notices when it should have grown.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// aggregateReaders are the files whose numbers a colleague reads about mail
// they may not, with what each one exposes.
//
// gatekit:fixture the subject list — each value says WHICH number a reader sees,
// so a failure names the disclosure rather than only the file. These are not
// waived costs: every entry must satisfy the rule, and none is exempt from it.
var aggregateReaders = map[string]string{
	"internal/modules/people/strength.go":            "relationship strength and warmth, shown on a person and an account to any seat",
	"internal/modules/deals/health.go":               "the deal's recency score, shown to anybody who can open the deal",
	"internal/compose/meetingbrief/meeting.go":       "last-touch per attendee, the number a brief's reader trusts most",
	"internal/modules/search/graphedge.go":           "the global who-knows-whom projection, readable by everyone",
	"internal/modules/capture/digest.go":             "the weekly digest's counts of what came in",
	"internal/compose/person360/sectionstimeline.go": "the person page's last-inbound/last-outbound dates, read by any seat that can open the record",
}

func TestEveryAggregateAsksTheAudience(t *testing.T) {
	t.Parallel()
	for path, why := range aggregateReaders {
		body, err := os.ReadFile(filepath.Join(strings.Split(path, "/")...))
		if err != nil {
			t.Errorf("reading %s (%s): %v — a named aggregate that moved must be re-pointed here, "+
				"not dropped, or the rule stops being checked for it", path, why, err)
			continue
		}
		text := string(body)
		// Either spelling: the shared helper, or the literal for a statement
		// assembled with positional verbs where a concatenated fragment would
		// land inside one.
		if !strings.Contains(text, "AudienceWorkspaceOnly") &&
			!strings.Contains(text, "audience = 'workspace'") {
			t.Errorf("%s counts activities and asks no audience question (%s). "+
				"A held message must not be visible in the shape of a number either — "+
				"compose auth.AudienceWorkspaceOnly, or spell the literal where a "+
				"concatenated fragment cannot go", path, why)
		}
	}
}
