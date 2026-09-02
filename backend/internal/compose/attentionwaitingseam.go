// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The who-is-waiting lane's binding, and the guardrail over its own hiding
// rules.
//
// Its own file because it is the one lane read BESIDE the assembled day rather
// than as one of its fourteen — it carries its own truncation answer, its own
// machine-sender filter and, now, a second question about the same query. The
// sibling seams in attentionlanesseam.go are each a single pass-through.

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
)

// attentionWaiting binds the who-is-waiting lane to the activities module's
// own gated read. The thread walk, the discover gate and the audience arm all
// live there; nothing about who may see what is decided here.
type attentionWaiting struct {
	store *activities.Store
	now   attention.Clock
}

// The instant comes from the caller so the whole read is one snapshot. Asking
// the clock again here would let the anti-joins judge against a moment the rest
// of the day was not read at.
// Answered asks the module how fast it replied over a window. A pass-through,
// like Hidden: the median and the counts are SQL and belong beside the query.
func (w attentionWaiting) Answered(
	ctx context.Context, from, to time.Time,
) (attention.AnsweredWork, error) {
	got, err := w.store.ResponseWindow(ctx, from, to)
	if err != nil {
		return attention.AnsweredWork{}, err
	}
	return attention.AnsweredWork{
		Answered:         got.Answered,
		MedianMinutes:    got.MedianMinutes,
		Disposed:         got.Disposed,
		DisposedNotSales: got.DisposedNotSales,
	}, nil
}

// Hidden asks the module what its own hiding rules are keeping off the queue.
//
// A pass-through: the arithmetic is five reads of the eligibility query and
// belongs beside that query, not here. What this seam does is what every seam
// here does — carry the answer across in compose's own vocabulary.
func (w attentionWaiting) Hidden(
	ctx context.Context, asOf time.Time,
) (attention.HiddenWork, error) {
	got, err := w.store.HiddenWaiting(ctx, asOf)
	if err != nil {
		return attention.HiddenWork{}, err
	}
	return attention.HiddenWork{
		Shown:       got.Shown,
		SetAside:    got.SetAside,
		NotSales:    got.NotSales,
		PastHorizon: got.PastHorizon,
		Unlinked:    got.Unlinked,
		Truncated:   got.Truncated,
	}, nil
}

func (w attentionWaiting) Unanswered(
	ctx context.Context, asOf time.Time,
) ([]attention.WaitingCustomer, bool, error) {
	rows, err := w.store.WaitingReplies(ctx, asOf)
	if err != nil {
		return nil, false, err
	}
	// Asked of what the STORE returned, before keepWaitingCustomers runs.
	//
	// That filter drops machine senders and folds duplicate threads, so what it
	// returns is smaller than what was read — and a caller comparing the
	// SURVIVORS against the scan bound would read a full scan whose survivors
	// are few as a complete one. This is the only place both numbers exist.
	cut := len(rows) >= activities.WaitingScanCap
	out := make([]attention.WaitingCustomer, 0, len(rows))
	for _, row := range keepWaitingCustomers(rows) {
		out = append(out, attention.WaitingCustomer{
			ActivityID:     row.ActivityID,
			Subject:        row.Subject,
			Since:          row.OccurredAt,
			PersonID:       row.PersonID,
			OrganizationID: row.OrganizationID,
			DealID:         row.DealID,
			HasOpenDeal:    row.HasOpenDeal,
			OwnerID:        row.OwnerID,
		})
	}
	return out, cut, nil
}

// keepWaitingCustomers keeps the rows that are a PERSON waiting on this reader.
//
// Two rules, both learned from the live page.
//
// A machine is not a customer. Judged by capture's own address rule rather than
// a second one spelled here: an e-signature notification, a shared-folder
// notice and a booking confirmation opened a rep's day, and a queue that asks
// somebody to answer a no-reply address teaches them to stop reading it.
//
// One subject FROM ONE SENDER is one row. A notification service sends the same
// request on several threads, and two rows reading identically are two
// obligations to somebody scanning the page.
//
// Keyed on sender AND subject, never subject alone: two customers both writing
// "Re: proposal" are two people waiting, and folding them would drop the second
// one silently — the worst failure this queue has, because nothing on the page
// would say a customer had been hidden.
//
// An UNTITLED message is never folded, because several untitled waits are
// several customers and collapsing them would hide all but one behind an empty
// string.
func keepWaitingCustomers(rows []activities.WaitingReply) []activities.WaitingReply {
	kept := make([]activities.WaitingReply, 0, len(rows))
	seen := make(map[string]bool, len(rows))
	for _, row := range rows {
		if capture.IsMachineAddress(row.Sender) {
			continue
		}
		if row.Subject != "" {
			key := row.Sender + "\x00" + row.Subject
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		kept = append(kept, row)
	}
	return kept
}
