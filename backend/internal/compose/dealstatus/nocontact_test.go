// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// A note, a booked meeting and a canceled one are on the timeline but are no
// contact. A deal holding only those must not be told that nothing is logged.
func TestADealWithNoContactIsNotToldNothingWasLogged(t *testing.T) {
	canceled := crmcontracts.ActivityMeetingStatusCanceled
	called := act(crmcontracts.ActivityKindMeeting, testNow.AddDate(0, 0, -2))
	called.MeetingStatus = &canceled

	for name, timeline := range map[string][]crmcontracts.Activity{
		"a note":             {act(crmcontracts.ActivityKindNote, testNow.AddDate(0, 0, -1))},
		"a booked meeting":   {act(crmcontracts.ActivityKindMeeting, testNow.AddDate(0, 0, 3))},
		"a canceled meeting": {called},
	} {
		t.Run(name, func(t *testing.T) {
			f := facts{deal: openDeal(), now: testNow, timeline: timeline}
			texts := []string{nextStepReason(f)}
			for _, l := range storyLines(f) {
				texts = append(texts, l.Text)
			}
			for _, text := range texts {
				if strings.Contains(text, "Nothing has been logged") {
					t.Errorf("a deal with %s was told %q", name, text)
				}
			}
			if got := nextStepReason(f); !strings.Contains(got, noContactYet) {
				t.Errorf("next step reason %q does not say what is missing", got)
			}
			if got := storyLines(f)[1].Text; got != noContactYet+"." {
				t.Errorf("story line %q, want %q", got, noContactYet+".")
			}
		})
	}
}
