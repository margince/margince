// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (s *Store) insertSnapshot(ctx context.Context, tx pgx.Tx, in NewSnapshot, landing Landing, capturedBy string) (ids.UUID, error) {
	//craft:ignore naked-any pgx binds heterogeneous scalar values at the SQL boundary.
	arguments := []any{
		in.Period.StartDate, in.Period.EndDate, in.Scope.Kind, in.Scope.ID,
		in.TakenAt, in.Period.LocalDay(in.TakenAt), in.Trigger, DefinitionVersion, in.BaseCurrency,
		in.Readings.WonMinor, in.Readings.EvidenceMinor, in.Readings.BestCaseMinor, in.Readings.OpenMinor,
		in.Readings.WeightedMinor, in.Readings.EligibleCount, in.Readings.PricedCount,
		in.Readings.ConfirmedDateCount, in.Readings.FxMissingCount, in.CallID, capturedBy,
		in.PipelineID, in.PopulationFingerprint,
		nullIfEmpty(string(in.ForwardMeasure)), nullIfEmpty(in.QualifiedPolicyVersion),
		InputFingerprint(in.Readings.Contributions),
		in.SelfCorrecting, in.EvidenceCutoff, nullIfEmpty(in.RiskSignalsVersion),
	}
	arguments = append(arguments, frozenRange(in.Readings.Range)...)
	arguments = append(arguments, frozenLandingColumns(landing)...)
	query := `INSERT INTO forecast_snapshot
        (period_start,period_end,scope_kind,scope_id,taken_at,local_day,trigger,definition_version,
        base_currency,won_minor,evidence_minor,best_case_minor,open_minor,weighted_minor,
        eligible_count,priced_count,confirmed_date_count,fx_missing_count,call_id,captured_by,
        pipeline_id,population_fingerprint,forward_measure,qualified_policy_version,input_fingerprint,
        self_correcting,evidence_cutoff,risk_signals_version,
        range_low_minor,range_high_minor,range_runs,range_quantiles,range_version,
        landing_measure,landing_call_minor) VALUES (` + storekit.Placeholders(arguments) + `) RETURNING id`
	var id ids.UUID
	if err := tx.QueryRow(ctx, query, arguments...).Scan(&id); err != nil {
		return ids.Nil, fmt.Errorf("forecasting: writing the snapshot: %w", err)
	}
	return id, nil
}

// frozenRange is the range columns of a snapshot, all NULL when no range was
// drawn: low, high, runs, quantile convention and range rules.
//
//craft:ignore naked-any pgx binds heterogeneous scalar values at the SQL boundary.
func frozenRange(drawn *Range) []any {
	if drawn == nil {
		return []any{nil, nil, nil, nil, nil}
	}
	return []any{drawn.LowMinor, drawn.HighMinor, drawn.Runs, RangeQuantiles, RangeVersion}
}

// frozenLandingColumns is the measure the landing used and, for a call, the
// call's amount. Both are NULL when the writer named no forward measure.
//
//craft:ignore naked-any pgx binds heterogeneous scalar values at the SQL boundary.
func frozenLandingColumns(landing Landing) []any {
	if landing.Measure == "" {
		return []any{nil, nil}
	}
	if landing.Measure == MeasureManagerCall {
		return []any{string(landing.Measure), landing.AmountMinor}
	}
	return []any{string(landing.Measure), nil}
}
