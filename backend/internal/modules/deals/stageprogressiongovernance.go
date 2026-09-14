// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Recording what an admin has asked of a transition, and what the product has
// had to do about it.
//
// Split from the judgement beside it because the two answer different
// questions and change for different reasons: stageprogressionpolicy.go says
// what a transition MAY do right now, reading four inputs in one transaction;
// this file is the governance surface that writes three of them.
//
// The line between them is load-bearing. Asking is not being allowed — writing
// `auto` here makes no move automatic, because the judgement re-checks the
// thresholds in the transaction that would apply. That is what lets an admin
// turn a transition on before it has earned the bar and have it start applying
// by itself the day the record clears, without anybody coming back.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SetTransitionPolicyInput is an admin's answer for one transition.
type SetTransitionPolicyInput struct {
	TransitionRef
	Mode AutopilotMode
	// The bar, where the admin wants a different one. Nil keeps what the row
	// has, or the column default on a first write — so a caller who only means
	// to turn a transition on does not have to restate four thresholds and
	// cannot silently reset them by omitting one.
	CleanAcceptanceThreshold    *float64
	CorrectionReversalThreshold *float64
	MinReviewed                 *int
	MinObservationDays          *int
	WindowDays                  *int
	UndoWindowHours             *int
	// IfVersion is the row the caller read. Absent on a first write, because
	// there is no row to have read.
	IfVersion *int64
}

// SetTransitionPolicy records what an admin has asked for on one transition.
//
// ASKING IS NOT BEING ALLOWED. Writing 'auto' here does not make a move
// automatic — StageAutopilotModeTx re-checks the thresholds in the transaction
// that would apply it, every time. That separation is what lets an admin turn
// a transition on before it has earned the bar: the setting stands, the moves
// keep going to a contact, and the day the record clears it starts applying
// without anybody having to come back.
//
// It does NOT clear a suspension. A rule the product turned off went off for a
// measured reason, and letting an ordinary save clear it would make the
// safety mechanism a checkbox — ResumeTransitionPolicy is the separate,
// deliberate act.
func (s *Store) SetTransitionPolicy(
	ctx context.Context, in SetTransitionPolicyInput,
) (TransitionPolicy, error) {
	if err := auth.Require(ctx, "pipeline", principal.ActionUpdate); err != nil {
		return TransitionPolicy{}, err
	}
	// AND read, because this answers the whole rule — thresholds, who first
	// enabled it, any suspension and its reason. A caller who may write but
	// not read would otherwise learn from the response what the GET refuses
	// them, using a one-field save as a read.
	//
	// No seeded role grants update without read today. This is a line of
	// defence against a custom role that does, not a fix for one that exists.
	if err := auth.Require(ctx, "pipeline", principal.ActionRead); err != nil {
		return TransitionPolicy{}, err
	}
	if in.Mode != ModePropose && in.Mode != ModeAuto {
		return TransitionPolicy{}, fmt.Errorf(
			"deals: %q is not a stage automation mode", in.Mode)
	}
	var out TransitionPolicy
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		if err := requireTransitionStages(ctx, tx, in.TransitionRef); err != nil {
			return err
		}
		before, err := lockTransitionPolicy(ctx, tx, in.TransitionRef)
		switch {
		case errors.Is(err, apperrors.ErrNotFound):
			before = nil
		case err != nil:
			return err
		}
		if err := checkPolicyVersion(before, in.IfVersion); err != nil {
			return err
		}
		saved, err := upsertTransitionPolicy(ctx, tx, in, policyActor(ctx))
		if err != nil {
			return err
		}
		out = saved
		return auditTransitionPolicy(ctx, tx, before, saved)
	})
	return out, err
}

// requireTransitionStages refuses a rule about stages that are not both in the
// pipeline it names.
//
// Checked here rather than left to the foreign keys, because the keys admit a
// pair from two different pipelines: each stage exists, so both references
// resolve, and the row would sit there describing a move no deal can make.
func requireTransitionStages(ctx context.Context, tx pgx.Tx, in TransitionRef) error {
	var both bool
	if err := tx.QueryRow(ctx, `
		SELECT count(*) = 2
		  FROM stage
		 WHERE id IN ($2, $3) AND pipeline_id = $1 AND archived_at IS NULL`,
		in.PipelineID, in.FromStageID, in.ToStageID).Scan(&both); err != nil {
		return fmt.Errorf("resolve the transition's stages: %w", err)
	}
	if !both {
		return apperrors.ErrNotFound
	}
	if in.FromStageID == in.ToStageID {
		return fmt.Errorf("deals: a transition from a stage to itself is not a move")
	}
	return nil
}

// checkPolicyVersion holds an edit to the row the caller actually read.
//
// A first write carries no version because there was no row to read; an edit
// that carries none is admitted for the same reason the criteria editor admits
// one — the pin is the caller's to offer, and its absence is a caller who did
// not read first rather than a conflict.
func checkPolicyVersion(before *TransitionPolicy, ifVersion *int64) error {
	if ifVersion == nil {
		return nil
	}
	if before == nil {
		// A pin against a row that does not exist. The caller read something
		// that has since been removed, which is the same surprise a stale
		// version is.
		return apperrors.ErrVersionSkew
	}
	if *ifVersion != before.Version {
		return apperrors.ErrVersionSkew
	}
	return nil
}

// upsertTransitionPolicy writes the rule, keeping every threshold the caller
// did not name.
//
// COALESCE on each optional column rather than a read-modify-write. Composing
// an UPDATE from a struct read a moment earlier writes back every column,
// including the ones this caller never mentioned — so a concurrent edit to a
// threshold gets silently reverted by somebody who only meant to move the
// window.
//
// enabled_by/at are stamped only when this write is what turns the transition
// on. Re-saving a rule already on auto keeps the original enabling — who first
// trusted this transition is a different fact from who last adjusted a
// threshold, and overwriting it loses the one an auditor asks for.
func upsertTransitionPolicy(
	ctx context.Context, tx pgx.Tx, in SetTransitionPolicyInput, actor *ids.UUID,
) (TransitionPolicy, error) {
	var enabler *ids.UUID
	if in.Mode == ModeAuto {
		enabler = actor
	}
	var p TransitionPolicy
	err := tx.QueryRow(ctx, `
		INSERT INTO stage_progression_policy (
			pipeline_id, from_stage_id, to_stage_id, mode,
			clean_acceptance_threshold, correction_reversal_threshold,
			min_reviewed, min_observation_days, window_days, undo_window_hours,
			enabled_by, enabled_at)
		VALUES ($1, $2, $3, $4,
			coalesce($5, 0.950), coalesce($6, 0.010),
			coalesce($7, 200), coalesce($8, 28), coalesce($9, 30), coalesce($10, 72),
			-- BOTH conditioned on the mode, not just the timestamp. Passing
			-- the actor unconditionally would insert enabled_by with a NULL
			-- enabled_at on a propose row and fail the enabling_is_timed
			-- CHECK; it does not today only because the caller happens to
			-- pass nil for propose, which makes the SQL right by accident.
			CASE WHEN $4 = 'auto' THEN $11::uuid END,
			CASE WHEN $4 = 'auto' THEN now() END)
		ON CONFLICT (pipeline_id, from_stage_id, to_stage_id) DO UPDATE SET
			mode = EXCLUDED.mode,
			clean_acceptance_threshold =
				coalesce($5, stage_progression_policy.clean_acceptance_threshold),
			correction_reversal_threshold =
				coalesce($6, stage_progression_policy.correction_reversal_threshold),
			min_reviewed = coalesce($7, stage_progression_policy.min_reviewed),
			min_observation_days =
				coalesce($8, stage_progression_policy.min_observation_days),
			window_days = coalesce($9, stage_progression_policy.window_days),
			undo_window_hours =
				coalesce($10, stage_progression_policy.undo_window_hours),
			-- Only the FIRST enabling is recorded. A rule already on auto keeps
			-- who trusted it; one being turned on now names the contact doing it.
			enabled_by = CASE
				WHEN $4 = 'auto' AND stage_progression_policy.enabled_at IS NULL THEN $11
				WHEN $4 = 'auto' THEN stage_progression_policy.enabled_by
				ELSE NULL END,
			enabled_at = CASE
				WHEN $4 = 'auto' AND stage_progression_policy.enabled_at IS NULL THEN now()
				WHEN $4 = 'auto' THEN stage_progression_policy.enabled_at
				ELSE NULL END,
			updated_at = now(),
			version = stage_progression_policy.version + 1
		RETURNING id, pipeline_id, from_stage_id, to_stage_id, mode,
			clean_acceptance_threshold, correction_reversal_threshold,
			min_reviewed, min_observation_days, window_days, undo_window_hours,
			enabled_by, enabled_at, suspended_at, suspended_reason, version`,
		in.PipelineID, in.FromStageID, in.ToStageID, string(in.Mode),
		in.CleanAcceptanceThreshold, in.CorrectionReversalThreshold,
		in.MinReviewed, in.MinObservationDays, in.WindowDays, in.UndoWindowHours,
		enabler).
		Scan(&p.ID, &p.PipelineID, &p.FromStageID, &p.ToStageID, &p.Mode,
			&p.CleanAcceptanceThreshold, &p.CorrectionReversalThreshold,
			&p.MinReviewed, &p.MinObservationDays, &p.WindowDays, &p.UndoWindowHours,
			&p.EnabledBy, &p.EnabledAt, &p.SuspendedAt, &p.SuspendedReason, &p.Version)
	if err != nil {
		return TransitionPolicy{}, fmt.Errorf("save the transition's automation rule: %w", err)
	}
	return p, nil
}

// policyEntity is what the audit trail calls a rule.
const policyEntity = "stage_progression_policy"

// auditTransitionPolicy records the change and announces it.
//
// Announced because turning a transition on is a governance change other
// surfaces care about: a rule going to auto is why a rep's inbox stops
// receiving cards for that move, and a queue that simply went quiet is
// indistinguishable from one that broke.
func auditTransitionPolicy(
	ctx context.Context, tx pgx.Tx, before *TransitionPolicy, after TransitionPolicy,
) error {
	// The verb is spelled at each branch rather than computed into a variable:
	// a runtime verb is one no gate can judge, and the trail's verbs are what a
	// reader filters on.
	var auditID ids.UUID
	var err error
	if before == nil {
		auditID, err = storekit.Audit(ctx, tx, "create", policyEntity,
			after.ID, nil, policyImage(after))
	} else {
		auditID, err = storekit.Audit(ctx, tx, "update", policyEntity,
			after.ID, policyImage(*before), policyImage(after))
	}
	if err != nil {
		return fmt.Errorf("audit the transition's automation rule: %w", err)
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, after.PipelineID.UUID,
		crmcontracts.PublicEventPipelineUpdated{
			ChangedFields: map[string]any{"stage_automation": string(after.Mode)},
		}); err != nil {
		return fmt.Errorf("announce the automation rule: %w", err)
	}
	return nil
}

// policyImage is the audit trail's view of a rule.
func policyImage(p TransitionPolicy) map[string]any {
	return map[string]any{
		"mode":                          string(p.Mode),
		progressionFromKey:              p.FromStageID.String(),
		progressionToKey:                p.ToStageID.String(),
		"clean_acceptance_threshold":    p.CleanAcceptanceThreshold,
		"correction_reversal_threshold": p.CorrectionReversalThreshold,
		"min_reviewed":                  p.MinReviewed,
		"min_observation_days":          p.MinObservationDays,
		"window_days":                   p.WindowDays,
		"undo_window_hours":             p.UndoWindowHours,
	}
}

// policyActor is the contact a first enabling is recorded against.
//
// Nil for a principal with no user behind it — the system, or an agent acting
// on nobody's seat. The column is nullable for exactly that case, and a rule
// enabled by machinery names no contact rather than borrowing one.
func policyActor(ctx context.Context) *ids.UUID {
	p, ok := principal.Actor(ctx)
	if !ok || p.UserID.IsZero() {
		return nil
	}
	id := p.UserID
	return &id
}

// SuspendTransitionPolicy turns a rule off because its record went bad.
//
// The PRODUCT's act, not an admin's: it is called from the outcome path when
// the measured rates cross a ceiling, and from the safety paths when a defect
// makes the transition untrustworthy regardless of volume. An admin turning
// automation off writes mode instead, and the two are kept apart so an
// admin's setting survives a suspension and comes back when it lifts.
//
// Idempotent: a rule already suspended keeps its FIRST reason. The reason that
// matters is the one that stopped it, and a second sweep overwriting it with a
// consequence of the first would lose what an operator needs.
func (s *Store) SuspendTransitionPolicy(
	ctx context.Context, in TransitionRef, reason string,
) error {
	if err := auth.Require(ctx, "pipeline", principal.ActionUpdate); err != nil {
		return err
	}
	return s.Tx(ctx, func(tx pgx.Tx) error {
		return suspendTransitionPolicyTx(ctx, tx, in, reason)
	})
}

// suspendTransitionPolicyTx is the suspension itself, inside a caller's
// transaction.
//
// Split from the exported verb so the measured sweep on the decision path can
// call it in the transaction that records the outcome — the count that trips
// the ceiling and the suspension it justifies have to commit together, or
// there is a window where the rule is off in the numbers and on in the table.
//
// It carries no auth.Require of its own: the two callers reach it having
// already answered that question — the exported verb gates the admin, and the
// decision path is the product acting on its own measurements, where there is
// no principal whose pipeline:update permission is the thing in question.
func suspendTransitionPolicyTx(
	ctx context.Context, tx pgx.Tx, in TransitionRef, reason string,
) error {
	if reason == "" {
		// The column's CHECK refuses this too. Answering here says which field
		// the caller should fill rather than a 23514.
		return errors.New("deals: a suspension names why it happened")
	}
	before, err := lockTransitionPolicy(ctx, tx, in)
	if err != nil {
		return err
	}
	if before.Suspended() {
		return nil
	}
	var after TransitionPolicy
	if err := tx.QueryRow(ctx, `
		UPDATE stage_progression_policy
		   SET suspended_at = now(), suspended_reason = $2,
		       updated_at = now(), version = version + 1
		 WHERE id = $1 AND suspended_at IS NULL
		RETURNING id, pipeline_id, from_stage_id, to_stage_id, mode,
			clean_acceptance_threshold, correction_reversal_threshold,
			min_reviewed, min_observation_days, window_days, undo_window_hours,
			enabled_by, enabled_at, suspended_at, suspended_reason, version`,
		before.ID, reason).
		Scan(&after.ID, &after.PipelineID, &after.FromStageID, &after.ToStageID,
			&after.Mode, &after.CleanAcceptanceThreshold,
			&after.CorrectionReversalThreshold, &after.MinReviewed,
			&after.MinObservationDays, &after.WindowDays, &after.UndoWindowHours,
			&after.EnabledBy, &after.EnabledAt, &after.SuspendedAt,
			&after.SuspendedReason, &after.Version); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Suspended by a concurrent pass between the read and here. Its
			// reason stands, for the same reason the check above lets the
			// first one stand.
			return nil
		}
		return fmt.Errorf("suspend the transition's automation: %w", err)
	}
	return auditPolicySuspension(ctx, tx, *before, after)
}

// ResumeTransitionPolicy clears a suspension, deliberately.
//
// A separate verb from SetTransitionPolicy, so lifting a safety stop is
// something an admin does on purpose rather than a side effect of saving a
// threshold. What it does NOT do is re-enable: the rule comes back to whatever
// mode it was in, and if that is auto the thresholds still have to hold in the
// transaction that would apply — a resumed rule on a record that is still bad
// simply proposes.
func (s *Store) ResumeTransitionPolicy(
	ctx context.Context, in TransitionRef,
) (TransitionPolicy, error) {
	if err := auth.Require(ctx, "pipeline", principal.ActionUpdate); err != nil {
		return TransitionPolicy{}, err
	}
	// Read too, for the reason SetTransitionPolicy states: this answers the
	// whole rule, so it must not disclose more than the GET would.
	if err := auth.Require(ctx, "pipeline", principal.ActionRead); err != nil {
		return TransitionPolicy{}, err
	}
	var out TransitionPolicy
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		before, err := lockTransitionPolicy(ctx, tx, in)
		if err != nil {
			return err
		}
		if !before.Suspended() {
			// Already running. The row it holds IS the answer — resuming a
			// rule nobody suspended is a no-op, not an error.
			out = *before
			return nil
		}
		var after TransitionPolicy
		if err := tx.QueryRow(ctx, `
			UPDATE stage_progression_policy
			   SET suspended_at = NULL, suspended_reason = NULL,
			       updated_at = now(), version = version + 1
			 WHERE id = $1 AND suspended_at IS NOT NULL
			RETURNING id, pipeline_id, from_stage_id, to_stage_id, mode,
				clean_acceptance_threshold, correction_reversal_threshold,
				min_reviewed, min_observation_days, window_days, undo_window_hours,
				enabled_by, enabled_at, suspended_at, suspended_reason, version`,
			before.ID).
			Scan(&after.ID, &after.PipelineID, &after.FromStageID, &after.ToStageID,
				&after.Mode, &after.CleanAcceptanceThreshold,
				&after.CorrectionReversalThreshold, &after.MinReviewed,
				&after.MinObservationDays, &after.WindowDays, &after.UndoWindowHours,
				&after.EnabledBy, &after.EnabledAt, &after.SuspendedAt,
				&after.SuspendedReason, &after.Version); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// Resumed by a concurrent caller between the read and here.
				// Two contacts clearing one suspension is one clearing, and the
				// second is a decline rather than a failure.
				//
				// The row this caller LOCKED is still the honest answer: it
				// says suspended, which is what was true when this
				// transaction read it, and the caller's next read gets the
				// other one's result.
				out = *before
				return nil
			}
			return fmt.Errorf("resume the transition's automation: %w", err)
		}
		out = after
		return auditPolicySuspension(ctx, tx, *before, after)
	})
	if err != nil {
		return TransitionPolicy{}, err
	}
	return out, nil
}

// auditPolicySuspension records a rule going off or coming back.
func auditPolicySuspension(
	ctx context.Context, tx pgx.Tx, before, after TransitionPolicy,
) error {
	auditID, err := storekit.Audit(ctx, tx, "update", policyEntity, after.ID,
		policySuspensionImage(before), policySuspensionImage(after))
	if err != nil {
		return fmt.Errorf("audit the automation suspension: %w", err)
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, after.PipelineID.UUID,
		crmcontracts.PublicEventPipelineUpdated{
			ChangedFields: map[string]any{
				"stage_automation_suspended": after.Suspended(),
			},
		}); err != nil {
		return fmt.Errorf("announce the automation suspension: %w", err)
	}
	return nil
}

func policySuspensionImage(p TransitionPolicy) map[string]any {
	image := map[string]any{"suspended": p.Suspended(), "mode": string(p.Mode)}
	if p.SuspendedReason != nil {
		image["suspended_reason"] = *p.SuspendedReason
	}
	return image
}
