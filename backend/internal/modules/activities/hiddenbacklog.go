// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// What the queue is NOT showing, and which rule is holding it back.
//
// The Worklist is designed to look finite: a rep works it to the bottom and the
// day is done. Five rules make a waiting customer disappear from it, and three
// of them are somebody's choice — `not_sales`, `not_mine`, `snooze`. The other
// two are nobody's: a message older than the horizon, and one with no link to a
// record the workspace sells to.
//
// Nothing watched whether any of that was hiding real work. A rep who marks
// every hard reply `not_sales` produces a page identical to a rep with a clean
// queue, and the screen is BUILT to make that read as success. So the failure is
// invisible by construction, which is the one shape of defect a finite queue
// cannot report on itself.
//
// This is the counter-reading: the same eligibility rules, with one hiding rule
// relaxed at a time, so the difference is attributable rather than a single
// number nobody can act on.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// hiddenHorizonDays is how far back the guardrail looks past the queue's own
// horizon.
//
// A year rather than forever: the question is "is the horizon hiding work a
// human would still act on", and a two-year-old unanswered message is history
// by any reading. An unbounded scan would answer a different question and cost
// a full table read to do it.
const hiddenHorizonDays = 365

// HiddenBacklog is how much waiting work each rule is keeping off one reader's
// queue, at one instant.
//
// Every figure is counted under the CALLER's own visibility, like the team
// board's are: a count summing rows the reader may not open would publish work
// they have no access to. So this reports what is hidden FROM THEM, which is the
// only honest answer available without handing one person a licence to read
// another's records.
type HiddenBacklog struct {
	// Truncated says a read hit WaitingScanCap, which makes every figure below
	// it a floor and `Clear` unsafe to believe.
	//
	// The cap is on the shared statement, so all five reads clip at the same
	// 200. On a queue at the cap the strict read and every relaxed read return
	// 200, all four differences are zero, and a guardrail with no flag would
	// report a clear backlog over the one installation most likely to be hiding
	// work — the exact under-reporting this reading exists to prevent, in the
	// one shape that produces no failing assertion anywhere.
	Truncated bool

	// Shown is what this query FOUND under the rules as they stand. It is here
	// so the others are readable as a proportion rather than as bare volumes:
	// three hidden against four found is a broken queue, and three against three
	// hundred is a rep tidying up.
	//
	// A near neighbour of what the page draws rather than equal to it. Machine
	// senders are filtered twice on purpose: this query removes the obvious ones
	// before its cap, because a scan filled with notification threads would push
	// a real customer past it, and the seam then applies capture's fuller
	// address rule over the survivors. So a mail relayed by a transactional
	// domain is counted here and dropped there, as is a repeat thread from one
	// sender. Measuring either here would put a second copy of that baseline in
	// the database.
	//
	// The four figures below are differences between runs of this same query, so
	// they are counted the same way and the proportions hold.
	Shown int
	// SetAside is work this reader has snoozed or marked not_mine. Their own
	// choice, and the least alarming of the three — a snooze lifts on its own
	// moment, so this figure includes work that will come back without anybody
	// remembering it.
	SetAside int
	// NotSales is work somebody judged to be no business of the queue's. It
	// hides the thread from the WHOLE workspace and does not lift, so it is the
	// judgement worth watching: one rep's mistake removes a customer from
	// everybody's day permanently.
	NotSales int
	// PastHorizon is work older than the queue's horizon with no open deal
	// behind it. NOBODY CHOSE THIS. A customer who wrote four months ago and was
	// never answered is exactly the failure a sales queue exists to prevent, and
	// the horizon removes them silently.
	PastHorizon int
	// Unlinked is inbound mail that qualifies in every other way and is attached
	// to no record the workspace sells to. Also nobody's choice: it is usually
	// genuine — a rep's dentist is not a customer — and it is also where a real
	// customer lands when capture failed to link their thread. That ambiguity is
	// why it is its own figure rather than folded into a total.
	Unlinked int
	// Colleagues is mail from our own email domains. Nobody's choice either,
	// and the figure matters because the rule is only as good as the domain
	// list behind it: a domain entered by mistake suppresses a real customer's
	// correspondence workspace-wide, and this is the number that would show it.
	Colleagues int
}

// Clear reports whether nothing is being held back.
//
// The guardrail's target is zero, and the target is the point: a number that
// only ever gets read next to other numbers becomes decoration. This is what a
// check asserts on.
// A truncated read is never clear. It is not a claim that work IS hidden — it
// is a refusal to claim the opposite, which is the only honest answer available
// when the scan stopped before the question was settled.
func (h HiddenBacklog) Clear() bool {
	return !h.Truncated &&
		h.SetAside == 0 && h.NotSales == 0 && h.PastHorizon == 0 && h.Unlinked == 0 &&
		h.Colleagues == 0
}

// HiddenWaiting counts what each hiding rule is keeping off this reader's queue.
//
// Five reads of ONE query rather than five queries. `waitingRepliesSQL` carries
// every eligibility rule the Worklist trusts — the anti-joins, the machine-sender
// exclusion, the live-record predicates, the visibility gates — and a second
// statement restating them would be a second answer to "is this person waiting",
// wrong the first time either is edited. What varies between the reads is only
// which hiding rule is switched off, through holes the constant already has.
//
// The count is of THREADS, matching what the queue counts, because the query
// groups by thread before it is counted here.
func (s *Store) HiddenWaiting(ctx context.Context, asOf time.Time) (HiddenBacklog, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return HiddenBacklog{}, err
	}
	var out HiddenBacklog
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		reader := readerOrNobody(ctx)
		// The queue's own horizon, measured once and handed to every relaxation
		// below. Measured per count instead, the guardrail could report a
		// difference between two scans that came from two different cutoffs
		// rather than from the rule it was relaxing.
		measured, err := s.waitingHorizonFor(ctx, tx, asOf)
		if err != nil {
			return err
		}
		// What the eligibility query finds under the rules as they stand — a
		// near neighbour of the page's own count, for the reason Shown states.
		shown, err := s.countWaiting(ctx, tx, asOf, waitingRelaxation{reader: reader}, measured)
		if err != nil {
			return err
		}
		out.Shown = shown
		// Asked of the STRICT read as well as the relaxed ones below: a queue
		// already at the cap cannot have its hidden work measured at all, since
		// every relaxation clips at the same bound.
		out.Truncated = shown >= WaitingScanCap
		// Each rule relaxed ALONE, so a difference names one cause. Relaxing
		// them together would answer "how much is hidden" and leave a reader
		// unable to act on it — the whole point of this reading is which rule to
		// look at.
		//
		// `ids.UUID{}` is the no-reader spelling readerOrNobody already uses for
		// a background job: it matches no activity_reader_state row, so nothing
		// is set aside for it.
		for _, relaxed := range []struct {
			into *int
			with waitingRelaxation
		}{
			{&out.SetAside, waitingRelaxation{reader: ids.UUID{}}},
			{&out.NotSales, waitingRelaxation{reader: reader, keepNotSales: true}},
			{&out.PastHorizon, waitingRelaxation{reader: reader, wholeHorizon: true}},
			{&out.Unlinked, waitingRelaxation{reader: reader, keepUnlinked: true}},
			{&out.Colleagues, waitingRelaxation{reader: reader, keepColleagues: true}},
		} {
			widened, err := s.countWaiting(ctx, tx, asOf, relaxed.with, measured)
			if err != nil {
				return err
			}
			// A relaxed read at the cap has been cut short too, so the
			// difference it yields is a floor rather than a count.
			if widened >= WaitingScanCap {
				out.Truncated = true
			}
			// The DIFFERENCE, floored at zero. A relaxed read can only be a
			// superset of the strict one, so a negative is impossible — and
			// flooring says so rather than publishing a nonsense figure if some
			// future edit breaks that property. The invariant is held by a test
			// rather than by this clamp.
			if widened > shown {
				*relaxed.into = widened - shown
			}
		}
		return nil
	})
	if err != nil {
		return HiddenBacklog{}, fmt.Errorf("activities: counting the hidden backlog: %w", err)
	}
	return out, nil
}

// waitingRelaxation names which hiding rule this read switches off.
//
// The zero value is the queue's own behaviour with a reader who has set nothing
// aside, so a field added here that nobody sets changes nothing — which is what
// keeps a new relaxation from silently widening the strict read every figure is
// measured against.
type waitingRelaxation struct {
	// reader is whose set-asides apply. The zero uuid matches no reader_state
	// row, which is how the SetAside figure is taken.
	reader ids.UUID
	// keepNotSales admits threads somebody judged to be no sales business.
	keepNotSales bool
	// wholeHorizon looks back hiddenHorizonDays instead of the queue's own
	// horizon.
	wholeHorizon bool
	// keepUnlinked admits inbound mail attached to no sales record.
	keepUnlinked bool
	// keepColleagues admits mail from our own email domains.
	keepColleagues bool
}

// countWaiting runs the waiting query under one relaxation and counts its rows.
func (s *Store) countWaiting(
	ctx context.Context, tx pgx.Tx, asOf time.Time, relax waitingRelaxation, measured int,
) (int, error) {
	args := []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }
	instant := arg(asOf)
	content, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return 0, err
	}
	linkVisible, err := auth.LinkTargetVisibleClause(ctx, "wl", arg)
	if err != nil {
		return 0, err
	}
	if linkVisible == "" {
		linkVisible = scopeUnbounded
	}
	horizon := horizonOrDefault(measured)
	if relax.wholeHorizon {
		horizon = hiddenHorizonDays
	}
	// The two judgement relaxations are predicates OR-ed in front of their
	// clause rather than clauses removed, so the statement's shape — and every
	// rule around it — is identical across all five reads.
	// `scopeUnbounded` is this package's word for an always-true predicate, and
	// it is the right one here: a relaxation admits every row the clause would
	// have removed, which is the same "no bound applies" this constant already
	// spells everywhere a scope is absent.
	notSales, unlinked, colleague := neverRelaxed, neverRelaxed, neverRelaxed
	if relax.keepNotSales {
		notSales = scopeUnbounded
	}
	if relax.keepUnlinked {
		unlinked = scopeUnbounded
	}
	if relax.keepColleagues {
		colleague = scopeUnbounded
	}
	ownDomains, err := s.ownDomainList(ctx, tx)
	if err != nil {
		return 0, err
	}
	inner := fmt.Sprintf(waitingRepliesSQL, instant, content, linkVisible, WaitingScanCap,
		horizon,
		liveRecord(openDealPredicate, "d"),
		liveRecord(workingLeadPredicate, "ld"),
		liveRecord(openDealPredicate, "openDeal"),
		liveRecord(openDealPredicate, "fd"),
		arg(relax.reader),
		scopeUnbounded,
		notSales, unlinked,
		colleague, ownDomainSenderSQL("a", arg(ownDomains)))
	var count int
	// Counted around the whole statement rather than by replacing its SELECT
	// list: the query GROUPs and LIMITs, so the row count IS the answer and a
	// count(*) pushed inside would report groups before the cap.
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM (`+inner+`) waiting`, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}
