// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Which rows one hiding rule is keeping off this reader's queue.
//
// HiddenWaiting answers how MANY, one rule at a time. This answers which — the
// question a reader has the moment they read a figure they did not expect, and
// the one the guardrail could not answer while it carried counts alone.
//
// Deliberately a second read rather than ids carried on the summary: the
// summary is read on every worklist load and four id arrays would ride every
// one of them, where this is asked only when somebody clicks a figure.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// HiddenRule names one of the queue's hiding rules — the vocabulary a caller
// asks "what is behind this figure" in, and the same five HiddenBacklog counts.
type HiddenRule string

// The five rules, named as the counts name them. A caller asks for rows behind
// the figure it just read, so the words have to be the same words — a sixth
// spelling here would be a rule nothing measures.
const (
	HiddenRuleSetAside    HiddenRule = "set_aside"
	HiddenRuleNotSales    HiddenRule = "not_sales"
	HiddenRulePastHorizon HiddenRule = "past_horizon"
	HiddenRuleUnlinked    HiddenRule = "unlinked"
	HiddenRuleColleagues  HiddenRule = "colleagues"
)

// relaxationFor answers the relaxation one rule name asks for, against this
// reader.
//
// The table is the SAME shape HiddenWaiting counts through, and the two are
// meant to be read side by side: a rule whose relaxation here disagreed with
// the one counted there would list rows the figure never counted.
func relaxationFor(rule HiddenRule, reader ids.UUID) (waitingRelaxation, bool) {
	switch rule {
	case HiddenRuleSetAside:
		// The zero reader matches no reader_state row, which is how the
		// set-aside figure is taken — the rule is relaxed by forgetting whose
		// set-asides they were.
		return waitingRelaxation{reader: ids.UUID{}}, true
	case HiddenRuleNotSales:
		return waitingRelaxation{reader: reader, keepNotSales: true}, true
	case HiddenRulePastHorizon:
		return waitingRelaxation{reader: reader, wholeHorizon: true}, true
	case HiddenRuleUnlinked:
		return waitingRelaxation{reader: reader, keepUnlinked: true}, true
	case HiddenRuleColleagues:
		return waitingRelaxation{reader: reader, keepColleagues: true}, true
	default:
		return waitingRelaxation{}, false
	}
}

// ErrUnknownHiddenRule refuses a rule name this guardrail does not measure.
//
// A refusal rather than an empty list: "no rows behind that rule" and "there is
// no such rule" are different answers, and returning the first for the second
// would let a typo read as a clean queue.
var ErrUnknownHiddenRule = fmt.Errorf("activities: unknown hiding rule")

// HiddenWaitingRows lists the threads one rule is holding back.
//
// The rows are the difference the figure reports: what the relaxed read finds
// and the strict read does not. Computed as a difference for the reason the
// figures are — relaxing a rule admits rows the others still hide, so a plain
// relaxed read would return the whole queue plus the hidden few, and a reader
// clicking "3 set aside" would get three hundred rows.
//
// Every visibility gate rides along untouched, because both halves are the same
// eligibility statement: a thread this reader may not read produces no row in
// either, so it cannot appear in the difference.
func (s *Store) HiddenWaitingRows(
	ctx context.Context, asOf time.Time, rule HiddenRule,
) ([]WaitingReply, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	reader := readerOrNobody(ctx)
	relax, known := relaxationFor(rule, reader)
	if !known {
		return nil, fmt.Errorf("%w: %q", ErrUnknownHiddenRule, rule)
	}
	var out []WaitingReply
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// One horizon for both halves, for the reason HiddenWaiting measures it
		// once: two cutoffs would make the difference report a change in the
		// clock rather than the rule being relaxed.
		measured, err := s.waitingHorizonFor(ctx, tx, asOf)
		if err != nil {
			return err
		}
		// ONE argument list across both halves, so their placeholders number
		// continuously and neither statement has to be rewritten to sit beside
		// the other.
		args := []any{}
		arg := func(v any) int { args = append(args, v); return len(args) }
		relaxed, err := s.waitingStatement(ctx, tx, asOf, relax, measured, arg)
		if err != nil {
			return err
		}
		strict, err := s.waitingStatement(
			ctx, tx, asOf, waitingRelaxation{reader: reader}, measured, arg)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx,
			`SELECT r.* FROM (`+relaxed+`) r
			 WHERE r.id NOT IN (SELECT s.id FROM (`+strict+`) s)`, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row WaitingReply
			if err := rows.Scan(&row.ActivityID, &row.Kind, &row.Subject, &row.Sender, &row.OccurredAt,
				&row.ContactID, &row.CompanyID, &row.DealID,
				&row.HasOpenDeal, &row.OwedVerdict, &row.CaptureLabel, &row.AddressedElsewhere,
				&row.Engaged, &row.OwnerID, &row.Threaded); err != nil {
				return err
			}
			out = append(out, row)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("activities: reading what the %s rule holds back: %w", rule, err)
	}
	return out, nil
}
