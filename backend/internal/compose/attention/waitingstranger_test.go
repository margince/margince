// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// A first-time sender's Bcc or foreign-link mail is routine work, even when
// the classifier read it as a request. The same mail from a known sender ranks
// as before.

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// strangerRequest is a two-day-old mail the classifier judged as asking us,
// from a sender the workspace never wrote to, with no thread history or deal.
func strangerRequest() WaitingCustomer {
	return WaitingCustomer{
		ActivityID:        ids.MustParse("01a05500-0000-7000-8000-0000000000e1"),
		Subject:           "View the document",
		Since:             rankInstant.Add(-2 * 24 * time.Hour),
		ConfirmedRequest:  true,
		SenderWasStranger: true,
	}
}

func becauseKinds(row ranked) map[crmcontracts.WorklistReasonKind]bool {
	kinds := map[crmcontracts.WorklistReasonKind]bool{}
	for _, because := range row.item.Because {
		kinds[because.Kind] = true
	}
	return kinds
}

func TestAStrangersBccRequestIsRoutineAndSaysWhy(t *testing.T) {
	waiting := strangerRequest()
	waiting.BccOnly = true

	row := classifyWaiting(waiting, rankInstant)

	if row.item.Level != levelRoutine {
		t.Fatalf("a stranger's Bcc request ranked at level %d, want routine (%d)", row.item.Level, levelRoutine)
	}
	if kinds := becauseKinds(row); !kinds["bcc_from_stranger"] || kinds["link_off_sender"] {
		t.Errorf("the row's reasons are %v, want bcc_from_stranger and no link reason", kinds)
	}
}

func TestAStrangersRequestWithAForeignLinkIsRoutineAndSaysWhy(t *testing.T) {
	waiting := strangerRequest()
	waiting.ForeignLink = true

	row := classifyWaiting(waiting, rankInstant)

	if row.item.Level != levelRoutine {
		t.Fatalf("a stranger's foreign-link request ranked at level %d, want routine (%d)", row.item.Level, levelRoutine)
	}
	if kinds := becauseKinds(row); !kinds["link_off_sender"] || kinds["bcc_from_stranger"] {
		t.Errorf("the row's reasons are %v, want link_off_sender and no Bcc reason", kinds)
	}
}

// The signals alone demote nothing. Each case makes the sender known in one
// way and keeps both signals set, so only that one fact keeps the top band.
func TestAKnownSendersRequestKeepsTheTopBandWhateverItsEnvelope(t *testing.T) {
	cases := map[string]func(*WaitingCustomer){
		"the workspace wrote to the sender before": func(w *WaitingCustomer) { w.SenderWasStranger = false },
		"the workspace wrote on this thread":       func(w *WaitingCustomer) { w.Engaged = true },
		"an open deal is on the thread":            func(w *WaitingCustomer) { w.HasOpenDeal = true },
	}
	for name, known := range cases {
		t.Run(name, func(t *testing.T) {
			waiting := strangerRequest()
			waiting.BccOnly, waiting.ForeignLink = true, true
			known(&waiting)

			row := classifyWaiting(waiting, rankInstant)

			if row.item.Level != levelWaiting {
				t.Fatalf("a known sender's request ranked at level %d, want the top band (%d)", row.item.Level, levelWaiting)
			}
			if kinds := becauseKinds(row); kinds["bcc_from_stranger"] || kinds["link_off_sender"] {
				t.Errorf("a known sender's row gave a stranger reason: %v", kinds)
			}
		})
	}
}

// A stranger's ordinary request, addressed to the reader with no foreign link,
// is the case the top band exists for.
func TestAStrangersOpenlyAddressedRequestKeepsTheTopBand(t *testing.T) {
	row := classifyWaiting(strangerRequest(), rankInstant)

	if row.item.Level != levelWaiting {
		t.Fatalf("a stranger's plain request ranked at level %d, want the top band (%d)", row.item.Level, levelWaiting)
	}
}
