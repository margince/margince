// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/draftcore"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const seatCopyFollowUp = "and the countersigned annex too"

// The thread a reply drafter reads counts a mail both mailboxes hold as one
// turn, whichever copy the reply answers. The answered copy is the anchor, and
// so is the other one.
func TestAReplyThreadHoldsAMailBothMailboxesCapturedOnce(t *testing.T) {
	s := newSeatCopyEnv(t, seatCopyPostures{first: true, second: true})
	s.first(t)
	s.second(t)
	// A second message of the thread, so the read is seen to run.
	s.syncFirst(t, []byte(strings.Join([]string{
		"From: " + seatCopySender, "To: " + captureOwner, "Subject: Re: Telematics renewal",
		"Date: Wed, 04 Jun 2026 09:00:00 +0000", "Message-ID: <follow-up@acme.example>",
		"In-Reply-To: <" + seatCopyKey + ">", "References: <" + seatCopyKey + ">",
		"Content-Type: text/plain", "", seatCopyFollowUp, "",
	}, "\r\n")))
	copies := capturedMailRows(t, s.e)[:2]
	store := activities.NewStore(s.e.DB())
	for _, reader := range []ids.UUID{s.e.Rep1, s.e.Rep3} {
		ctx := readerCtx(s.e, reader)
		for _, copied := range copies {
			anchorID := ids.From[ids.ActivityKind](copied.ID)
			anchor, err := store.GetActivityContent(ctx, anchorID, storekit.LiveOnly)
			if err != nil {
				t.Fatalf("reading %s as %s: %v", copied.ID, reader, err)
			}
			// As the drafter asks it: a mail with no reply address reads the thread without one.
			delivery, err := store.ReplyAddresseeFor(ctx, anchorID, nil)
			if _, none := errors.AsType[*activities.NoReplyAddressError](err); err != nil && !none {
				t.Fatalf("reading who %s was delivered to: %v", copied.ID, err)
			}
			rows, err := draftcore.ThreadRows(ctx, store, anchor, delivery, captureNow.Add(24*time.Hour), 60)
			if err != nil {
				t.Fatalf("reading the thread of %s as %s: %v", copied.ID, reader, err)
			}
			bodies := map[string]int{}
			for _, row := range rows {
				if row.Body != nil {
					bodies[seatCopyBody] += strings.Count(*row.Body, seatCopyBody)
					bodies[seatCopyFollowUp] += strings.Count(*row.Body, seatCopyFollowUp)
				}
			}
			if bodies[seatCopyFollowUp] != 1 || bodies[seatCopyBody] != 0 {
				t.Errorf("reply to %s as %s: the thread holds the follow-up %d times and the answered mail's other copy %d times, want 1 and 0",
					copied.ID, reader, bodies[seatCopyFollowUp], bodies[seatCopyBody])
			}
		}
	}
}
