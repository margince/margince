// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The chance each open deal is weighted by, and the risk signals on it.
//
// Both are read installation-wide as the system, so a deal's chance never
// depends on who reads the forecast. Only counts per stage and per signal leave
// these reads, never another deal's row.

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// forecastChanceActor names the system reads behind a deal's chance.
const forecastChanceActor = "system:forecast_chances"

// forecastChanceModel is what an installation's own evidence says, read once
// per forecast.
type forecastChanceModel struct {
	SelfCorrecting bool
	// Cutoff is the start of the as-of local day; no outcome after it counts.
	Cutoff time.Time
	// Stages holds each open stage's recorded evidence, keyed by stage id.
	Stages map[string]forecasting.StageEvidence
	// Effects holds each risk signal's odds ratio; a signal missing is 1.
	Effects map[string]float64
}

// readForecastChanceModel reads the evidence a forecast taken at `at` stands
// on. While the installation switched self-correction off it reads nothing.
func readForecastChanceModel(ctx context.Context, tx pgx.Tx, period forecasting.Period, at time.Time) (forecastChanceModel, error) {
	on, err := identity.ForecastSelfCorrectingTx(ctx, tx)
	if err != nil {
		return forecastChanceModel{}, err
	}
	out := forecastChanceModel{SelfCorrecting: on, Cutoff: period.LocalDay(at)}
	if !on {
		return out, nil
	}
	system := principal.SystemActing(ctx, forecastChanceActor)
	if out.Stages, err = forecastStageEvidence(system, tx, out.Cutoff); err != nil {
		return forecastChanceModel{}, err
	}
	if out.Effects, err = forecastRiskEffects(system, tx, out.Cutoff); err != nil {
		return forecastChanceModel{}, err
	}
	return out, nil
}

// applyForecastChances sets each deal's typed chance, chance in use, evidence
// count, risk signals and effect. Off, the typed chance stays the chance in use
// and nothing else is set.
func applyForecastChances(ctx context.Context, tx pgx.Tx, model forecastChanceModel, at time.Time, in []forecasting.Deal) error {
	for i := range in {
		in[i].TypedProbability = new(in[i].StageProbability)
	}
	if !model.SelfCorrecting {
		return nil
	}
	signals, err := forecastRiskSignals(principal.SystemActing(ctx, forecastChanceActor), tx, at, in)
	if err != nil {
		return err
	}
	for i := range in {
		evidence := model.Stages[in[i].StageID]
		in[i].StageProbability = forecasting.ChanceInUse(*in[i].TypedProbability, evidence)
		in[i].EvidenceN = new(evidence.Resolved)
		if found, ok := signals[in[i].ID]; ok {
			in[i].Signals = found
			in[i].Effect = forecasting.StrongestEffect(forecasting.SignalCodes(found), model.Effects)
		}
	}
	return nil
}

// forecastStandingSQL is a deal's latest stage move before the cutoff, in the
// order moves were written: how the deal stood then. A reopen after the cutoff
// leaves the earlier close standing. Columns: semantic, at.
func forecastStandingSQL(deal, cutoff string) string {
	return `SELECT x.semantic_at_change AS semantic, x.changed_at AS at FROM deal_stage_history x
		 WHERE x.deal_id = ` + deal + ` AND x.changed_at < ` + cutoff + `
		 ORDER BY ` + weekly.StageMoveOrder("x") + ` DESC LIMIT 1`
}

// forecastStageEntriesSQL is each deal's first recorded entry into an open
// stage in the two years before the cutoff, with how it stood at the cutoff.
// Only moves made in this installation start an entry; imported moves and
// moves saved before their origin was kept do not. A later move into another
// pipeline ends the entry open. Archived deals count by how they closed.
var forecastStageEntriesSQL = `
	WITH entry AS (
		SELECT DISTINCT ON (h.deal_id, h.to_stage_id)
		       h.deal_id, h.to_stage_id AS stage_id, h.pipeline_id_at_change AS pipeline_id, h.changed_at
		  FROM deal_stage_history h
		 WHERE h.provenance = 'recorded' AND h.semantic_at_change = 'open'
		   AND h.pipeline_id_at_change IS NOT NULL
		   AND h.changed_at >= $1 AND h.changed_at < $2
		 ORDER BY h.deal_id, h.to_stage_id, h.changed_at, COALESCE(h.write_seq, 0), h.id
	)
	SELECT e.stage_id,
	       CASE WHEN closed.semantic IS NULL THEN 0 WHEN closed.semantic = 'won' THEN 1 ELSE 2 END,
	       greatest(extract(epoch FROM COALESCE(closed.at, left_pipeline.at, $2) - e.changed_at) / 86400, 0)
	  FROM entry e
	  LEFT JOIN LATERAL (
		SELECT min(x.changed_at) AS at FROM deal_stage_history x
		 WHERE x.deal_id = e.deal_id AND x.changed_at > e.changed_at AND x.changed_at < $2
		   AND x.pipeline_id_at_change IS NOT NULL AND x.pipeline_id_at_change <> e.pipeline_id
	  ) left_pipeline ON true
	  LEFT JOIN LATERAL (` + forecastStandingSQL("e.deal_id", "$2") + `) standing ON true
	  LEFT JOIN LATERAL (
		SELECT standing.semantic, standing.at
		 WHERE standing.semantic IN ('won', 'lost')
		   AND (left_pipeline.at IS NULL OR standing.at <= left_pipeline.at)
	  ) closed ON true`

// forecastStageEvidence reads every open stage's recorded entries and
// estimates its won share.
func forecastStageEvidence(ctx context.Context, tx pgx.Tx, cutoff time.Time) (map[string]forecasting.StageEvidence, error) {
	since := cutoff.AddDate(-conversionWindowYears, 0, 0)
	rows, err := tx.Query(ctx, forecastStageEntriesSQL, since, cutoff)
	if err != nil {
		return nil, fmt.Errorf("compose: reading the recorded stage entries: %w", err)
	}
	defer rows.Close()
	byStage := map[string][]forecasting.StageEntry{}
	for rows.Next() {
		var stage ids.UUID
		var outcome int
		var entry forecasting.StageEntry
		if err := rows.Scan(&stage, &outcome, &entry.Days); err != nil {
			return nil, fmt.Errorf("compose: reading a recorded stage entry: %w", err)
		}
		entry.Outcome = forecasting.EntryOutcome(outcome)
		byStage[stage.String()] = append(byStage[stage.String()], entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("compose: reading the recorded stage entries: %w", err)
	}
	out := make(map[string]forecasting.StageEvidence, len(byStage))
	for stage, entries := range byStage {
		out[stage] = forecasting.EstimateStageEvidence(entries)
	}
	return out, nil
}

// forecastSettledSignalsSQL reads each deal a forecast froze open with signals
// before the cutoff, and that stood closed at the cutoff after it. It answers
// the last such forecast's signals and whether the deal was won. Only signals
// of the current rules are pooled.
var forecastSettledSignalsSQL = `
	SELECT DISTINCT ON (c.deal_id) c.risk_signals, standing.semantic = 'won'
	  FROM forecast_contribution c
	  JOIN forecast_snapshot s ON s.id = c.snapshot_id
	  CROSS JOIN LATERAL (` + forecastStandingSQL("c.deal_id", "$2") + `) standing
	 WHERE c.in_open AND c.risk_signals IS NOT NULL AND s.risk_signals_version = $1
	   AND s.taken_at < $2
	   AND standing.semantic IN ('won', 'lost') AND standing.at > s.taken_at
	 ORDER BY c.deal_id, s.taken_at DESC, s.id DESC`

// forecastRiskEffects tallies frozen deals settled by the cutoff per signal
// and answers each signal's shrunk odds ratio. With none, every effect is 1.
func forecastRiskEffects(ctx context.Context, tx pgx.Tx, cutoff time.Time) (map[string]float64, error) {
	rows, err := tx.Query(ctx, forecastSettledSignalsSQL, assurance.RiskSignalsVersion, cutoff)
	if err != nil {
		return nil, fmt.Errorf("compose: reading the settled forecast signals: %w", err)
	}
	defer rows.Close()
	tallies := map[string]*forecasting.RiskTally{}
	for _, signal := range assurance.RiskSignalNames() {
		tallies[signal] = &forecasting.RiskTally{}
	}
	for rows.Next() {
		var signals []string
		var won bool
		if err := rows.Scan(&signals, &won); err != nil {
			return nil, fmt.Errorf("compose: reading a settled forecast signal: %w", err)
		}
		for name, tally := range tallies {
			present := slices.Contains(signals, name)
			switch {
			case present && won:
				tally.PresentWon++
			case present:
				tally.PresentLost++
			case won:
				tally.AbsentWon++
			default:
				tally.AbsentLost++
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("compose: reading the settled forecast signals: %w", err)
	}
	out := make(map[string]float64, len(tallies))
	for name, tally := range tallies {
		out[name] = forecasting.RiskEffect(*tally)
	}
	return out, nil
}

// forecastRiskSignals assesses the open deals through the assurance subjects,
// the evidence the nightly check reads, and answers their signals by deal id.
func forecastRiskSignals(ctx context.Context, tx pgx.Tx, at time.Time, in []forecasting.Deal) (map[string][]forecasting.Signal, error) {
	open := make([]ids.UUID, 0, len(in))
	for _, deal := range in {
		if deal.Won || deal.Lost {
			continue
		}
		id, err := ids.Parse(deal.ID)
		if err != nil {
			return nil, fmt.Errorf("compose: a forecast deal id %q does not parse: %w", deal.ID, err)
		}
		open = append(open, id)
	}
	out := map[string][]forecasting.Signal{}
	if len(open) == 0 {
		return out, nil
	}
	zone, err := installationZoneTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	subjects, err := assuranceSubjects(ctx, tx, at, AssuranceCoverage(ctx, tx, at), open)
	if err != nil {
		return nil, err
	}
	stuck, err := forecastStuckAfter(ctx, tx, open)
	if err != nil {
		return nil, err
	}
	cfg := AssuranceConfig()
	for _, subject := range subjects {
		var threshold *time.Duration
		if pace, ok := stuck[subject.StageID]; ok {
			threshold = &pace
		}
		out[subject.DealID] = forecastSignals(assurance.RiskSignals(at, subject, cfg, threshold, zone))
	}
	return out, nil
}

// forecastStuckAfter reads the won-deal stage pace of every pipeline the deals
// sit in, keyed by stage id.
func forecastStuckAfter(ctx context.Context, tx pgx.Tx, dealIDs []ids.UUID) (map[string]time.Duration, error) {
	rows, err := tx.Query(ctx, `SELECT DISTINCT pipeline_id FROM deal WHERE id = ANY($1) AND pipeline_id IS NOT NULL`, dealIDs)
	if err != nil {
		return nil, fmt.Errorf("compose: reading the forecast's pipelines: %w", err)
	}
	pipelines, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("compose: reading the forecast's pipelines: %w", err)
	}
	out := map[string]time.Duration{}
	for _, pipeline := range pipelines {
		pace, err := deals.StuckAfter(ctx, tx, pipeline, analyticsquery.PercentileSampleFloor)
		if err != nil {
			return nil, err
		}
		maps.Copy(out, pace)
	}
	return out, nil
}

// ForecastStageChances answers every live open stage's typed chance, its
// recorded evidence and the chance the forecast uses, as of `at`. The evidence
// is read even while self-correction is off, so an admin sees what turning it
// on would do.
func ForecastStageChances(ctx context.Context, tx pgx.Tx, at time.Time) (forecasting.StageChances, error) {
	period, _, err := ForecastPeriodAt(ctx, tx, forecasting.PeriodQuarter, at)
	if err != nil {
		return forecasting.StageChances{}, err
	}
	on, err := identity.ForecastSelfCorrectingTx(ctx, tx)
	if err != nil {
		return forecasting.StageChances{}, err
	}
	out := forecasting.StageChances{SelfCorrecting: on, EvidenceCutoff: period.LocalDay(at)}
	evidence, err := forecastStageEvidence(principal.SystemActing(ctx, forecastChanceActor), tx, out.EvidenceCutoff)
	if err != nil {
		return forecasting.StageChances{}, err
	}
	rows, err := tx.Query(ctx, `
		SELECT s.id, s.win_probability FROM stage s
		  JOIN pipeline p ON p.id = s.pipeline_id
		 WHERE s.semantic = 'open' AND s.archived_at IS NULL AND p.archived_at IS NULL
		 ORDER BY p.position, s.position, s.id`)
	if err != nil {
		return forecasting.StageChances{}, fmt.Errorf("compose: reading the open stages: %w", err)
	}
	out.Stages, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (forecasting.StageChance, error) {
		var stage forecasting.StageChance
		err := row.Scan(&stage.StageID, &stage.Typed)
		stage.Evidence = evidence[stage.StageID.String()]
		stage.ChanceInUse = stage.Typed
		if on {
			stage.ChanceInUse = forecasting.ChanceInUse(stage.Typed, stage.Evidence)
		}
		return stage, err
	})
	if err != nil {
		return forecasting.StageChances{}, fmt.Errorf("compose: reading the open stages: %w", err)
	}
	return out, nil
}

// forecastSignals carries the assessment's signals and their facts across to
// the forecast's own type, field for field.
func forecastSignals(in []assurance.RiskSignal) []forecasting.Signal {
	out := make([]forecasting.Signal, len(in))
	for i, signal := range in {
		out[i] = forecasting.Signal{
			Code: signal.Code, Days: signal.Days, Count: signal.Count, StageID: signal.StageID,
			ClosePassed: signal.ClosePassed, ClosesInDays: signal.ClosesInDays,
		}
	}
	return out
}
