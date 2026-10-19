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
// summary is read on every worklist load and an id array per rule would ride every
// one of them, where this is asked only when somebody clicks a figure.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// HiddenRule names one of the queue's hiding rules — the vocabulary a caller
// asks "what is behind this figure" in, and the same rules HiddenBacklog counts.
type HiddenRule string

// The rules, named as the counts name them. A caller asks for rows behind
// the figure it just read, so the words have to be the same words — a sixth
// spelling here would be a rule nothing measures.
const (
	HiddenRuleSetAside    HiddenRule = "set_aside"
	HiddenRuleNotSales    HiddenRule = "not_sales"
	HiddenRulePastHorizon HiddenRule = "past_horizon"
	HiddenRuleUnlinked    HiddenRule = "unlinked"
	HiddenRuleColleagues  HiddenRule = "colleagues"
	HiddenRuleInformsUs   HiddenRule = "informs_us"
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
		return waitingRelaxation{reader: reader, keepSetAside: true}, true
	case HiddenRuleNotSales:
		return waitingRelaxation{reader: reader, keepNotSales: true}, true
	case HiddenRulePastHorizon:
		return waitingRelaxation{reader: reader, wholeHorizon: true}, true
	case HiddenRuleUnlinked:
		return waitingRelaxation{reader: reader, keepUnlinked: true}, true
	case HiddenRuleColleagues:
		return waitingRelaxation{reader: reader, keepColleagues: true}, true
	case HiddenRuleInformsUs:
		return waitingRelaxation{reader: reader, keepInformsUs: true}, true
	default:
		return waitingRelaxation{}, false
	}
}

// ErrUnknownHiddenRule refuses a rule name this guardrail does not measure.
//
// A refusal rather than an empty list: "no rows behind that rule" and "there is
// no such rule" are different answers, and returning the first for the second
// would let a typo read as a clean queue.
var ErrUnknownHiddenRule = errors.New("activities: unknown hiding rule")

// UnknownHiddenRuleError is that refusal with the field it is about, so the
// transport answers 422 rather than a server fault.
//
// The endpoint's handler used to say the generated router validated the path
// enum before the request arrived. It does not.
//
// So this refusal was the only thing between a mistyped rule and the store.
// It reached the caller as `500 internal`, because a bare sentinel names no
// field.
//
// It wraps the sentinel, so a caller testing errors.Is still sees it.
type UnknownHiddenRuleError struct{ Rule string }

func (e *UnknownHiddenRuleError) Error() string {
	return "unknown hiding rule " + e.Rule
}

func (e *UnknownHiddenRuleError) Unwrap() error { return ErrUnknownHiddenRule }

// FieldFault names the path parameter the caller can correct.
func (e *UnknownHiddenRuleError) FieldFault() (field, code, message string) {
	return "rule", "invalid_enum", e.Error()
}

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
		return nil, &UnknownHiddenRuleError{Rule: string(rule)}
	}
	var out []WaitingReply
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// One horizon for both halves, as in HiddenWaiting: two cutoffs would
		// make the difference report a changed clock, not the relaxed rule.
		measured, err := s.waitingHorizonFor(ctx, tx)
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
			ctx, tx, asOf, waitingRelaxation{reader: reader}, measured, arg,
		)
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
			row, err := scanWaitingReply(rows)
			if err != nil {
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
