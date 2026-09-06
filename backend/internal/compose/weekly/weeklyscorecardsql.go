// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// Freezing the scorecard, and reading it back unchanged.
//
// Split from the queries that compute it: those ask the week what happened,
// these put the answer beyond further change. A scorecard recomputed on read
// would answer differently every time somebody edits an old deal, so the
// numbers are written once and every later reader gets the row.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// insertScorecard freezes one week's judgement beside its review.
//
// ON CONFLICT DO NOTHING on the one-per-review constraint: freezing is
// reachable on its own, and a second pass must leave the first pass's figures
// alone rather than overwrite a record of a week with a later reading of it.
//
// A nil block writes NULLs and its presence boolean false, which is what the
// table's shape CHECK requires and what tells "no funnel this week" from "a
// funnel that scored zero".
func insertScorecard(ctx context.Context, tx pgx.Tx, reviewID ids.UUID, card Scorecard) error {
	cols, args := insertColumns{}, []any(nil)
	add := func(name string, value any) {
		cols = append(cols, name)
		args = append(args, value)
	}
	add("weekly_review_id", reviewID)

	add("has_lead_block", card.Lead != nil)
	if l := card.Lead; l != nil {
		add("lead_advanced", l.Advanced)
		add("lead_disqualified", l.Disqualified)
		add("lead_promoted", l.Promoted)
		add("lead_answered_in_target", l.AnsweredInTarget)
		add("lead_breached", l.Breached)
		add("meetings_booked", l.Booked)
		add("meetings_held", l.Held)
		add("meetings_no_show", l.NoShow)
		add("meetings_partial_history", l.PartialHistory)
	}

	add("has_deal_block", card.Deal != nil)
	if d := card.Deal; d != nil {
		add("deal_advances", d.Advances)
		add("deal_regressions", d.Regressions)
		// Nil travels as NULL: no deal changed stage, and a median of nothing
		// is absent rather than zero days.
		add("deal_median_days_in_stage", d.MedianDaysInStage)
		add("deal_with_next_step", d.WithNextStep)
		add("deal_open", d.Open)
		add("deal_multi_threaded", d.MultiThreaded)
		add("deal_close_date_sound", d.CloseDateSound)
		add("deal_forecast_up", d.ForecastUp)
		add("deal_forecast_down", d.ForecastDown)
	}

	_, err := tx.Exec(ctx, fmt.Sprintf(
		`INSERT INTO weekly_review_scorecard (%s) VALUES (%s)
		 ON CONFLICT (weekly_review_id) DO NOTHING`,
		cols.names(), cols.placeholders()), args...)
	if err != nil {
		return fmt.Errorf("weekly: freezing the week's scorecard: %w", err)
	}
	return nil
}

// readScorecard reads one review's frozen scorecard, or nil when it has none.
//
// A review written before this table existed has no row, and that is not an
// error: the panel draws nothing rather than the caller failing.
func readScorecard(ctx context.Context, tx pgx.Tx, reviewID ids.UUID) (*Scorecard, error) {
	var hasLead, hasDeal bool
	var l LeadBlock
	var d DealBlock
	// Every nullable column scans through a pointer, because a block's absence
	// is exactly what NULL means here.
	var la, ld, lp, lat, lb, mb, mh, mn, mp *int
	var da, dr, dns, do, dmt, dcd, dfu, dfd *int
	err := tx.QueryRow(ctx, `
		SELECT has_lead_block, lead_advanced, lead_disqualified, lead_promoted,
		       lead_answered_in_target, lead_breached, meetings_booked,
		       meetings_held, meetings_no_show, meetings_partial_history,
		       has_deal_block, deal_advances, deal_regressions,
		       deal_median_days_in_stage, deal_with_next_step, deal_open,
		       deal_multi_threaded, deal_close_date_sound,
		       deal_forecast_up, deal_forecast_down
		  FROM weekly_review_scorecard WHERE weekly_review_id = $1`, reviewID).
		Scan(&hasLead, &la, &ld, &lp, &lat, &lb, &mb, &mh, &mn, &mp,
			&hasDeal, &da, &dr, &d.MedianDaysInStage, &dns, &do, &dmt, &dcd, &dfu, &dfd)
	if errors.Is(err, pgx.ErrNoRows) {
		//nolint:nilnil // a review written before scorecards existed HAS none; the panel draws nothing rather than the read failing.
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("weekly: reading the week's scorecard: %w", err)
	}

	card := Scorecard{}
	if hasLead {
		l.Advanced, l.Disqualified, l.Promoted = deref(la), deref(ld), deref(lp)
		l.AnsweredInTarget, l.Breached = deref(lat), deref(lb)
		l.Booked, l.Held, l.NoShow, l.PartialHistory = deref(mb), deref(mh), deref(mn), deref(mp)
		card.Lead = &l
	}
	if hasDeal {
		d.Advances, d.Regressions = deref(da), deref(dr)
		d.WithNextStep, d.Open = deref(dns), deref(do)
		d.MultiThreaded, d.CloseDateSound = deref(dmt), deref(dcd)
		d.ForecastUp, d.ForecastDown = deref(dfu), deref(dfd)
		card.Deal = &d
	}
	return &card, nil
}
