// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// A note is on the timeline but is no contact. A deal holding only notes must
// not be told that nothing has been logged on it.
func TestADealWithOnlyNotesIsNotToldNothingWasLogged(t *testing.T) {
	f := facts{
		deal: openDeal(), now: testNow,
		timeline: []crmcontracts.Activity{act(crmcontracts.ActivityKindNote, testNow.AddDate(0, 0, -1))},
	}
	texts := []string{nextStepReason(f)}
	for _, l := range storyLines(f) {
		texts = append(texts, l.Text)
	}
	for _, text := range texts {
		if strings.Contains(text, "Nothing has been logged") {
			t.Errorf("a deal with a note was told %q", text)
		}
	}
	if got := nextStepReason(f); !strings.Contains(got, noContactLogged) {
		t.Errorf("next step reason %q does not say what is missing", got)
	}
}
