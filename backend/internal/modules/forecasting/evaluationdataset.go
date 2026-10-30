// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// EvaluationDatasetVersion names the shape of EvaluationDataset. Evaluate
// refuses a dataset of another shape.
const EvaluationDatasetVersion = "forecast_evaluation_dataset_v1"

// EvaluationDataset is the versioned input of one forecast evaluation: the
// frozen snapshots, oldest first, and the outcome of each period as it stood
// at OutcomesAsOf. It holds the workspace scope only and names no deal, team
// or seat. A copy kept for a rerun holds nothing an erasure has to reach.
type EvaluationDataset struct {
	Version string `json:"version"`
	// DefinitionVersion is the reading rules of the build that read the
	// outcomes. A snapshot frozen under other rules is not compared with them.
	DefinitionVersion string    `json:"definition_version"`
	OutcomesAsOf      time.Time `json:"outcomes_as_of"`
	// OutcomesDay is OutcomesAsOf's local day in the installation's zone.
	OutcomesDay time.Time `json:"outcomes_day"`
	// Truncated is true when the read stopped at its vintage bound, so the
	// newest snapshots are missing from Vintages.
	Truncated bool `json:"truncated"`
	// OtherScopes counts the team and owner snapshots left out. Their outcome
	// needs the population of a past day, which no snapshot stores.
	OtherScopes int                 `json:"other_scopes"`
	Vintages    []EvaluationVintage `json:"vintages"`
}

// EvaluationVintage is one frozen snapshot and what it predicted. Every field
// but OpenDeals' labels and Outcome was stored when the snapshot was taken.
type EvaluationVintage struct {
	ID string `json:"id"`
	// Context is CaptureKey of the workspace scope and the snapshot's pipeline.
	Context     string     `json:"context"`
	PipelineID  *ids.UUID  `json:"pipeline_id,omitempty"`
	PeriodStart time.Time  `json:"period_start"`
	PeriodEnd   time.Time  `json:"period_end"`
	LocalDay    time.Time  `json:"local_day"`
	TakenAt     time.Time  `json:"taken_at"`
	Trigger     string     `json:"trigger"`
	Definition  string     `json:"definition_version"`
	Currency    string     `json:"base_currency"`
	Measure     string     `json:"forward_measure,omitempty"`
	Policy      string     `json:"qualified_policy_version,omitempty"`
	Population  string     `json:"population_fingerprint"`
	Eligible    int        `json:"eligible_count"`
	Priced      int        `json:"priced_count"`
	FxMissing   int        `json:"fx_missing_count"`
	Rows        int        `json:"contribution_count"`
	WonMinor    int64      `json:"won_minor"`
	Evidence    int64      `json:"evidence_minor"`
	Weighted    int64      `json:"weighted_minor"`
	OpenMinor   int64      `json:"open_minor"`
	OpenDeals   []OpenDeal `json:"open_deals"`
	// Outcome is nil until the period has ended and its won money was read.
	Outcome *VintageOutcome `json:"outcome,omitempty"`
}

// OpenDeal is one open deal a snapshot froze, without its id or money.
type OpenDeal struct {
	// DealID joins the deal to its outcome while the dataset is built, and is
	// never written out.
	DealID           string `json:"-"`
	StageProbability *int   `json:"stage_probability"`
	InEvidence       bool   `json:"in_evidence"`
	// WonInHorizon is the label: whether the deal was won inside the period.
	// Nil while the vintage has no outcome.
	WonInHorizon *bool `json:"won_in_horizon"`
}

// VintageOutcome is the money a period actually won, in the vintage's currency.
type VintageOutcome struct {
	WonMinor int64 `json:"won_minor"`
	// FxMissing counts won deals no rate converted. WonMinor then leaves them
	// out, and the evaluation excludes the vintage.
	FxMissing int `json:"fx_missing_count"`
}

// EvaluationSnapshots is what EvaluationVintagesTx read.
type EvaluationSnapshots struct {
	Vintages []EvaluationVintage
	// Truncated is true when more workspace snapshots exist than the bound.
	Truncated   bool
	OtherScopes int
}

// EvaluationVintagesTx reads up to limit workspace snapshots, oldest first,
// with the open deals each froze.
//
// The rows come from the snapshot alone, so a deal created or changed after
// a snapshot was taken cannot reach that snapshot's features.
func (s *Store) EvaluationVintagesTx(ctx context.Context, tx pgx.Tx, limit, maxDeals int) (EvaluationSnapshots, error) {
	if err := auth.Require(ctx, "forecast", principal.ActionRead); err != nil {
		return EvaluationSnapshots{}, err
	}
	// Every workspace snapshot and its deal ids, under no row scope: only a
	// system pass, never a seat, may read the whole installation's history.
	if actor, ok := principal.Actor(ctx); !ok || actor.Type != principal.PrincipalSystem {
		return EvaluationSnapshots{}, apperrors.ErrPermissionDenied
	}
	var out EvaluationSnapshots
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM forecast_snapshot WHERE scope_kind <> $1`,
		ScopeWorkspace).Scan(&out.OtherScopes); err != nil {
		return EvaluationSnapshots{}, fmt.Errorf("forecasting: counting the snapshots of other scopes: %w", err)
	}
	rows, err := tx.Query(ctx, `
		SELECT id, pipeline_id, period_start, period_end, local_day, taken_at,
		       trigger, definition_version, base_currency, COALESCE(forward_measure, ''),
		       COALESCE(qualified_policy_version, ''), population_fingerprint,
		       eligible_count, priced_count, fx_missing_count,
		       won_minor, evidence_minor, weighted_minor, open_minor
		  FROM forecast_snapshot
		 WHERE scope_kind = $1
		 ORDER BY taken_at, id
		 LIMIT $2`, ScopeWorkspace, limit+1)
	if err != nil {
		return EvaluationSnapshots{}, fmt.Errorf("forecasting: reading the evaluation's snapshots: %w", err)
	}
	out.Vintages, err = pgx.CollectRows(rows, scanEvaluationVintage)
	if err != nil {
		return EvaluationSnapshots{}, fmt.Errorf("forecasting: collecting the evaluation's snapshots: %w", err)
	}
	if len(out.Vintages) > limit {
		out.Vintages, out.Truncated = out.Vintages[:limit], true
	}
	if err := attachOpenDeals(ctx, tx, out.Vintages, maxDeals); err != nil {
		return EvaluationSnapshots{}, err
	}
	return out, nil
}

func scanEvaluationVintage(row pgx.CollectableRow) (EvaluationVintage, error) {
	var v EvaluationVintage
	var id ids.UUID
	err := row.Scan(&id, &v.PipelineID, &v.PeriodStart, &v.PeriodEnd,
		&v.LocalDay, &v.TakenAt, &v.Trigger, &v.Definition, &v.Currency, &v.Measure, &v.Policy,
		&v.Population, &v.Eligible, &v.Priced, &v.FxMissing,
		&v.WonMinor, &v.Evidence, &v.Weighted, &v.OpenMinor)
	v.ID = id.String()
	v.Context = CaptureKey(Scope{Kind: ScopeWorkspace}, v.PipelineID)
	return v, err
}

// attachOpenDeals reads every vintage's contribution rows in one statement,
// refusing past maxDeals rows rather than holding an unbounded set.
func attachOpenDeals(ctx context.Context, tx pgx.Tx, vintages []EvaluationVintage, maxDeals int) error {
	if len(vintages) == 0 {
		return nil
	}
	index := make(map[string]int, len(vintages))
	snapshotIDs := make([]string, len(vintages))
	for i, v := range vintages {
		index[v.ID], snapshotIDs[i] = i, v.ID
	}
	rows, err := tx.Query(ctx, `
		SELECT snapshot_id, deal_id, stage_probability, in_open, in_evidence
		  FROM forecast_contribution
		 WHERE snapshot_id = ANY($1::uuid[])
		 ORDER BY snapshot_id, deal_id
		 LIMIT $2`, snapshotIDs, maxDeals+1)
	if err != nil {
		return fmt.Errorf("forecasting: reading the evaluation's deals: %w", err)
	}
	defer rows.Close()
	read := 0
	for rows.Next() {
		if read++; read > maxDeals {
			return fmt.Errorf("forecasting: the evaluation's snapshots hold more than %d deal rows; "+
				"read fewer snapshots or raise the deal bound", maxDeals)
		}
		var snapshotID, dealID ids.UUID
		var deal OpenDeal
		var open bool
		if err := rows.Scan(&snapshotID, &dealID, &deal.StageProbability, &open, &deal.InEvidence); err != nil {
			return fmt.Errorf("forecasting: scanning the evaluation's deals: %w", err)
		}
		v := &vintages[index[snapshotID.String()]]
		v.Rows++
		if open {
			deal.DealID = dealID.String()
			v.OpenDeals = append(v.OpenDeals, deal)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("forecasting: reading the evaluation's deals: %w", err)
	}
	return nil
}
