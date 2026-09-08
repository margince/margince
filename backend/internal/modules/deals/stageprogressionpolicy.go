// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Whether a stage move may apply without asking.
//
// FOUR things have to agree, and this file asks all four in ONE transaction:
// the installation's kill switch, the transition's own rule, the measured
// record behind it, and the absence of a suspension. Asked separately, they
// answer about different moments — and the moment that matters is the one the
// move commits in.
//
// It answers a MODE, never applies anything. The caller that applies is
// compose's, because applying crosses into approvals; what lives here is the
// judgement, so it can be read and tested as a sentence rather than inferred
// from what a sweep did.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// AutopilotMode is what a transition may do right now.
type AutopilotMode string

const (
	// ModePropose is what a transition answers until it has earned otherwise,
	// and what this file returns whenever it is unsure. A card goes up and a
	// person decides.
	ModePropose AutopilotMode = "propose"
	// ModeAuto means the move applies and the receipt says so afterwards.
	ModeAuto AutopilotMode = "auto"
)

// refused is the answer every error path gives.
//
// It names ModePropose explicitly rather than returning a zero AutopilotVerdict.
// The zero value's Mode is "" — safe against a caller asking `== ModeAuto` and
// NOT safe against one asking `== ModePropose`, which would fall through to
// neither branch. A judgement whose safety depends on which way the caller
// spells the question is one waiting for a caller who spells it the other way.
func refused() AutopilotVerdict {
	return AutopilotVerdict{Mode: ModePropose}
}

// AutopilotVerdict is the mode plus WHY, because an admin who turned automation
// on and sees cards still arriving needs to be told which of the four
// conditions is not met — a silent `propose` reads as the feature not working.
type AutopilotVerdict struct {
	Mode AutopilotMode
	// Why names the condition that withheld auto. Empty exactly when the mode
	// is auto.
	Why string
	// Rule is the transition's own row, absent when none is configured.
	Rule *TransitionPolicy
}

// TransitionPolicy is one transition's rule.
type TransitionPolicy struct {
	// ID is the rule's own row, carried so the audit names the record it
	// changed rather than reading it back.
	ID          ids.UUID
	PipelineID  ids.PipelineID
	FromStageID ids.StageID
	ToStageID   ids.StageID
	Mode        AutopilotMode
	// The bar this installation set.
	CleanAcceptanceThreshold    float64
	CorrectionReversalThreshold float64
	MinReviewed                 int
	MinObservationDays          int
	WindowDays                  int
	UndoWindowHours             int
	EnabledBy                   *ids.UUID
	EnabledAt                   *time.Time
	SuspendedAt                 *time.Time
	SuspendedReason             *string
	Version                     int64
}

// Suspended answers whether the product has turned this rule off itself.
func (p TransitionPolicy) Suspended() bool { return p.SuspendedAt != nil }

// UndoWindow is how long a person has to take an automatic move back.
func (p TransitionPolicy) UndoWindow() time.Duration {
	return time.Duration(p.UndoWindowHours) * time.Hour
}

// StageAutopilotModeTx answers what one transition may do, inside the caller's
// transaction.
//
// THE TRANSACTION IS THE POINT. Every input here can change between the
// moment a card was staged and the moment it would apply: an admin flips the
// kill switch, a rejection lands and drops the acceptance rate below the bar,
// the product suspends the rule. Read at staging, this would authorize a move
// on facts that were true hours ago; read here, the answer and the write it
// authorizes commit together or not at all.
func StageAutopilotModeTx(
	ctx context.Context, tx pgx.Tx, in TransitionRef, now time.Time,
) (AutopilotVerdict, error) {
	// The kill switch first, because it is the cheapest question and the one
	// an admin expects to answer everything. Nothing below it is consulted
	// when the installation has said no.
	on, err := settings.ApplyTx(ctx, tx, StageAutopilotEnabled)
	if err != nil {
		return refused(), fmt.Errorf("read whether stage automation is on: %w", err)
	}
	if !on {
		return AutopilotVerdict{
			Mode: ModePropose,
			Why:  "stage automation is switched off for this installation",
		}, nil
	}

	rule, err := readTransitionPolicy(ctx, tx, in)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			// No rule configured. The DEFAULT STATE of every transition in the
			// product, and it means propose — a transition nobody has decided
			// about has not been trusted with anything.
			return AutopilotVerdict{
				Mode: ModePropose,
				Why:  "no automation rule is configured for this transition",
			}, nil
		}
		return refused(), err
	}
	verdict := AutopilotVerdict{Mode: ModePropose, Rule: rule}

	if rule.Suspended() {
		verdict.Why = "automation for this transition is suspended: " + *rule.SuspendedReason
		return verdict, nil
	}
	if rule.Mode != ModeAuto {
		verdict.Why = "this transition is set to propose"
		return verdict, nil
	}

	// The RATES, re-counted here rather than trusted from when the rule was
	// enabled. A rule turned on in March on a good record is not a licence for
	// June: the bar is a claim about the transition's CURRENT behaviour, and
	// re-reading it is what lets a run of rejections stop automation before
	// anybody notices to suspend it.
	rates, err := transitionRatesTx(ctx, tx, *rule, now)
	if err != nil {
		return refused(), err
	}
	if why := rule.withholds(rates); why != "" {
		verdict.Why = why
		return verdict, nil
	}
	return AutopilotVerdict{Mode: ModeAuto, Rule: rule}, nil
}

// TransitionRef names one transition.
type TransitionRef struct {
	PipelineID  ids.PipelineID
	FromStageID ids.StageID
	ToStageID   ids.StageID
}

// withholds answers which threshold is not met, or "" when all are.
//
// ORDERED from the condition a reader can act on soonest. An admin told "not
// enough reviewed yet" waits; one told "acceptance is below the bar" goes and
// looks at what the product is proposing. Telling them the second when the
// first is also true would send them hunting for a quality problem that has
// simply not been measured yet.
func (p TransitionPolicy) withholds(r TransitionRates) string {
	switch {
	case r.Reviewed < p.MinReviewed:
		return fmt.Sprintf(
			"only %d of the %d reviewed proposals this transition needs before it may move deals itself",
			r.Reviewed, p.MinReviewed)
	case r.ObservationDays < p.MinObservationDays:
		return fmt.Sprintf(
			"observed for %d of the %d days this transition needs — a record earned in a few days is not a record",
			r.ObservationDays, p.MinObservationDays)
	case r.CleanAcceptanceRate() < p.CleanAcceptanceThreshold:
		return fmt.Sprintf(
			"people accept this move as proposed %.0f%% of the time, below the %.0f%% this transition asks for",
			r.CleanAcceptanceRate()*100, p.CleanAcceptanceThreshold*100)
	case r.UnsafeRate() > p.CorrectionReversalThreshold:
		return fmt.Sprintf(
			"%.1f%% of these moves were undone or corrected, above the %.1f%% ceiling",
			r.UnsafeRate()*100, p.CorrectionReversalThreshold*100)
	}
	return ""
}

// readTransitionPolicy answers one transition's rule.
func readTransitionPolicy(
	ctx context.Context, tx pgx.Tx, in TransitionRef,
) (*TransitionPolicy, error) {
	return transitionPolicy(ctx, tx, in, "")
}

// lockTransitionPolicy reads the rule and holds it for the rest of the
// transaction.
//
// For a writer that decides FROM what it read: suspend keeps the first reason,
// resume clears one — and both compare the row they read against the row they
// write. Unlocked, two passes read the same unsuspended rule, both decide to
// act, and the second overwrites a reason the first had just recorded.
func lockTransitionPolicy(
	ctx context.Context, tx pgx.Tx, in TransitionRef,
) (*TransitionPolicy, error) {
	return transitionPolicy(ctx, tx, in, " FOR UPDATE")
}

func transitionPolicy(
	ctx context.Context, tx pgx.Tx, in TransitionRef, lock string,
) (*TransitionPolicy, error) {
	var p TransitionPolicy
	err := tx.QueryRow(ctx, `
		SELECT id, pipeline_id, from_stage_id, to_stage_id, mode,
		       clean_acceptance_threshold, correction_reversal_threshold,
		       min_reviewed, min_observation_days, window_days, undo_window_hours,
		       enabled_by, enabled_at, suspended_at, suspended_reason, version
		  FROM stage_progression_policy
		 WHERE pipeline_id = $1 AND from_stage_id = $2 AND to_stage_id = $3`+lock,
		in.PipelineID, in.FromStageID, in.ToStageID).
		Scan(&p.ID, &p.PipelineID, &p.FromStageID, &p.ToStageID, &p.Mode,
			&p.CleanAcceptanceThreshold, &p.CorrectionReversalThreshold,
			&p.MinReviewed, &p.MinObservationDays, &p.WindowDays, &p.UndoWindowHours,
			&p.EnabledBy, &p.EnabledAt, &p.SuspendedAt, &p.SuspendedReason, &p.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read the transition's automation rule: %w", err)
	}
	return &p, nil
}

// transitionRatesTx counts ONE transition's record inside the caller's
// transaction, over that rule's own window.
//
// Through readTransitionRates — the same helper the report calls — so the
// number an admin reads on the settings page comes from the same arithmetic
// that authorizes a move. Counting it again here would spell clean acceptance
// twice, and the spelling that decides anything is the one nobody sees.
func transitionRatesTx(
	ctx context.Context, tx pgx.Tx, rule TransitionPolicy, now time.Time,
) (TransitionRates, error) {
	since := now.Add(-time.Duration(rule.WindowDays) * 24 * time.Hour)
	rows, err := readTransitionRates(ctx, tx, rule.PipelineID, since)
	if err != nil {
		return TransitionRates{}, err
	}
	for _, r := range rows {
		if r.FromStageID == rule.FromStageID && r.ToStageID == rule.ToStageID {
			return r, nil
		}
	}
	// No rows at all for this transition in the window. Not an error: a
	// transition nobody has proposed on has a record of nothing, and every
	// threshold below reads that as "not yet".
	return TransitionRates{
		PipelineID:  rule.PipelineID,
		FromStageID: rule.FromStageID,
		ToStageID:   rule.ToStageID,
	}, nil
}

// ReadTransitionPolicies answers every rule on one pipeline, for the settings
// surface.
func (s *Store) ReadTransitionPolicies(
	ctx context.Context, pipelineID ids.PipelineID,
) ([]TransitionPolicy, error) {
	if err := auth.Require(ctx, "pipeline", principal.ActionRead); err != nil {
		return nil, err
	}
	var out []TransitionPolicy
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		if err := requirePipeline(ctx, tx, pipelineID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			-- Every column QUALIFIED. The stage joins below carry a
			-- pipeline_id of their own, so the bare name is ambiguous and
			-- Postgres refuses the statement outright.
			SELECT p.id, p.pipeline_id, p.from_stage_id, p.to_stage_id, p.mode,
			       p.clean_acceptance_threshold, p.correction_reversal_threshold,
			       p.min_reviewed, p.min_observation_days, p.window_days,
			       p.undo_window_hours, p.enabled_by, p.enabled_at,
			       p.suspended_at, p.suspended_reason, p.version
			  FROM stage_progression_policy p
			  LEFT JOIN stage f ON f.id = p.from_stage_id
			  LEFT JOIN stage t ON t.id = p.to_stage_id
			 WHERE p.pipeline_id = $1
			 ORDER BY f."position", t."position"`, pipelineID)
		if err != nil {
			return fmt.Errorf("read the pipeline's automation rules: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var p TransitionPolicy
			if err := rows.Scan(&p.ID, &p.PipelineID, &p.FromStageID, &p.ToStageID, &p.Mode,
				&p.CleanAcceptanceThreshold, &p.CorrectionReversalThreshold,
				&p.MinReviewed, &p.MinObservationDays, &p.WindowDays, &p.UndoWindowHours,
				&p.EnabledBy, &p.EnabledAt, &p.SuspendedAt, &p.SuspendedReason,
				&p.Version); err != nil {
				return fmt.Errorf("scan an automation rule: %w", err)
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, err
}

// MayAutoApplyStageMove answers whether ONE proposed move may apply without
// asking, resolving the deal's pipeline and the rule in one transaction.
//
// The entry point the auto-applier reaches through its seam. It exists because
// the applier knows an approval and a payload, not a transition: the pipeline
// is the DEAL'S, read here rather than taken from the staged card, so a deal
// moved to another pipeline since the card was raised is judged by the rule
// that governs where it is now.
//
// It answers only. Applying is the caller's, under the caller's authority.
//
// GATED on reading the deal, not on the pipeline. The question is about one
// deal's move, and the caller that asks it is about to write that deal — so a
// principal who may not see the deal must not learn from this whether its
// transition is on automatic. The pipeline rule underneath is read through
// StageAutopilotModeTx, which is reached only after this gate.
func (s *Store) MayAutoApplyStageMove(
	ctx context.Context, dealID ids.DealID, fromStage, toStage ids.StageID,
) (AutopilotVerdict, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return refused(), err
	}
	var out AutopilotVerdict
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		var pipelineID ids.PipelineID
		if err := tx.QueryRow(ctx,
			`SELECT pipeline_id FROM deal WHERE id = $1 AND archived_at IS NULL`,
			dealID).Scan(&pipelineID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// Archived or gone. A move onto a deal that is not there is not
				// a move this may make, and the card stays for a person to
				// close.
				out = refused()
				return nil
			}
			return fmt.Errorf("read the deal's pipeline: %w", err)
		}
		verdict, err := StageAutopilotModeTx(ctx, tx, TransitionRef{
			PipelineID: pipelineID, FromStageID: fromStage, ToStageID: toStage,
		}, s.clock())
		if err != nil {
			return err
		}
		out = verdict
		return nil
	})
	if err != nil {
		return refused(), err
	}
	return out, nil
}
