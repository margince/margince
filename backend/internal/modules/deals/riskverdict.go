// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Which deals the queue judged material and at risk as a day began, and
// whether somebody booked a next step on them that day.
//
// The judgement itself is made in compose (attention's material bar) and
// cannot be recomputed later: the bar is the median of that day's at-risk
// pipeline. So the day's first pass writes it down, and the figure is a join
// against task creation over the recorded days.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// riskDayEntity is the entity type a recorded day's audit rows are filed
// under; the retention sweep files its erase under the same word.
const riskDayEntity = "deal_risk_day"

// RiskDay is one day's first pass: the local day, its bounds as instants in
// the zone in force, and the one instant the pass judged at.
type RiskDay struct {
	LocalDay   time.Time
	Start, End time.Time
	JudgedAt   time.Time
}

// RecordRiskDay writes the day's first pass and the deals it judged material
// and at risk, and reports whether this pass was the first.
//
// System-only. The figure these rows feed measures what the product judged, so
// a seat able to plant verdicts could move a number that is supposed to report
// on it.
//
// A later pass on a recorded day writes nothing: the figure's denominator is
// the set the day started with, not every deal that turned at risk by evening.
func (s *Store) RecordRiskDay(ctx context.Context, day RiskDay, dealIDs []ids.UUID) (bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return false, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return false, err
	}
	first := false
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		var dayID ids.UUID
		err := tx.QueryRow(ctx, `
			INSERT INTO deal_risk_day (local_day, day_start, day_end, judged_at, captured_by)
			VALUES ($1::date, $2, $3, $4, $5)
			ON CONFLICT ON CONSTRAINT uq_deal_risk_day DO NOTHING
			RETURNING id`,
			day.LocalDay.Format(time.DateOnly), day.Start, day.End, day.JudgedAt, by).Scan(&dayID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("deals: recording the day's risk pass: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO deal_risk_verdict (day_id, deal_id)
			SELECT $1, judged FROM unnest($2::uuid[]) AS judged`, dayID, dealIDs); err != nil {
			return fmt.Errorf("deals: recording risk verdicts: %w", err)
		}
		// No images on the audit row. It records that the pass judged, and
		// when; the deals stay on the verdict rows alone, so the retention
		// delete takes them with it instead of leaving a copy here.
		if _, err := storekit.Audit(ctx, tx, "create", riskDayEntity, dayID, nil, nil); err != nil {
			return fmt.Errorf("deals: auditing the day's risk pass: %w", err)
		}
		first = true
		return nil
	})
	return first, err
}

// NextStepReading is the same-day next-step figure over a run of days.
type NextStepReading struct {
	// Withheld says the caller may not read the money the material bar is
	// taken from, so the figure — and every field below — is not theirs.
	Withheld bool
	// Judged is how many deal-days began material and at risk.
	Judged int
	// Booked is how many of those got a next step booked the same day.
	Booked int
	// RecordedSince is the first day on record, nil when there is none. Days
	// before it are not zero; they were never measured.
	RecordedSince *time.Time
}

// sameDayNextStepSQL counts the verdicts in a run of days and the booked ones.
//
// A next step is a TASK on the deal, created within the day's recorded bounds,
// done or not, and not archived. Meetings are left out on purpose: a meeting
// row is created when the calendar sync first sees it, so its created_at says
// when the product learned of the meeting rather than when anybody booked it.
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
	            AND t.created_at >= rd.day_start
	            AND t.created_at < rd.day_end))
	  FROM deal_risk_day rd
	  JOIN deal_risk_verdict v ON v.day_id = rd.id
	  JOIN deal d ON d.id = v.deal_id
	 WHERE rd.local_day >= $%[1]d::date
	   AND rd.local_day < $%[2]d::date`

// SameDayNextSteps reads the figure over the local days [firstDay, endDay),
// counting only the deals this caller may see.
//
// The days are the caller's to choose and to have ended: a day still running
// can still have its step booked, and counting it would report a miss that is
// only a morning.
//
// The material bar is the workspace's, taken over every amount by the system
// pass. A caller whose field masks withhold deal money reads the figure as
// Withheld: a rate over deals chosen by amounts they may not see would say
// something about those amounts.
func (s *Store) SameDayNextSteps(ctx context.Context, firstDay, endDay time.Time) (NextStepReading, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return NextStepReading{}, err
	}
	masked, err := moneyMasked(ctx)
	if err != nil || masked {
		return NextStepReading{Withheld: masked}, err
	}
	var out NextStepReading
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		// When recording began is a fact about the installation rather than
		// about any deal, so it is read across every day on record.
		if err := tx.QueryRow(ctx,
			`SELECT min(local_day) FROM deal_risk_day`).Scan(&out.RecordedSince); err != nil {
			return fmt.Errorf("deals: reading when risk verdicts began: %w", err)
		}
		if !endDay.After(firstDay) {
			return nil
		}
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		first, end := arg(firstDay.Format(time.DateOnly)), arg(endDay.Format(time.DateOnly))
		scope, err := auth.ScopeClauseFor(ctx, dealTable, "d", arg)
		if err != nil {
			return err
		}
		query := storekit.SQLf(sameDayNextStepSQL, first, end)
		if scope != "" {
			query += " AND " + scope
		}
		if err := tx.QueryRow(ctx, query, args...).Scan(&out.Judged, &out.Booked); err != nil {
			return fmt.Errorf("deals: reading the same-day next-step figure: %w", err)
		}
		return nil
	})
	if err != nil {
		return NextStepReading{}, err
	}
	return out, nil
}

// moneyMasked reports whether any mask this caller carries on a deal withholds
// its amount or currency, the two the material bar is priced from, on any row.
//
// Which masks those are is read off dealMaskableFields by what each withholds,
// so a money mask added there joins this test by existing.
func moneyMasked(ctx context.Context) (bool, error) {
	for field, withhold := range dealMaskableFields {
		minor, currency := int64(1), "EUR"
		probe := crmcontracts.Deal{AmountMinor: &minor, Currency: &currency}
		withhold(&probe)
		if probe.AmountMinor != nil && probe.Currency != nil {
			continue
		}
		masked, err := auth.MasksAnyRowOf(ctx, maskObject, field)
		if err != nil || masked {
			return masked, err
		}
	}
	return false, nil
}
