// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The stage gate: a stage whose required legal_reviewed criterion is not met
// cannot be passed.
//
// The one exit criterion that blocks a move. Every other kind informs the
// rep and leaves the decision to them. This one is a decision legal made.
//
// The gate reads the review, not the evidence ledger: an approval holds for
// the deal on the terms legal read, wherever the gated stages stand.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// LegalReviewRequiredError refuses a move past a stage that waits for legal.
// It names where the review stands, so the screen can say what to do next.
type LegalReviewRequiredError struct {
	// ReviewStatus is the newest review's status, empty when none was asked for.
	ReviewStatus LegalReviewStatus
	// TermsChanged marks an approval given on an amount or offer the deal no
	// longer carries.
	TermsChanged bool
}

func (e *LegalReviewRequiredError) Error() string {
	switch {
	case e.TermsChanged:
		return "the deal's amount or offer changed since legal approved it: request the review again"
	case e.ReviewStatus == ReviewRequested:
		return "this stage needs legal's approval, and the review is still waiting for an answer"
	case e.ReviewStatus == ReviewChangesNeeded:
		return "this stage needs legal's approval, and legal asked for changes: make them and request the review again"
	default:
		return "this stage needs legal's approval before the deal can move on: request a legal review"
	}
}

// FieldFault names the target the caller asked for.
func (e *LegalReviewRequiredError) FieldFault() (field, code, message string) {
	return progressionToKey, "legal_review_required", e.Error()
}

// ensureLegalReviewed refuses a move onto a stage past a required
// legal_reviewed stage while the deal lacks an approval of its current terms.
// Direction does not matter: a lost or won deal reopened past the gate passes
// it too. Gates behind the deal count, so a gate an admin adds, requires or
// moves behind a deal still holds it. Only a move to lost, or onto or before
// the gated stage, passes.
//
// The caller holds the deal row lock. Every write that can change the answer
// takes it first: a request, a decision, a withdrawal, a deal edit and an
// offer edit (lockOfferDeal).
func ensureLegalReviewed(
	ctx context.Context, tx pgx.Tx, dealID ids.DealID, fromStageID, toStageID ids.StageID,
) error {
	on, err := legalReviewOnForDecision(ctx, tx)
	if err != nil || !on {
		return err
	}
	var moving, gated bool
	err = tx.QueryRow(ctx, `
		SELECT t.semantic <> 'lost' AND t.id <> f.id,
		       EXISTS (SELECT 1 FROM stage s JOIN stage_exit_criterion c ON c.stage_id = s.id
		                WHERE s.pipeline_id = t.pipeline_id AND s.archived_at IS NULL
		                  AND s.position < t.position
		                  AND c.kind = $3 AND c.required AND c.archived_at IS NULL)
		  FROM stage f, stage t
		 WHERE f.id = $1 AND t.id = $2`,
		fromStageID, toStageID, string(CriterionLegalReviewed)).Scan(&moving, &gated)
	if err != nil {
		return fmt.Errorf("read whether the move passes a stage that waits for legal: %w", err)
	}
	if !moving || !gated {
		return nil
	}
	newest, err := newestReview(ctx, tx, dealID)
	if err != nil {
		return err
	}
	if newest.status != ReviewApproved {
		return &LegalReviewRequiredError{ReviewStatus: newest.status}
	}
	terms, err := readReviewTerms(ctx, tx, dealID)
	if err != nil {
		return err
	}
	now, err := terms.fingerprint()
	if err != nil {
		return err
	}
	if now != newest.terms {
		return &LegalReviewRequiredError{ReviewStatus: newest.status, TermsChanged: true}
	}
	return nil
}

// legalReviewOnForDecision reads the switch for a caller about to act on it.
// The shared advisory lock makes settings.Set wait for this transaction. It
// also waits for a Set in flight, so a decision never rests on a stale value.
func legalReviewOnForDecision(ctx context.Context, tx pgx.Tx) (bool, error) {
	if err := settings.ShareLockForDecision(ctx, tx, LegalReviewEnabled.Key()); err != nil {
		return false, err
	}
	return settings.ApplyTx(ctx, tx, LegalReviewEnabled)
}

type reviewStanding struct {
	status LegalReviewStatus
	terms  string
}

// newestReview answers the deal's latest review and the terms it was asked
// on; an empty status when none was ever asked for.
func newestReview(ctx context.Context, tx pgx.Tx, dealID ids.DealID) (reviewStanding, error) {
	var out reviewStanding
	var status string
	err := tx.QueryRow(ctx, `
		SELECT status, COALESCE(packet ->> 'terms', '') FROM legal_review WHERE deal_id = $1
		 ORDER BY created_at DESC, id DESC LIMIT 1`, dealID).Scan(&status, &out.terms)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, fmt.Errorf("read the deal's newest legal review: %w", err)
	}
	out.status = LegalReviewStatus(status)
	return out, nil
}

// LegalGateOnCreateError refuses a deal born on a stage past one that waits
// for legal. It would be beyond the gated stage without having passed it.
type LegalGateOnCreateError struct{}

func (e *LegalGateOnCreateError) Error() string {
	return "this stage comes after one that needs legal's approval: create the deal on that stage or before it, and request a legal review"
}

// FieldFault names the stage the caller chose.
func (e *LegalGateOnCreateError) FieldFault() (field, code, message string) {
	return filterStageID, "legal_review_required", e.Error()
}

// refuseBirthPastLegalGate refuses a new deal on a stage that lies after a
// live stage of its pipeline with a required legal_reviewed criterion.
func refuseBirthPastLegalGate(ctx context.Context, tx pgx.Tx, stageID ids.StageID) error {
	on, err := legalReviewOnForDecision(ctx, tx)
	if err != nil || !on {
		return err
	}
	var past bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM stage t JOIN stage s ON s.pipeline_id = t.pipeline_id
		           JOIN stage_exit_criterion c ON c.stage_id = s.id
		   WHERE t.id = $1 AND s.archived_at IS NULL AND s.position < t.position
		     AND c.kind = $2 AND c.required AND c.archived_at IS NULL)`,
		stageID, string(CriterionLegalReviewed)).Scan(&past)
	if err != nil {
		return fmt.Errorf("read whether the stage lies past one that waits for legal: %w", err)
	}
	if past {
		return &LegalGateOnCreateError{}
	}
	return nil
}

// notFoundOr answers ErrNotFound for a row that is not there, and wraps any
// other failure with what was being read.
func notFoundOr(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	return fmt.Errorf("%s: %w", what, err)
}

// refuseAMoveTheGatesDoNotAdmit asks the two gates on a stage move: the
// progression gate, and legal's.
func refuseAMoveTheGatesDoNotAdmit(
	ctx context.Context, tx pgx.Tx, id ids.DealID, current crmcontracts.Deal, to ids.StageID,
) error {
	if err := refuseAMoveTheGateDidNotAdmit(ctx, tx, current, to); err != nil {
		return err
	}
	// The deal row, after the target stage the move already holds, the order
	// a stage removal relies on. The legal gate decides on the deal's terms
	// and reviews, and every writer of those takes this lock first.
	if _, err := storekit.LockRow(ctx, tx, dealTable, id.UUID, storekit.LiveOnly); err != nil {
		return err
	}
	return ensureLegalReviewed(ctx, tx, id, ids.StageID{UUID: ids.UUID(*current.StageId)}, to)
}

// A pipeline admin may reshape stages, and ops holds pipeline.update for the
// wiring it runs. Weakening a legal gate is the switch's decision under
// another name: un-require it, archive it or its stage, or move a gated
// pipeline's stages. So it takes the admin role, as switching it off does.
func requireAdminToWeakenLegalGate(ctx context.Context) error {
	if err := auth.RequireAdmin(ctx); err != nil {
		return fmt.Errorf("only an admin weakens a legal review gate: %w", err)
	}
	return nil
}

// stageCarriesLegalGate answers whether a stage has a live, required
// legal_reviewed criterion.
func stageCarriesLegalGate(ctx context.Context, tx pgx.Tx, stageID ids.UUID) (bool, error) {
	var gated bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM stage_exit_criterion
		                WHERE stage_id = $1 AND kind = $2 AND required AND archived_at IS NULL)`,
		stageID, string(CriterionLegalReviewed)).Scan(&gated)
	if err != nil {
		return false, fmt.Errorf("read whether the stage waits for legal: %w", err)
	}
	return gated, nil
}

// pipelineCarriesLegalGate answers whether any live stage of a pipeline has
// a live, required legal_reviewed criterion.
func pipelineCarriesLegalGate(ctx context.Context, tx pgx.Tx, pipelineID ids.PipelineID) (bool, error) {
	var gated bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM stage s JOIN stage_exit_criterion c ON c.stage_id = s.id
		                WHERE s.pipeline_id = $1 AND s.archived_at IS NULL
		                  AND c.kind = $2 AND c.required AND c.archived_at IS NULL)`,
		pipelineID, string(CriterionLegalReviewed)).Scan(&gated)
	if err != nil {
		return false, fmt.Errorf("read whether the pipeline waits for legal: %w", err)
	}
	return gated, nil
}

// refuseMovingAGatedLadder takes the admin role for a stage move in a
// pipeline that waits for legal: a move changes which deals lie past a gate.
func refuseMovingAGatedLadder(ctx context.Context, tx pgx.Tx, pipelineID ids.PipelineID) error {
	gated, err := pipelineCarriesLegalGate(ctx, tx, pipelineID)
	if err != nil || !gated {
		return err
	}
	return requireAdminToWeakenLegalGate(ctx)
}
