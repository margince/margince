// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package winchance

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
	"sync"
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
	"github.com/margince/margince/backend/internal/shared/kernel/stagemove"
	"github.com/margince/margince/backend/internal/shared/kernel/winprobability"
)

// forecastChanceActor names the system reads behind a deal's chance.
const forecastChanceActor = "system:forecast_chances"

// Model is what an installation's own evidence says, read once
// per forecast.
type Model struct {
	forecasting.ChanceModel
	// Cutoff is the start of the as-of local day; no outcome after it counts.
	Cutoff time.Time
}

// ReadModel reads the evidence a forecast with this cutoff, the
// start of its as-of local day, stands on. While the installation switched
// self-correction off it reads nothing. Evidence the small-group floor holds
// back from any reader is left out for every reader, so one deal reads one
// chance wherever it is shown.
func (e *Engine) ReadModel(ctx context.Context, tx pgx.Tx, cutoff time.Time) (Model, error) {
	if model, ok := chanceModelMemo.get(tx, cutoff); ok {
		return model, nil
	}
	model, err := e.buildModel(ctx, tx, cutoff)
	if err == nil {
		chanceModelMemo.put(tx, cutoff, model)
	}
	return model, err
}

// chanceModelMemo keeps the model one transaction last read. A request reads it
// for its list page, its filter and its reports on one transaction. That
// transaction sees one snapshot of the evidence, so the answer is the same each
// time. It is not a cache across requests: another transaction never matches.
var chanceModelMemo modelMemo

type modelMemo struct {
	mu     sync.Mutex
	tx     pgx.Tx
	cutoff time.Time
	model  Model
}

func (m *modelMemo) get(tx pgx.Tx, cutoff time.Time) (Model, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.tx == nil || m.tx != tx || !m.cutoff.Equal(cutoff) {
		return Model{}, false
	}
	return m.model, true
}

func (m *modelMemo) put(tx pgx.Tx, cutoff time.Time, model Model) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tx, m.cutoff, m.model = tx, cutoff, model
}

func (e *Engine) buildModel(ctx context.Context, tx pgx.Tx, cutoff time.Time) (Model, error) {
	on, err := identity.ForecastSelfCorrectingTx(ctx, tx)
	if err != nil {
		return Model{}, err
	}
	out := Model{SelfCorrecting: on, QuietSignal: assurance.SignalQuiet, Cutoff: cutoff}
	if !on {
		return out, nil
	}
	system := principal.SystemActing(ctx, forecastChanceActor)
	if out.Stages, err = e.flooredStageEvidence(ctx, system, tx, out.Cutoff); err != nil {
		return Model{}, err
	}
	tallies, owners, err := forecastRiskTallies(system, tx, out.Cutoff)
	if err != nil {
		return Model{}, err
	}
	thin, err := e.ThinForAnyReader(ctx, tx, [][]ids.UUID{owners})
	if err != nil {
		return Model{}, err
	}
	// The learned effects pool every settled deal, so one owner's deals behind
	// them leave the fixed fallback and no learned effect.
	if !thin[0] {
		out.Effects, out.Samples = riskEffectsAndSamples(tallies)
	}
	return out, nil
}

// flooredStageEvidence is StageEvidence without the stages whose recorded
// owners the small-group floor holds back from some reader.
func (e *Engine) flooredStageEvidence(ctx, system context.Context, tx pgx.Tx, cutoff time.Time) (map[string]forecasting.StageEvidence, error) {
	evidence, err := StageEvidence(system, tx, cutoff)
	if err != nil {
		return nil, err
	}
	stages := slices.Sorted(maps.Keys(evidence))
	groups := make([][]ids.UUID, len(stages))
	for i, stage := range stages {
		groups[i] = evidence[stage].Seats
	}
	thin, err := e.ThinForAnyReader(ctx, tx, groups)
	if err != nil {
		return nil, err
	}
	for i, stage := range stages {
		if thin[i] {
			delete(evidence, stage)
		}
	}
	return evidence, nil
}

// Apply sets each deal's typed chance, chance in use, evidence
// count, risk signals and effect from the one resolver. It also sets the
// figure the forecast weights by and tests for Likely.
func (e *Engine) Apply(ctx context.Context, tx pgx.Tx, model Model, at time.Time, in []forecasting.Deal) error {
	chances, signals, err := e.resolve(ctx, tx, model, at, in)
	if err != nil {
		return err
	}
	for i, chance := range chances {
		in[i].TypedProbability = new(chance.Typed)
		if in[i].Source == forecasting.SourceStage {
			in[i].StageProbability = chance.InUse
			in[i].EvidenceN = chance.EvidenceN
			in[i].Effect = chance.Effect
			in[i].Signals = signals[in[i].ID]
		}
		in[i].WinProbability = chance.Percent
		in[i].Explanation = chance.Line()
	}
	return nil
}

// resolve runs every deal through forecasting.ResolveWinChance,
// in the deals' order, and answers the risk signals it read.
func (e *Engine) resolve(
	ctx context.Context, tx pgx.Tx, model Model, at time.Time, in []forecasting.Deal,
) ([]winprobability.Chance, map[string][]forecasting.Signal, error) {
	var signals map[string][]forecasting.Signal
	if model.SelfCorrecting {
		var err error
		if signals, err = e.riskSignals(principal.SystemActing(ctx, forecastChanceActor), tx, at, in); err != nil {
			return nil, nil, err
		}
	}
	out := make([]winprobability.Chance, len(in))
	for i := range in {
		out[i] = forecasting.ResolveWinChance(model.ChanceModel, forecasting.ChanceInput{
			StageID: in[i].StageID, Typed: in[i].StageProbability, Own: in[i].WinProbability,
			Source: in[i].Source, Signals: signals[in[i].ID],
		})
	}
	return out, signals, nil
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
// Only moves made in this installation start an entry; imported or synced
// moves and moves saved before their origin was kept do not. A later move into another
// pipeline ends the entry open. Archived deals count by how they closed.
var forecastStageEntriesSQL = `
	WITH entry AS (
		SELECT DISTINCT ON (h.deal_id, h.to_stage_id)
		       h.deal_id, h.to_stage_id AS stage_id, h.pipeline_id_at_change AS pipeline_id, h.changed_at
		  FROM deal_stage_history h
		 WHERE h.provenance = '` + stagemove.Recorded + `' AND h.semantic_at_change = 'open'
		   AND NOT EXISTS (SELECT 1 FROM deal m WHERE m.id = h.deal_id AND m.merged_into_id IS NOT NULL)
		   AND h.pipeline_id_at_change IS NOT NULL
		   AND h.changed_at >= $1 AND h.changed_at < $2
		 ORDER BY h.deal_id, h.to_stage_id, h.changed_at, COALESCE(h.write_seq, 0), h.id
	)
	SELECT e.stage_id, (SELECT d.owner_id FROM deal d WHERE d.id = e.deal_id),
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

// EvidenceWindowYears is how far back a stage's recorded entries count.
const EvidenceWindowYears = 2

// StageEvidence reads every open stage's recorded entries and
// estimates its won share.
func StageEvidence(ctx context.Context, tx pgx.Tx, cutoff time.Time) (map[string]forecasting.StageEvidence, error) {
	since := cutoff.AddDate(-EvidenceWindowYears, 0, 0)
	rows, err := tx.Query(ctx, forecastStageEntriesSQL, since, cutoff)
	if err != nil {
		return nil, fmt.Errorf("compose: reading the recorded stage entries: %w", err)
	}
	defer rows.Close()
	byStage, seats := map[string][]forecasting.StageEntry{}, map[string][]ids.UUID{}
	for rows.Next() {
		var stage ids.UUID
		var owner *ids.UUID
		var outcome int
		var entry forecasting.StageEntry
		if err := rows.Scan(&stage, &owner, &outcome, &entry.Days); err != nil {
			return nil, fmt.Errorf("compose: reading a recorded stage entry: %w", err)
		}
		entry.Outcome = forecasting.EntryOutcome(outcome)
		byStage[stage.String()] = append(byStage[stage.String()], entry)
		if owner != nil {
			seats[stage.String()] = append(seats[stage.String()], *owner)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("compose: reading the recorded stage entries: %w", err)
	}
	out := make(map[string]forecasting.StageEvidence, len(byStage))
	for stage, entries := range byStage {
		evidence := forecasting.EstimateStageEvidence(entries)
		evidence.Seats = seats[stage]
		out[stage] = evidence
	}
	return out, nil
}

// forecastSettledSignalsSQL reads each deal a forecast froze open with signals
// before the cutoff, and that stood closed at the cutoff after it. It answers
// the last such forecast's signals and whether the deal was won. Only signals
// of the current rules are pooled.
var forecastSettledSignalsSQL = `
	SELECT DISTINCT ON (c.deal_id) c.risk_signals, standing.semantic = 'won',
	       (SELECT d.owner_id FROM deal d WHERE d.id = c.deal_id)
	  FROM forecast_contribution c
	  JOIN forecast_snapshot s ON s.id = c.snapshot_id
	  CROSS JOIN LATERAL (` + forecastStandingSQL("c.deal_id", "$2") + `) standing
	 WHERE c.in_open AND c.risk_signals IS NOT NULL AND s.risk_signals_version = $1
	   AND s.taken_at < $2
	   AND standing.semantic IN ('won', 'lost') AND standing.at > s.taken_at
	 ORDER BY c.deal_id, s.taken_at DESC, s.id DESC`

// RiskEffects answers each signal's shrunk odds ratio from the frozen
// deals settled by the cutoff. With none, every effect is 1.
func RiskEffects(ctx context.Context, tx pgx.Tx, cutoff time.Time) (map[string]float64, error) {
	tallies, _, err := forecastRiskTallies(ctx, tx, cutoff)
	if err != nil {
		return nil, err
	}
	effects, _ := riskEffectsAndSamples(tallies)
	return effects, nil
}

// riskEffectsAndSamples turns each signal's tally into its odds ratio and the
// counts of settled deals that carried it and that did not.
func riskEffectsAndSamples(tallies map[string]*forecasting.RiskTally) (map[string]float64, map[string]forecasting.RiskSample) {
	effects := make(map[string]float64, len(tallies))
	samples := make(map[string]forecasting.RiskSample, len(tallies))
	for name, tally := range tallies {
		effects[name] = forecasting.RiskEffect(*tally)
		samples[name] = forecasting.RiskSample{
			Present: tally.PresentWon + tally.PresentLost, Absent: tally.AbsentWon + tally.AbsentLost,
		}
	}
	return effects, samples
}

// forecastRiskTallies tallies frozen deals settled by the cutoff per signal,
// with the owners of those deals.
func forecastRiskTallies(ctx context.Context, tx pgx.Tx, cutoff time.Time) (map[string]*forecasting.RiskTally, []ids.UUID, error) {
	rows, err := tx.Query(ctx, forecastSettledSignalsSQL, assurance.RiskSignalsVersion, cutoff)
	if err != nil {
		return nil, nil, fmt.Errorf("compose: reading the settled forecast signals: %w", err)
	}
	defer rows.Close()
	tallies := map[string]*forecasting.RiskTally{}
	for _, signal := range assurance.RiskSignalNames() {
		tallies[signal] = &forecasting.RiskTally{}
	}
	var owners []ids.UUID
	for rows.Next() {
		var signals []string
		var won bool
		var owner *ids.UUID
		if err := rows.Scan(&signals, &won, &owner); err != nil {
			return nil, nil, fmt.Errorf("compose: reading a settled forecast signal: %w", err)
		}
		if owner != nil {
			owners = append(owners, *owner)
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
		return nil, nil, fmt.Errorf("compose: reading the settled forecast signals: %w", err)
	}
	return tallies, owners, nil
}

// riskSignals assesses the open deals through the assurance subjects,
// the evidence the nightly check reads, and answers their signals by deal id.
func (e *Engine) riskSignals(ctx context.Context, tx pgx.Tx, at time.Time, in []forecasting.Deal) (map[string][]forecasting.Signal, error) {
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
	zone, err := identity.ZoneAppliedTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	subjects, cfg, err := e.RiskSubjects(ctx, tx, at, open)
	if err != nil {
		return nil, err
	}
	stuck, err := stuckAfter(ctx, tx, open)
	if err != nil {
		return nil, err
	}
	for _, subject := range subjects {
		var threshold *time.Duration
		if pace, ok := stuck[subject.StageID]; ok {
			threshold = &pace
		}
		out[subject.DealID] = forecastSignals(assurance.RiskSignals(at, subject, cfg, threshold, zone))
	}
	return out, nil
}

// stuckAfter reads the won-deal stage pace of every pipeline the deals
// sit in, keyed by stage id, in one read.
func stuckAfter(ctx context.Context, tx pgx.Tx, dealIDs []ids.UUID) (map[string]time.Duration, error) {
	return deals.StuckAfterDealPipelines(ctx, tx, dealIDs, analyticsquery.PercentileSampleFloor)
}

// StageChances answers every live open stage's typed chance, its
// recorded evidence and the chance the forecast uses, as of `at`. The evidence
// is read even while self-correction is off, so an admin sees what turning it
// on would do.
func (e *Engine) StageChances(ctx context.Context, tx pgx.Tx, at time.Time) (forecasting.StageChances, error) {
	period, err := e.PeriodAt(ctx, tx, forecasting.PeriodQuarter, at)
	if err != nil {
		return forecasting.StageChances{}, err
	}
	on, err := identity.ForecastSelfCorrectingTx(ctx, tx)
	if err != nil {
		return forecasting.StageChances{}, err
	}
	accuracy, err := e.Accuracy(ctx, tx, at)
	if err != nil {
		return forecasting.StageChances{}, err
	}
	out := forecasting.StageChances{SelfCorrecting: on, EvidenceCutoff: period.LocalDay(at), Accuracy: accuracy}
	evidence, err := e.flooredStageEvidence(ctx, principal.SystemActing(ctx, forecastChanceActor), tx, out.EvidenceCutoff)
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
