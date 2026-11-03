// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// snapshotFrame reads the period, zone and currency a frozen state was computed
// under, the instant it describes and the landing it froze.
//
// Read from the snapshot rather than carried on the share row. They are the
// snapshot's own facts, and a copy on the share would be a second place for
// each of them to be wrong.
type shareFrame struct {
	Period       forecasting.Period
	BaseCurrency string
	TakenAt      time.Time
	// SelfCorrecting is whether the frozen chances moved with recorded
	// outcomes; nil on a snapshot frozen before it was kept.
	SelfCorrecting *bool
	// ForwardMeasure is the setting; LandingMeasure is the measure the landing
	// used, and LandingCallMinor the call it used. LandingMeasure is nil on a
	// snapshot frozen before the landing was kept.
	ForwardMeasure   *string
	LandingMeasure   *string
	LandingCallMinor *int64
	// QualifiedPolicy is the viability policy enforced when the snapshot was
	// taken, nil while enforcement was off.
	QualifiedPolicy *string
}

// The timezone is JOINed from the setting row rather than read through
// platform/settings, whose gate is this table's only control. A share link
// carries no seat, so there is no principal to gate against. The frame is one
// row, and a second statement would add a round trip inside the share's
// transaction. Ratified in backend/gates/settingreaders_test.go.
func snapshotFrame(ctx context.Context, tx pgx.Tx, id ids.UUID) (shareFrame, error) {
	var out shareFrame
	var zoneName string
	if err := tx.QueryRow(ctx, `
		SELECT s.period_start, s.period_end, s.base_currency, s.taken_at,
		       COALESCE(st.value #>> '{}', 'UTC'), s.self_correcting,
		       s.forward_measure, s.landing_measure, s.landing_call_minor, s.qualified_policy_version
		FROM forecast_snapshot s
		LEFT JOIN setting st ON st.key = 'installation.timezone'
		WHERE s.id = $1`, id).Scan(
		&out.Period.StartDate, &out.Period.EndDate, &out.BaseCurrency, &out.TakenAt,
		&zoneName, &out.SelfCorrecting,
		&out.ForwardMeasure, &out.LandingMeasure, &out.LandingCallMinor, &out.QualifiedPolicy); err != nil {
		return shareFrame{}, fmt.Errorf("compose: reading a shared snapshot's frame: %w", err)
	}
	zone, err := time.LoadLocation(zoneName)
	if err != nil {
		return shareFrame{}, fmt.Errorf(
			"compose: the installation zone %q is not a zone: %w", zoneName, err)
	}
	out.Period.Zone = zone
	return out, nil
}

// frozenShareLanding is a snapshot's landing over the rows this reader may
// read. It takes the frozen measure, and the frozen call only where the reader
// may see it. A reader who may not falls back as GET /forecast does. The
// caller passes only a frame that kept its landing.
func frozenShareLanding(
	ctx context.Context, tx pgx.Tx, scope forecasting.Scope, readings forecasting.Readings, frame shareFrame,
) (crmcontracts.ForecastLanding, error) {
	measure := forecasting.ForwardMeasure(*frame.LandingMeasure)
	if frame.ForwardMeasure != nil {
		measure = forecasting.ForwardMeasure(*frame.ForwardMeasure)
	}
	var call *int64
	if frame.LandingCallMinor != nil {
		mayAssert, err := forecasting.MayAssertAbout(ctx, tx, ForecastWritableScope, scope)
		if err != nil {
			return crmcontracts.ForecastLanding{}, err
		}
		if mayAssert {
			call = frame.LandingCallMinor
		}
	}
	landing, err := forecasting.ProjectLanding(readings, measure, call)
	if err != nil {
		return crmcontracts.ForecastLanding{}, err
	}
	return forecasting.LandingToWire(landing), nil
}

// sharedQualified rebuilds the qualified readings a snapshot froze, over the
// rows this recipient may read, through forecasting.QualifiedOf: the rule the
// live forecast applies. Nil when no policy was enforced at freeze time.
func sharedQualified(
	ctx context.Context, tx pgx.Tx, snapshotID ids.UUID, frame shareFrame,
) (*forecasting.QualifiedReadings, error) {
	if frame.QualifiedPolicy == nil || *frame.QualifiedPolicy == "" {
		return nil, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	args = append(args, snapshotID)
	visible, err := sharedVisibilityClause(ctx, tx, arg)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, forecasting.ContributionSelect+`
		JOIN deal d ON d.id = c.deal_id
		WHERE c.snapshot_id = $1 AND `+visible, args...)
	if err != nil {
		return nil, fmt.Errorf("compose: reading a shared snapshot's qualified rows: %w", err)
	}
	frozen, err := pgx.CollectRows(rows, forecasting.CollectContribution)
	if err != nil {
		return nil, fmt.Errorf("compose: collecting a shared snapshot's qualified rows: %w", err)
	}
	return forecasting.QualifiedOf(frozen, *frame.QualifiedPolicy)
}
