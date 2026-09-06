// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Which waits offer to answer, and which do not.
//
// The verb opens the MAIL composer, so both conditions it rides are about
// whether that composer can do anything: the wait has to be an email, and it
// has to name a record the sent message files against.

import (
	"slices"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestOnlyAWaitTheComposerCanAnswerOffersReply(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC)
	summary := &crmcontracts.EmailSummary{}
	person := ids.NewV7()

	for _, tc := range []struct {
		name    string
		waiting WaitingCustomer
		offers  bool
		because string
	}{
		{
			name: "an email naming a person",
			waiting: WaitingCustomer{
				ActivityID: ids.NewV7(), Subject: "can you resend the quote?",
				Since: at.Add(-48 * time.Hour), EmailSummary: summary, PersonID: person,
			},
			offers:  true,
			because: "the composer can answer it and knows where to file the answer",
		},
		{
			name: "a channel message naming a person",
			waiting: WaitingCustomer{
				ActivityID: ids.NewV7(), Subject: "can you resend the quote?",
				Since: at.Add(-48 * time.Hour), PersonID: person,
			},
			offers: false,
			because: "the lane spans channel messages too, and answering a chat in the " +
				"mail composer replies in the wrong place, to a counterparty resolved " +
				"from a thread that is not one",
		},
		{
			name: "an email from a stranger",
			waiting: WaitingCustomer{
				ActivityID: ids.NewV7(), Subject: "hello",
				Since: at.Add(-48 * time.Hour), EmailSummary: summary,
			},
			offers: false,
			because: "no record is named, so the composer would open with nothing to " +
				"link the sent message to",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			row := classifyWaiting(tc.waiting, at).item
			offered := slices.Contains(row.Actions, crmcontracts.WorklistItemActionsReply)
			if offered != tc.offers {
				t.Fatalf("offers reply = %v, want %v: %s", offered, tc.offers, tc.because)
			}
		})
	}
}
