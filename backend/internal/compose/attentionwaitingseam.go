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
	"slices"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// attentionWaiting binds the who-is-waiting lane to the activities module's
// own gated read. The thread walk, the discover gate and the audience arm all
// live there; nothing about who may see what is decided here.
type attentionWaiting struct {
	store    *activities.Store
	contacts *contacts.Store
	// deals answers the same-day next-step figure /worklist/response carries
	// beside the waiting work's own two.
	deals *deals.Store
	now   attention.Clock
}

// The instant comes from the caller so the whole read is one snapshot. Asking
// the clock again here would let the anti-joins judge against a moment the rest
// of the day was not read at.
// Answered asks the modules how fast the workspace replied over a window, and
// how often an at-risk deal got its next step the same day. Pass-throughs, like
// Hidden: every figure is SQL and belongs beside its query.
func (w attentionWaiting) Answered(
	ctx context.Context, from, to time.Time, days attention.LocalDays,
) (attention.AnsweredWork, error) {
	got, err := w.store.ResponseWindow(ctx, from, to)
	if err != nil {
		return attention.AnsweredWork{}, err
	}
	steps, err := w.deals.SameDayNextSteps(ctx, days.First, days.End)
	if err != nil {
		return attention.AnsweredWork{}, err
	}
	return attention.AnsweredWork{
		Answered:         got.Answered,
		MedianMinutes:    got.MedianMinutes,
		Disposed:         got.Disposed,
		DisposedNotSales: got.DisposedNotSales,
		AtRiskWithheld:   steps.Withheld,
		AtRiskJudged:     steps.Judged,
		AtRiskBooked:     steps.Booked,
		RecordedSince:    steps.RecordedSince,
	}, nil
}

// Hidden asks the module what its own hiding rules are keeping off the queue.
//
// A pass-through: the arithmetic is one read of the eligibility query per rule and
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
		InformsUs:   got.InformsUs,
		Truncated:   got.Truncated,
	}, nil
}

func (w attentionWaiting) Unanswered(
	ctx context.Context, asOf time.Time, narrow attention.WaitingFor,
) ([]attention.WaitingCustomer, bool, error) {
	kept, cut, err := w.waitingPages(ctx, asOf, narrow)
	if err != nil {
		return nil, false, err
	}
	summaries, err := w.emailRows(ctx, kept)
	if err != nil {
		return nil, false, err
	}
	rows, err := w.asWaitingCustomers(ctx, kept, summaries)
	return rows, cut, err
}

// asWaitingCustomers carries the module's rows across in the queue's
// vocabulary.
//
// Shared by Unanswered and HiddenRows rather than written twice: both answer
// with the same card, and the translation below — which verdict word changes a
// ranking, when a summary is withheld — is the part that would go quietly
// wrong in a second copy.
func (w attentionWaiting) asWaitingCustomers(
	ctx context.Context, kept []activities.WaitingReply, summaries map[ids.UUID]crmcontracts.EmailSummary,
) ([]attention.WaitingCustomer, error) {
	suppliers, err := w.waitingSuppliers(ctx, kept)
	if err != nil {
		return nil, err
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
		supplier := suppliers[row.CompanyID]
		if row.CompanyID == (ids.UUID{}) {
			supplier = suppliers[row.ContactID]
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
			NonBuyer:          row.CaptureLabel == "noise" || (!row.HasOpenDeal && supplier),
			DirectRecipients:  row.DirectRecipients,
			OwnerID:           row.OwnerID,
			OwnerKnown:        row.OwnerKnown,
			EarlierRequests:   row.EarlierRequests,
			FirstAskedAt:      row.FirstAskedAt,
			MeetingBookedAt:   row.MeetingBookedAt,
			SenderWasStranger: row.SenderWasStranger,
			BccOnly:           row.BccOnly,
			ForeignLink:       row.ForeignLink,
		})
	}
	return out, nil
}

// HiddenRows names the threads one hiding rule is keeping off this reader's
// page.
//
// A pass-through like Hidden: the difference between the relaxed and strict
// reads is the module's arithmetic, and belongs beside the query it differences.
func (w attentionWaiting) HiddenRows(
	ctx context.Context, asOf time.Time, rule string,
) ([]attention.WaitingCustomer, error) {
	kept, err := w.store.HiddenWaitingRows(ctx, asOf, activities.HiddenRule(rule))
	if err != nil {
		return nil, err
	}
	summaries, err := w.emailRows(ctx, kept)
	if err != nil {
		return nil, err
	}
	return w.asWaitingCustomers(ctx, kept, summaries)
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
// The filter runs after the scan cap: the store's machine rule is six address
// patterns, and keepWaitingCustomers asks the whole rule (sentBySendingSystem). Two
// hundred relay mails would fill the cap, so the refill asks for another page.
// `cut` is true only when the last page was also full.
func (w attentionWaiting) waitingPages(ctx context.Context, asOf time.Time, narrow attention.WaitingFor) ([]activities.WaitingReply, bool, error) {
	var kept []activities.WaitingReply
	var before activities.WaitingPosition
	cut := false
	for range waitingRefillRounds {
		rows, err := w.store.WaitingRepliesNarrowedBefore(ctx, asOf, before, waitingNarrow(narrow))
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
		last := rows[len(rows)-1]
		before = activities.WaitingPosition{At: last.OccurredAt, ID: last.ActivityID}
	}
	// Folded across pages as well as within one: two mails with the same sender
	// and subject are one conversation whichever page each arrived on, and a
	// per-page fold would let the refill reintroduce what the first page
	// already collapsed.
	return keepWaitingCustomers(kept), cut, nil
}

// sentBySendingSystem is a message a sending system wrote that asks us nothing.
// The stamp covers mail captured since capture wrote it. The rule itself
// covers the rest, chat included, while the data task stamps older mail.
func sentBySendingSystem(row activities.WaitingReply) bool {
	machine := row.SenderIsMachine || capture.IsMachineAddress(row.Sender)
	return machine && row.OwedVerdict != activities.OwedVerdictAsksUs
}

// keepWaitingCustomers removes repetitive incidental mail, and shows one card
// per conversation: a conversation's unanswered requests fold into its newest,
// which carries how many earlier ones there are and when the first arrived.
// The store's card tail folds alike (TestTheWaitingTailIsTheFoldOfTheScan).
// Folding is presentation only. Each request keeps its own identity and
// settles on its own; a set-aside on the card reaches the earlier ones too
// (activities.setReaderState).
func keepWaitingCustomers(rows []activities.WaitingReply) []activities.WaitingReply {
	kept := make([]activities.WaitingReply, 0, len(rows))
	seen := make(map[string]bool, len(rows))
	cardOf := make(map[string]int, len(rows))
	for _, row := range rows {
		if sentBySendingSystem(row) {
			continue
		}
		if row.OwedVerdict == activities.OwedVerdictAsksUs && row.ThreadKey != "" {
			conversation := row.Kind + "\x00" + row.ChannelProvider + "\x00" + row.ThreadKey
			if at, folded := cardOf[conversation]; folded {
				foldIntoCard(&kept[at], row)
				continue
			}
			cardOf[conversation] = len(kept)
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

// foldIntoCard adds an earlier request to its conversation's card. The card is
// the newest request, whichever order the rows arrive in.
//
// Either side may already be a card: the lane folds each scan page and then the
// pages together, so the counts and first dates of both are merged.
func foldIntoCard(card *activities.WaitingReply, other activities.WaitingReply) {
	count := card.EarlierRequests + other.EarlierRequests + 1
	first := earliestAsk(*card, other)
	if other.OccurredAt.After(card.OccurredAt) {
		*card = other
	}
	card.EarlierRequests, card.FirstAskedAt = count, first
}

// earliestAsk is when the first request among two cards arrived.
func earliestAsk(a, b activities.WaitingReply) time.Time {
	first := a.OccurredAt
	for _, at := range []time.Time{a.FirstAskedAt, b.FirstAskedAt, b.OccurredAt} {
		if !at.IsZero() && at.Before(first) {
			first = at
		}
	}
	return first
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
	return emailSummariesReusingPlans(ctx, w.store, emailIDs)
}

// emailSummariesReusingPlans reads the email rows behind a lane's rows. Its
// statements key on the ids and spell kinds as literals, so the generic plan
// serves. Planned at every call, they cost more to plan than to run.
func emailSummariesReusingPlans(
	ctx context.Context, store *activities.Store, activityIDs []ids.UUID,
) (map[ids.UUID]crmcontracts.EmailSummary, error) {
	var summaries map[ids.UUID]crmcontracts.EmailSummary
	err := database.ReuseGenericPlans(ctx, func(ctx context.Context) error {
		var err error
		summaries, err = store.EmailSummariesByID(ctx, activityIDs)
		return err
	})
	return summaries, err
}

// waitingNarrow carries the lane's narrowing across in the store's vocabulary.
func waitingNarrow(narrow attention.WaitingFor) activities.WaitingNarrow {
	return activities.WaitingNarrow{Owner: narrow.Owner, Team: narrow.Team, Seats: narrow.Seats, Unowned: narrow.Unowned}
}

// WaitingAfter judges the next messages past a position, oldest first, and
// draws the cards among them as the lane draws them.
func (w attentionWaiting) WaitingAfter(
	ctx context.Context, asOf time.Time, narrow attention.WaitingFor, after string, judge int,
) (attention.WaitingTail, error) {
	var from activities.WaitingPosition
	if after != "" {
		position, err := storekit.DecodeOpaque[activities.WaitingPosition](after)
		if err != nil {
			return attention.WaitingTail{}, err
		}
		from = position
	}
	read, err := w.store.WaitingCardsAfter(ctx, asOf, waitingNarrow(narrow), from, judge)
	if err != nil {
		return attention.WaitingTail{}, err
	}
	// The store drops stamped mail before its fold. Mail from before the stamp
	// is dropped here; one sender's machine mail folds only with itself.
	read.Cards = slices.DeleteFunc(read.Cards, sentBySendingSystem)
	summaries, err := w.emailRows(ctx, read.Cards)
	if err != nil {
		return attention.WaitingTail{}, err
	}
	rows, err := w.asWaitingCustomers(ctx, read.Cards, summaries)
	if err != nil {
		return attention.WaitingTail{}, err
	}
	tail := attention.WaitingTail{More: read.More, Cards: make([]attention.Positioned[attention.WaitingCustomer], len(rows))}
	for i, row := range rows {
		position, err := storekit.EncodeOpaque(activities.WaitingPosition{At: read.Cards[i].OccurredAt, ID: read.Cards[i].ActivityID})
		if err != nil {
			return attention.WaitingTail{}, err
		}
		tail.Cards[i] = attention.Positioned[attention.WaitingCustomer]{Row: row, After: position}
	}
	if !read.Through.At.IsZero() {
		if tail.Through, err = storekit.EncodeOpaque(read.Through); err != nil {
			return attention.WaitingTail{}, err
		}
		tail.ThroughAt = read.Through.At
	}
	return tail, nil
}

// DealsWaiting names which of these deals a waiting card names, judged as the
// tail judges its cards, machine senders included.
func (w attentionWaiting) DealsWaiting(
	ctx context.Context, asOf time.Time, narrow attention.WaitingFor, deals []ids.UUID,
) (map[ids.UUID]bool, error) {
	cards, err := w.store.WaitingCardsOnDeals(ctx, asOf, waitingNarrow(narrow), deals)
	if err != nil {
		return nil, err
	}
	named := map[ids.UUID]bool{}
	for _, card := range slices.DeleteFunc(cards, sentBySendingSystem) {
		if !card.DealID.IsZero() {
			named[card.DealID] = true
		}
	}
	return named, nil
}

// CardsAmong names which of these messages are waiting cards the tail serves
// under the narrowing, machine senders left out as the tail leaves them.
func (w attentionWaiting) CardsAmong(
	ctx context.Context, asOf time.Time, narrow attention.WaitingFor, messages []ids.UUID,
) (map[ids.UUID]bool, error) {
	cards, err := w.store.WaitingCardsAmong(ctx, asOf, waitingNarrow(narrow), messages)
	if err != nil {
		return nil, err
	}
	served := map[ids.UUID]bool{}
	for _, card := range slices.DeleteFunc(cards, sentBySendingSystem) {
		served[card.ActivityID] = true
	}
	return served, nil
}
