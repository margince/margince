// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// EvaluationDatasetVersion names the shape of EvaluationDataset. Evaluate
// refuses a dataset of another shape.
const EvaluationDatasetVersion = "forecast_evaluation_dataset_v3"

// EvaluationActor is the one principal EvaluationVintagesTx answers.
const EvaluationActor = "system:forecast-evaluation"

// EvaluationDataset is the versioned input of one forecast evaluation: the
// frozen snapshots, oldest first, each with the outcome of its own open deals.
// It holds the workspace scope only and names no deal, team or seat.
type EvaluationDataset struct {
	Version string `json:"version"`
	// DefinitionVersion is the reading rules of the build that read the
	// outcomes. A snapshot frozen under other rules is not compared with them.
	DefinitionVersion string `json:"definition_version"`
	// OutcomesAsOf is the instant the scoring labels were read at, and
	// OutcomesDay its local day in the installation's zone.
	OutcomesAsOf time.Time `json:"outcomes_as_of"`
	OutcomesDay  time.Time `json:"outcomes_day"`
	// TrainEnd is the local day the training labels were read at. Only a plan
	// with the same training end may score this dataset.
	TrainEnd time.Time `json:"train_end"`
	// CohortFloor is the fewest deals a vintage or a bin needs before its
	// money or its share is reported.
	CohortFloor int `json:"cohort_floor"`
	// Truncated is true when the read stopped at its vintage bound, so the
	// newest snapshots are missing from Vintages.
	Truncated bool `json:"truncated"`
	// Snapshots left out by the read, counted by reason. Their figures would
	// describe too few deals, a team or owner, or one seat's work.
	LeftOut  []ExclusionCount    `json:"left_out"`
	Vintages []EvaluationVintage `json:"vintages"`
}

// EvaluationVintage is one frozen snapshot and what it predicted. Every field
// but the outcomes and Tallies was stored when the snapshot was taken.
type EvaluationVintage struct {
	ID string `json:"id"`
	// Context is CaptureKey of the workspace scope and the snapshot's pipeline.
	Context     string    `json:"context"`
	PipelineID  *ids.UUID `json:"pipeline_id,omitempty"`
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`
	LocalDay    time.Time `json:"local_day"`
	TakenAt     time.Time `json:"taken_at"`
	Trigger     string    `json:"trigger"`
	Definition  string    `json:"definition_version"`
	Currency    string    `json:"base_currency"`
	Measure     string    `json:"forward_measure,omitempty"`
	Policy      string    `json:"qualified_policy_version,omitempty"`
	Population  string    `json:"population_fingerprint"`
	Eligible    int       `json:"eligible_count"`
	Priced      int       `json:"priced_count"`
	FxMissing   int       `json:"fx_missing_count"`
	Rows        int       `json:"contribution_count"`
	WonMinor    int64     `json:"won_minor"`
	Evidence    int64     `json:"evidence_minor"`
	Weighted    int64     `json:"weighted_minor"`
	OpenMinor   int64     `json:"open_minor"`
	// SelfCorrecting is whether the chances moved with recorded outcomes when
	// the snapshot was taken; nil when it was not recorded.
	SelfCorrecting *bool `json:"self_correcting,omitempty"`
	// TypedWeighted is the open deals at their typed chances, rounded per deal
	// as the weighted reading rounds.
	TypedWeighted int64 `json:"typed_weighted_minor"`
	// The frozen range, nil on a snapshot taken before ranges were drawn.
	RangeLowMinor  *int64 `json:"range_low_minor,omitempty"`
	RangeHighMinor *int64 `json:"range_high_minor,omitempty"`
	// Sufficiency is how much recorded evidence the open deals stood on.
	Sufficiency string `json:"evidence_sufficiency"`
	// Outcome is the vintage's won money plus what its open deals won inside
	// the period, as the history stood at OutcomesAsOf. TrainOutcome is the
	// same as the history stood at TrainEnd. Nil before the period ended.
	Outcome      *VintageOutcome    `json:"outcome,omitempty"`
	TrainOutcome *VintageOutcome    `json:"train_outcome,omitempty"`
	Tallies      CalibrationTallies `json:"tallies"`
	// OpenDeals and Owners exist only while the dataset is built, and are
	// never written out.
	OpenDeals []OpenDeal `json:"-"`
	Owners    []ids.UUID `json:"-"`
}

// OpenDeal is one open deal a snapshot froze. StageProbability is the chance
// in use and TypedProbability the typed chance. TypedProbability is nil on a
// row frozen before it was kept, when the chance in use was the typed one.
type OpenDeal struct {
	DealID           string
	BaseMinor        *int64
	StageProbability *int
	TypedProbability *int
	EvidenceN        *int
	Effect           *float64
	InEvidence       bool
	// WonInHorizon is whether the deal was won inside the period as the
	// history stood at OutcomesAsOf. Nil when the history cannot say.
	WonInHorizon *bool
}

// VintageOutcome is money won, in the vintage's currency.
type VintageOutcome struct {
	WonMinor int64 `json:"won_minor"`
	// FxMissing counts won deals whose win recorded no amount in the vintage's
	// currency. WonMinor leaves them out, and the evaluation excludes the vintage.
	FxMissing int `json:"fx_missing_count"`
}

// CalibrationTallies are a vintage's labelled open deals, counted per bin:
// by typed chance, by applied chance in use, and by commit.
type CalibrationTallies struct {
	Labelled      int         `json:"labelled"`
	NoProbability int         `json:"no_probability"`
	Typed         ChanceTally `json:"typed"`
	Applied       ChanceTally `json:"applied"`
	Commit        [2]Tally    `json:"commit"`
}

// ChanceTally is one way of stating each deal's chance, binned by tenths.
// BrierSum is the sum of (probability - 100 x won) squared over the deals.
type ChanceTally struct {
	Bins     [10]Tally `json:"bins"`
	BrierSum int64     `json:"brier_sum"`
}

func (c *ChanceTally) add(p, won int64) {
	bin := min(p/10, 9)
	c.Bins[bin].Deals++
	c.Bins[bin].Won += won
	c.Bins[bin].PercentSum += p
	c.BrierSum += (p - 100*won) * (p - 100*won)
}

// Tally is one bin's deals, how many were won, and their summed probability.
type Tally struct {
	Deals      int64 `json:"deals"`
	Won        int64 `json:"won"`
	PercentSum int64 `json:"percent_sum"`
}

// TallyOpenDeals counts a vintage's labelled open deals into their bins.
func TallyOpenDeals(deals []OpenDeal) CalibrationTallies {
	var out CalibrationTallies
	for _, deal := range deals {
		if deal.WonInHorizon == nil {
			continue
		}
		out.Labelled++
		won := int64(0)
		if *deal.WonInHorizon {
			won = 1
		}
		commit := 0
		if deal.InEvidence {
			commit = 1
		}
		out.Commit[commit].Deals++
		out.Commit[commit].Won += won
		if deal.StageProbability == nil {
			out.NoProbability++
			continue
		}
		typed := deal.StageProbability
		if deal.TypedProbability != nil {
			typed = deal.TypedProbability
		}
		out.Typed.add(int64(*typed), won)
		out.Applied.add(int64(AppliedProbability(*deal.StageProbability, deal.Effect)), won)
	}
	return out
}

// summarizeOpenDeals is a vintage's open deals at their typed chances, and how
// much recorded evidence they stood on.
func summarizeOpenDeals(deals []OpenDeal) (int64, string, error) {
	var typedWeighted int64
	withEvidence, sufficient := 0, 0
	for _, deal := range deals {
		if deal.EvidenceN != nil && *deal.EvidenceN > 0 {
			withEvidence++
			if *deal.EvidenceN >= PriorStrength {
				sufficient++
			}
		}
		chance := deal.TypedProbability
		if chance == nil {
			chance = deal.StageProbability
		}
		if deal.BaseMinor == nil || chance == nil {
			continue
		}
		weighted, err := values.WeightedValue(*deal.BaseMinor, *chance)
		if err != nil {
			return 0, "", fmt.Errorf("forecasting: weighting deal %s at its typed chance: %w", deal.DealID, err)
		}
		if typedWeighted, err = addMinor(typedWeighted, weighted); err != nil {
			return 0, "", err
		}
	}
	switch {
	case withEvidence == 0:
		return typedWeighted, EvidenceNone, nil
	case 2*sufficient >= len(deals):
		return typedWeighted, EvidenceSufficient, nil
	default:
		return typedWeighted, EvidenceThin, nil
	}
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
	// Every workspace snapshot and its deal ids, under no row scope: only the
	// evaluation's own system pass may read the whole installation's history.
	if actor, ok := principal.Actor(ctx); !ok || actor.Type != principal.PrincipalSystem || actor.ID != EvaluationActor {
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
		       won_minor, evidence_minor, weighted_minor, open_minor,
		       self_correcting, range_low_minor, range_high_minor
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
		&v.WonMinor, &v.Evidence, &v.Weighted, &v.OpenMinor,
		&v.SelfCorrecting, &v.RangeLowMinor, &v.RangeHighMinor)
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
		SELECT snapshot_id, deal_id, owner_id, stage_probability, in_open, in_evidence,
		       base_minor, typed_probability, evidence_n, risk_effect
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
		var owner *ids.UUID
		var deal OpenDeal
		var open bool
		if err := rows.Scan(&snapshotID, &dealID, &owner, &deal.StageProbability, &open, &deal.InEvidence,
			&deal.BaseMinor, &deal.TypedProbability, &deal.EvidenceN, &deal.Effect); err != nil {
			return fmt.Errorf("forecasting: scanning the evaluation's deals: %w", err)
		}
		v := &vintages[index[snapshotID.String()]]
		v.Rows++
		if owner != nil && !slices.Contains(v.Owners, *owner) {
			v.Owners = append(v.Owners, *owner)
		}
		if open {
			deal.DealID = dealID.String()
			v.OpenDeals = append(v.OpenDeals, deal)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("forecasting: reading the evaluation's deals: %w", err)
	}
	for i := range vintages {
		var err error
		if vintages[i].TypedWeighted, vintages[i].Sufficiency, err = summarizeOpenDeals(vintages[i].OpenDeals); err != nil {
			return err
		}
	}
	return nil
}
