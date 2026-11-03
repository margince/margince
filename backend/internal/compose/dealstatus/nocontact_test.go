// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

const noContactShown = "No exchange is shown in this deal’s recent history"

// A note or an unheld meeting does not prove that contact took place.
func TestRecentHistoryDoesNotClaimThatNoContactEverHappened(t *testing.T) {
	canceled := crmcontracts.ActivityMeetingStatusCanceled
	called := act(crmcontracts.ActivityKindMeeting, testNow.AddDate(0, 0, -2))
	called.MeetingStatus = &canceled

	for name, timeline := range map[string][]crmcontracts.Activity{
		"a note":             {act(crmcontracts.ActivityKindNote, testNow.AddDate(0, 0, -1))},
		"a booked meeting":   {act(crmcontracts.ActivityKindMeeting, testNow.AddDate(0, 0, 3))},
		"a canceled meeting": {called},
	} {
		t.Run(name, func(t *testing.T) {
			f := facts{deal: openDeal(), now: testNow, loc: time.UTC, timeline: timeline}
			texts := []string{nextStepReason(f)}
			for _, l := range storyLines(f) {
				texts = append(texts, l.Text)
			}
			for _, text := range texts {
				if strings.Contains(text, "Nothing has been logged") {
					t.Errorf("a deal with %s was told %q", name, text)
				}
			}
			if got := nextStepReason(f); !strings.Contains(got, noContactShown) {
				t.Errorf("next step reason %q does not say what is missing", got)
			}
			if got := storyLines(f)[1].Text; got != noContactShown+"." {
				t.Errorf("story line %q, want %q", got, noContactShown+".")
			}
		})
	}
}
