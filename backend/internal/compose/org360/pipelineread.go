// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package org360

// The account's OPEN pipeline: the deals still in play, what they are worth in
// the installation's own currency, and which of them have gone quiet.
//
// Split from suggestionreads.go at the 500-line cap. The seam is a concept
// rather than a line count: everything here answers "what is open on this
// account and what is it worth", which is one read the state strip and the
// suggestion rules share, while what remains there is the other evidence the
// rules weigh — the newest message, the scheduled task, the lifecycle.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/idlebase"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// stalledDeal is one stalled open deal as the suggestion rules cite it.
type stalledDeal struct {
	ID   ids.UUID
	Name string
	// IdleSince is the instant the stall is measured from — the deal's last
	// activity, or its creation if it has none.
	IdleSince time.Time
	// StageMoves is how many times this deal has actually CHANGED stage, counting
	// the row its creation writes.
	//
	// Advancing a deal is the most deliberate kind of work there is on one, and it
	// moves no timestamp the stall rule reads — so without this the advice a rep
	// dismissed would stay silenced through every stage the deal went on to reach.
	//
	// Re-selecting the stage a deal is already in still writes a history row
	// (nothing rejects it), and that is not work. Counting it would hand the rep
	// back advice they dismissed because someone opened the stage picker and
	// changed nothing.
	StageMoves int
}

// episode identifies the STALL, not the deal: the deal's own activity and its
// stage moves.
//
// It must MOVE when the deal is worked and stalls again, or one dismissal silences
// that deal for good. It must move only FORWARD, or a shape the rep already
// dismissed can recur and that old dismissal comes back to life — silencing advice
// they may have been shown again in between. Both components satisfy the second
// property, for different reasons each. activities.LogActivity advances
// last_activity_at with greatest() and nothing lowers it. The move count rises
// because deal_stage_history is only ever appended to — nothing in the tree deletes
// a row, and erasure and retention archive the deal instead — and because the one
// thing that changes a row's from_stage_id is the FK's ON DELETE SET NULL, which
// can only turn an excluded row (from = to) into a counted one, never the reverse.
//
// Neither alone is enough — logging a call moves the timestamp and not the count,
// advancing a stage moves the count and not the timestamp — and the stall rule
// reads only the first, so the count is the half a fingerprint built from
// IsStalled's own inputs would miss.
//
// They are not everything a person can do to a deal. Editing it — re-pricing,
// pushing the close date, changing the owner — moves neither, so a dismissal
// survives that. deal.version would catch all of it and is monotone, but it also
// bumps on writes no person made: CloseDateCorrector patches expected_close_date
// from a sweep, and keying on it would hand a rep back advice they dismissed
// because a nightly job touched the row. Which edits count as working a deal is a
// product question rather than one to infer from the schema, and it has not been
// answered: this fingerprint should derive from that answer, not stand in for it.
//
// wait_until is deliberately NOT here, though the stall rule reads it: a deferral
// can be set, expire, and be cleared, returning the deal to a shape the rep already
// dismissed. While it runs the deal is not stalled at all, so no advice is due for
// a dismissal to affect, and when it ends the deal is in exactly the state they
// declined with nothing worked in between.
//
// The rule that leaves is one sentence, and it is the one a rep means: "not now"
// silences this deal until it is next worked.
func (d stalledDeal) episode() string {
	return fmt.Sprintf("%s@%s#%d", d.ID, d.IdleSince.UTC().Format(time.RFC3339Nano), d.StageMoves)
}

// pipeline is the account's open pipeline as the deal-shaped rules read it.
//
// Every field is derived from ONE read of the whole visible open set, so the
// count, the digest and the stalled list cannot disagree with each other. Two
// statements would take two Read Committed snapshots, and a deal closing between
// them would leave the card reporting a pipeline that never existed at any
// instant — the 360's as_of promises the opposite.
type pipeline struct {
	// OpenCount is how many open deals the caller can see.
	OpenCount int
	// OpenDigest identifies WHICH ones, so a dismissal keyed on it re-arms the
	// moment the set changes — including a change to a deal no card listed.
	OpenDigest string
	// Open is the deals themselves, id and name, in the read's own order. The
	// advice cites them: "there is no next step here" is a claim a reader can
	// only check against the deals it was read from, and a count alone leaves
	// them to go and find out which ones those are.
	Open []openDeal
	// Stalled is every stalled one, longest idle first. The display cap is
	// applied by the rule that lists them, AFTER dismissals are filtered out, so
	// dismissing one suggestion reveals the next rather than shrinking the card.
	Stalled []stalledDeal
	// ValueMinorBase is the open pipeline in the workspace's base currency, and
	// Priced counts how many of the open deals carry a figure at all.
	//
	// Both travel together because the sum alone cannot be read honestly: a
	// deal with no amount, and one whose currency has no conversion rate, both
	// contribute nothing, and a total that silently omits them reads as the
	// whole pipeline. Priced < OpenCount is what lets the page say the figure
	// covers part of it rather than showing a number that is quietly short.
	ValueMinorBase int64
	Priced         int
	// NextCloseOn is the nearest expected close date among the open deals, or
	// nil when none of them names one.
	NextCloseOn *time.Time
	// Converted counts the deals that needed a rate to enter the sum, and
	// FXAsOf is the OLDEST rate date among them. Each deal converts at the
	// latest rate on or before the read's as-of day, and installations do not
	// hold every currency's rate for every day, so the dates behind one total
	// can differ — this is the furthest back any part of the figure reaches.
	// Without both, the total is a cross-currency sum with no conversion source
	// behind it, which is what plan §4.2 forbids showing.
	Converted int
	FXAsOf    *time.Time
	// BaseCurrency is what ValueMinorBase is denominated in, read in the SAME
	// statement as the figure so the two cannot come from different snapshots
	// — a converted sum labelled with a currency fetched separately is the
	// unlabelled cross-currency total the page must never show.
	BaseCurrency string
}

// openPipeline reads every open deal on the account the caller may see, in one
// statement, and folds the figures the rules need out of it.
//
// It is deliberately unbounded. A bound would put the card's own read inside
// every number it reports: a count capped at the fetch is one a rep cannot tell
// from a real one, a digest over a fetched page leaves a dismissal in force when
// a deal outside it changes, and a stalled list cut before dismissals are
// applied shrinks by one each time the rep judges a row. It reads one narrow row
// per open deal of one account — columns through the organization_id index, a
// count served by idx_dsh_deal, and one rate lookup served by
// idx_fx_rate_lookup.
//
// The stall flag is folded with deals.IsStalled — the same call that stamps the
// wire flag — rather than filtered in SQL. The deals module's SQL spelling of the
// rule is unexported, and it evaluates against the database's now(); this read
// pins its own instant, so a clause on the database clock would put a suggestion
// on a different moment than the as_of it is reported under.
func openPipeline(
	ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID, now time.Time,
	baseCcy string,
) (pipeline, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	orgPos := arg(orgID)
	dealScope, err := scopeClause(ctx, "deal", "d", arg)
	if err != nil {
		return pipeline{}, err
	}
	// Longest idle first, which is the order the stalled rows are offered in, and
	// by id so that order is deterministic between two deals idle since the same
	// instant.

	// An OPEN deal has no frozen rate. The rate freezes on close
	// (deals.deal_advance), and this query reads only open deals
	// (openDealsWhere), so `amount_minor_base` is null on every row it returns
	// and `fx_rate_date` names nothing that has been applied to anything: the
	// schema constrains the two together only for a CLOSED deal
	// (deal_closed_fx), so a stored date here can outlive the rate beside it.
	// Neither stored column is read.
	//
	// The conversion is NOT in this statement. It used to be — a lateral on
	// fx_rate and a round()::bigint per row — and that was a second
	// implementation of a decision the hierarchy rollup already made in Go:
	// the direction, the as-of cutoff, newest-wins, and the multiply-and-round.
	// The two agreed, and nothing made them keep agreeing; the first divergence
	// anyone predicted was rounding, half-away-from-zero against Postgres
	// round(), which is a one-minor-unit disagreement between two pages about
	// the same account and nobody able to reproduce it on demand.
	//
	// So this reads the deal and deals.FXRates does the rest, ONCE PER CURRENCY
	// rather than once per row. What stays here is this surface's own policy: a
	// deal with no usable rate is priced at nothing and still counted, which is
	// what priced_count reports.
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT d.id, d.name, d.status, d.created_at, d.last_activity_at, d.wait_until,
		       (SELECT count(*) FROM deal_stage_history h
		         WHERE h.deal_id = d.id AND h.from_stage_id IS DISTINCT FROM h.to_stage_id),
		       d.amount_minor,
		       -- Cast the DATE column to a timestamp: pgx decodes a bare date
		       -- (OID 1082) into its own Date type, not into time.Time, and the
		       -- scan below fails at runtime rather than at compile time. Only
		       -- a read against real rows finds that, which is why it is spelled
		       -- here rather than left to the driver.
		       d.expected_close_date::timestamptz,
		       d.currency
		FROM deal d
		%s
		ORDER BY %s, d.id`,
		openDealsWhere(orgPos, dealScope), idlebase.SQL("d")), args...)
	if err != nil {
		return pipeline{}, fmt.Errorf("read the account's open pipeline: %w", err)
	}
	open, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (openRow, error) {
		var r openRow
		var status string
		var createdAt time.Time
		var lastActivityAt, waitUntil *time.Time
		if err := row.Scan(&r.id, &r.name, &status, &createdAt, &lastActivityAt, &waitUntil,
			&r.stageMoves, &r.amountMinor, &r.closeOn, &r.currency); err != nil {
			return r, err
		}
		r.baseCcy = baseCcy
		r.stalled = deals.IsStalled(status, createdAt, lastActivityAt, waitUntil, now)
		// The same base IsStalled measures from, so the fingerprint moves exactly
		// when the stall the rep judged is replaced by a new one.
		r.idleSince = idlebase.Since(createdAt, lastActivityAt)
		return r, nil
	})
	if err != nil {
		return pipeline{}, err
	}
	if err := priceOpenDeals(ctx, tx, open, baseCcy, now); err != nil {
		return pipeline{}, err
	}

	return foldPipeline(open), nil
}

// priceOpenDeals fills each row's converted amount and the day its rate is
// dated, through the one engine both this read and the hierarchy rollup use.
//
// A deal with no amount, or in a currency the estate holds no rate for, is left
// UNPRICED rather than refused: this surface reports a partial figure with the
// count of deals that reached it, where the rollup refuses the whole read. That
// difference is the policy, and it is the only thing the two are allowed to
// disagree about.
//
// The rate and its DATE travel together, because they are one answer: a date
// coalesced from somewhere else could name a day whose rate is not the rate the
// figure was computed at, which is the unlabelled cross-currency total in a more
// convincing disguise (plan §4.2).
func priceOpenDeals(
	ctx context.Context, tx pgx.Tx, open []openRow, baseCcy string, now time.Time,
) error {
	rates := deals.NewFXRates(baseCcy, now.UTC())
	for i := range open {
		row := &open[i]
		if row.amountMinor == nil || row.currency == nil {
			continue
		}
		rate, found, err := rates.For(ctx, tx, *row.currency)
		if err != nil {
			return fmt.Errorf("price the account's open pipeline: %w", err)
		}
		if !found {
			continue
		}
		converted, err := deals.ConvertToBase(*row.amountMinor, rate.Rate)
		if err != nil {
			// The deal is unpriceable rather than the read unreadable: an
			// amount whose converted value does not fit is one deal the figure
			// cannot cover, and priced_count is what says so.
			continue
		}
		row.valueBase = &converted
		on := rate.On
		row.rateDate = &on
	}
	return nil
}
