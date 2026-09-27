// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// What the queue judged about a deal on a day, and whether somebody booked a
// next step on it that same day.
//
// The judgement itself is made in compose (attention's material bar) and
// cannot be recomputed later: the bar is the median of that day's at-risk
// pipeline. So the verdict is written down when it is made, and the figure is a
// join against task creation over the recorded days.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// riskVerdictEntity is the entity type a verdict's audit rows are filed under;
// the retention sweep files its erase under the same word.
const riskVerdictEntity = "deal_risk_verdict"

// RecordRiskVerdicts writes one verdict per deal for one local day, and
// answers how many were new.
//
// System-only. The figure these rows feed measures what the product judged, so
// a seat able to plant verdicts could move a number that is supposed to report
// on it.
//
// Idempotent per deal and day: the pass runs hourly, and a deal judged in the
// morning is not judged again in the afternoon.
func (s *Store) RecordRiskVerdicts(ctx context.Context, day time.Time, dealIDs []ids.UUID) (int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return 0, err
	}
	if len(dealIDs) == 0 {
		return 0, nil
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return 0, err
	}
	written := 0
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			INSERT INTO deal_risk_verdict (deal_id, local_day, captured_by)
			SELECT judged, $2::date, $3 FROM unnest($1::uuid[]) AS judged
			ON CONFLICT ON CONSTRAINT uq_deal_risk_verdict_day DO NOTHING
			RETURNING id`, dealIDs, day.Format(time.DateOnly), by)
		if err != nil {
			return fmt.Errorf("deals: recording risk verdicts: %w", err)
		}
		recorded, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		if err != nil {
			return fmt.Errorf("deals: recording risk verdicts: %w", err)
		}
		// No images on the audit row. It records that the pass judged, and
		// by whom; the deal and the day stay on the verdict row alone, so the
		// retention delete takes them with it instead of leaving a copy here.
		for _, id := range recorded {
			if _, err := storekit.Audit(ctx, tx, "create", riskVerdictEntity, id, nil, nil); err != nil {
				return fmt.Errorf("deals: auditing a risk verdict: %w", err)
			}
		}
		written = len(recorded)
		return nil
	})
	return written, err
}

// NextStepReading is the same-day next-step figure over a run of days.
type NextStepReading struct {
	// Judged is how many deal-days the queue called material and at risk.
	Judged int
	// Booked is how many of those got a next step booked the same day.
	Booked int
	// RecordedSince is the first day any verdict is on record, nil when there
	// is none. Days before it are not zero; they were never measured.
	RecordedSince *time.Time
}

// sameDayNextStepSQL counts the verdicts in a run of days and the booked ones.
//
// A next step is a TASK on the deal, created on the verdict's local day, done
// or not, and not archived. Meetings are left out on purpose: a meeting row is
// created when the calendar sync first sees it, so its created_at says when
// the product learned of the meeting rather than when anybody booked it.
//
// The task side is not narrowed to what the caller may open, for the reason the
// waiting lane leaves its reply side open: a step somebody else may see is
// still a step booked on the deal. What that discloses is one boolean per
// visible deal-day, folded into a count.
const sameDayNextStepSQL = `
	SELECT count(*),
	       count(*) FILTER (WHERE EXISTS (
	         SELECT 1 FROM activity t
	           JOIN activity_link tl ON tl.activity_id = t.id AND tl.deal_id = v.deal_id
	          WHERE t.kind = 'task'
	            AND t.archived_at IS NULL
	            AND (t.created_at AT TIME ZONE $%[3]d)::date = v.local_day))
	  FROM deal_risk_verdict v
	  JOIN deal d ON d.id = v.deal_id
	 WHERE v.local_day >= $%[1]d::date
	   AND v.local_day < $%[2]d::date`

// SameDayNextSteps reads the figure over [firstDay, endDay), counting only the
// deals this caller may see.
//
// The days are the caller's to choose and to have ended: a day still running
// can still have its step booked, and counting it would report a miss that is
// only a morning.
func (s *Store) SameDayNextSteps(
	ctx context.Context, firstDay, endDay time.Time, zone *time.Location,
) (NextStepReading, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return NextStepReading{}, err
	}
	var out NextStepReading
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		// When recording began is a fact about the installation rather than
		// about any deal, so it is read across every verdict: a caller whose
		// own deals were first judged later still learns that the days before
		// were measured, and were zero for them.
		if err := tx.QueryRow(ctx,
			`SELECT min(local_day) FROM deal_risk_verdict`).Scan(&out.RecordedSince); err != nil {
			return fmt.Errorf("deals: reading when risk verdicts began: %w", err)
		}
		if !endDay.After(firstDay) {
			return nil
		}
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		first, end := arg(firstDay.Format(time.DateOnly)), arg(endDay.Format(time.DateOnly))
		if zone == nil {
			zone = time.UTC
		}
		zonePos := arg(zone.String())
		scope, err := auth.ScopeClauseFor(ctx, dealTable, "d", arg)
		if err != nil {
			return err
		}
		query := storekit.SQLf(sameDayNextStepSQL, first, end, zonePos)
		if scope != "" {
			query += " AND " + scope
		}
		if err := tx.QueryRow(ctx, query, args...).
			Scan(&out.Judged, &out.Booked); err != nil {
			return fmt.Errorf("deals: reading the same-day next-step figure: %w", err)
		}
		return nil
	})
	if err != nil {
		return NextStepReading{}, err
	}
	return out, nil
}
