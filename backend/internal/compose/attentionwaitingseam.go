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
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
		Colleagues:  got.Colleagues,
		Truncated:   got.Truncated,
	}, nil
}

func (w attentionWaiting) Unanswered(
	ctx context.Context, asOf time.Time,
) ([]attention.WaitingCustomer, bool, error) {
	kept, cut, err := w.waitingPages(ctx, asOf)
	if err != nil {
		return nil, false, err
	}
	summaries, err := w.emailRows(ctx, kept)
	if err != nil {
		return nil, false, err
	}
	out := make([]attention.WaitingCustomer, 0, len(kept))
	for _, row := range kept {
		// Nil when this wait is not an email, or is one whose content the
		// reader may not read. A zero-valued struct would be a row claiming an
		// empty subject and a `team` badge, which is a message rather than the
		// absence of one.
		var summary *crmcontracts.EmailSummary
		if got, ok := summaries[row.ActivityID]; ok {
			summary = &got
		}
		out = append(out, attention.WaitingCustomer{
			ActivityID:         row.ActivityID,
			EmailSummary:       summary,
			Subject:            row.Subject,
			Since:              row.OccurredAt,
			ContactID:          row.ContactID,
			CompanyID:          row.CompanyID,
			DealID:             row.DealID,
			HasOpenDeal:        row.HasOpenDeal,
			Engaged:            row.Engaged,
			Threaded:           row.Threaded,
			AddressedElsewhere: row.AddressedElsewhere,
			// Translated here, at the one boundary that already crosses from
			// the module's vocabulary to the queue's. Only "informs us" changes
			// a ranking; unjudged and "asks us" both leave it alone, so the
			// queue never needs the word.
			AsksNothing:       row.OwedVerdict == activities.OwedVerdictInformsUs,
			ConfirmedRequest:  row.OwedVerdict == activities.OwedVerdictAsksUs,
			ActionUnconfirmed: row.OwedVerdict == "",
			OwnerID:           row.OwnerID,
		})
	}
	return out, cut, nil
}

// waitingRefillRounds bounds how many pages one assembly will read.
//
// Three, not "until enough": each page is a full scan of the waiting predicate,
// and a workspace whose recent mail is ENTIRELY machine would otherwise walk
// its whole history to fill a queue that has nothing to show. Three pages is
// six hundred rows, which is past any flood a real installation produces and
// still one read of bounded cost.
const waitingRefillRounds = 3

// waitingPages reads waiting rows until enough survive the filter, the scan
// runs out, or the round ceiling is reached. It answers what survived and
// whether anything was left unread.
//
// The refill exists because the filter runs AFTER the scan cap. The store's own
// machine rule is a coarse subset — six patterns against an address — while
// keepWaitingCustomers asks capture.IsMachineAddress, which reads a registrable
// domain against the transactional baseline. An address like hello@sendgrid.net
// matches none of the six, fills a slot under the cap, and is discarded here.
// Two hundred of those and a genuinely waiting customer never appears at all.
//
// `cut` still means what it meant: something was left unread. It is now true
// only when the LAST page was also full, so a refill that reached the end of
// the matching rows reports a complete scan rather than inheriting the first
// page's truncation.
func (w attentionWaiting) waitingPages(ctx context.Context, asOf time.Time) ([]activities.WaitingReply, bool, error) {
	var kept []activities.WaitingReply
	var before time.Time
	cut := false
	for round := 0; round < waitingRefillRounds; round++ {
		rows, err := w.store.WaitingRepliesBefore(ctx, asOf, before)
		if err != nil {
			return nil, false, err
		}
		// Asked of what the STORE returned, before keepWaitingCustomers runs.
		//
		// That filter drops machine senders and folds duplicate threads, so
		// what it returns is smaller than what was read — and a caller
		// comparing the SURVIVORS against the scan bound would read a full
		// scan whose survivors are few as a complete one. This is the only
		// place both numbers exist.
		cut = len(rows) >= activities.WaitingScanCap
		kept = append(kept, keepWaitingCustomers(rows)...)
		if !cut || len(kept) >= activities.WaitingScanCap {
			break
		}
		// The page is ordered newest first, so the oldest row on it is where
		// the next page starts.
		before = rows[len(rows)-1].OccurredAt
	}
	// Folded across pages as well as within one: two mails with the same sender
	// and subject are one conversation whichever page each arrived on, and a
	// per-page fold would let the refill reintroduce what the first page
	// already collapsed.
	return keepWaitingCustomers(kept), cut, nil
}

// keepWaitingCustomers removes repetitive incidental mail. Confirmed requests
// retain their source identities: matching subjects, including within a thread,
// do not prove that two asks describe the same unfinished work.
func keepWaitingCustomers(rows []activities.WaitingReply) []activities.WaitingReply {
	kept := make([]activities.WaitingReply, 0, len(rows))
	seen := make(map[string]bool, len(rows))
	for _, row := range rows {
		if capture.IsMachineAddress(row.Sender) && row.OwedVerdict != activities.OwedVerdictAsksUs {
			continue
		}
		if row.Subject != "" && row.OwedVerdict != activities.OwedVerdictAsksUs {
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

// emailRows reads the canonical email row behind each waiting message that is
// one, in a single statement over the whole lane.
//
// The lane spans email and channel messages (the waiting query's own
// `a.kind IN ('email', 'message')`), so the ids are filtered to email BEFORE
// the read rather than after: a chat has no email row to fetch, and asking for
// one would spend the statement's budget on rows that can only come back
// absent.
//
// The reader carries its own content gate, so a summary reaches the lane only
// for a message this caller may read. That is belt and braces here — a message
// the reader may not read produces no waiting row at all — and it is the lock
// that holds if the lane's own gate is ever loosened.
func (w attentionWaiting) emailRows(
	ctx context.Context, rows []activities.WaitingReply,
) (map[ids.UUID]crmcontracts.EmailSummary, error) {
	var emailIDs []ids.UUID
	for _, row := range rows {
		if row.Kind == string(crmcontracts.ActivityKindEmail) {
			emailIDs = append(emailIDs, row.ActivityID)
		}
	}
	if len(emailIDs) == 0 {
		// An empty map rather than a nil one, for the reason the reader itself
		// gives: the caller reads this by key, and "no emails in this lane" is
		// the same answer to that question as "no rows for you".
		return map[ids.UUID]crmcontracts.EmailSummary{}, nil
	}
	return w.store.EmailSummariesByID(ctx, emailIDs)
}
