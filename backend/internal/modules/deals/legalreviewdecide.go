// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Legal's answer on a request, and what it settles.
//
// An approval writes its evidence against the deal's legal_reviewed criteria
// in the transaction that records it. No event hop stands between the two, so
// an outage of the bus cannot leave an approved deal blocked at its stage.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// DecideLegalReviewInput is legal's answer.
type DecideLegalReviewInput struct {
	Outcome  LegalReviewStatus
	Comments *string
}

// DecideLegalReview records an outcome on an open request.
//
// Read and update both: update alone would let a seat decide a review it
// cannot open, by guessing its id. Neither the requester nor the deal's
// owner may answer, because a review is a second pair of eyes.
func (s *Store) DecideLegalReview(
	ctx context.Context, id ids.UUID, in DecideLegalReviewInput,
) (crmcontracts.LegalReviewSummary, error) {
	var out crmcontracts.LegalReviewSummary
	decider, err := requireReviewSeat(ctx)
	if err != nil {
		return out, err
	}
	if err := auth.Require(ctx, legalReviewEntity, principal.ActionRead); err != nil {
		return out, err
	}
	if err := auth.Require(ctx, legalReviewEntity, principal.ActionUpdate); err != nil {
		return out, err
	}
	if err := validDecision(in); err != nil {
		return out, err
	}
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		if err := ensureLegalReviewOn(ctx, tx); err != nil {
			return err
		}
		current, lock, err := lockReview(ctx, tx, id)
		if err != nil {
			return err
		}
		if current.status != ReviewRequested {
			return &LegalReviewDecidedError{Status: current.status}
		}
		if err := refuseOwnSide(current, decider); err != nil {
			return err
		}
		decidedAt := s.clock().UTC()
		patch := storekit.NewPatch()
		patch.Set("status", string(current.status), string(in.Outcome))
		patch.Set("decided_by", nil, decider)
		patch.Set("decided_at", nil, decidedAt)
		patch.Set("comments", nil, nonBlank(in.Comments))
		if err := patch.ApplyLocked(ctx, tx, lock); err != nil {
			return fmt.Errorf("record the legal review outcome: %w", err)
		}
		auditID, err := storekit.Audit(ctx, tx, "resolve", legalReviewEntity, id, patch.Before(), patch.After())
		if err != nil {
			return fmt.Errorf("audit the legal review outcome: %w", err)
		}
		if err := storekit.EmitPipelinePayload(ctx, tx, auditID, crmcontracts.InternalEventLegalReviewDecided{
			Outcome: string(in.Outcome),
		}); err != nil {
			return fmt.Errorf("emit legal_review.decided: %w", err)
		}
		if in.Outcome == ReviewApproved {
			if err := s.settleLegalReviewedCriteria(ctx, tx, ids.From[ids.DealKind](current.dealID), id, decidedAt); err != nil {
				return err
			}
		}
		out, err = readReviewSummary(ctx, tx, id)
		return err
	})
	return out, err
}

// refuseOwnSide keeps the deal's own side from answering: neither the
// colleague who asked nor the deal's owner is a second pair of eyes.
func refuseOwnSide(current reviewHead, decider ids.UUID) error {
	if current.requestedBy != nil && *current.requestedBy == decider {
		return &values.ParseError{
			Field: progressionOutcomeKey, Code: "own_request",
			Message: "a legal review is answered by someone other than the colleague who asked for it",
		}
	}
	if current.dealOwner != nil && *current.dealOwner == decider {
		return &values.ParseError{
			Field: progressionOutcomeKey, Code: "own_deal",
			Message: "a legal review is answered by someone other than the deal's owner",
		}
	}
	return nil
}

func validDecision(in DecideLegalReviewInput) error {
	switch in.Outcome {
	case ReviewApproved:
	case ReviewChangesNeeded:
		if nonBlank(in.Comments) == nil {
			return &values.ParseError{
				Field: "comments", Code: codeRequired,
				Message: "say what has to change before legal can approve",
			}
		}
	default:
		return &values.ParseError{
			Field: "outcome", Code: "invalid_enum",
			Message: "outcome is one of approved, changes_needed",
		}
	}
	return boundedText("comments", in.Comments, maxReviewComments)
}

// settleLegalReviewedCriteria marks every live legal_reviewed criterion of
// the deal's pipeline as met, so the stage checklist shows the approval. Legal
// approved the deal, not one step of it. The gate itself reads the review.
func (s *Store) settleLegalReviewedCriteria(
	ctx context.Context, tx pgx.Tx, dealID ids.DealID, reviewID ids.UUID, decidedAt time.Time,
) error {
	rows, err := tx.Query(ctx, `
		SELECT c.id
		  FROM deal d JOIN stage s ON s.pipeline_id = d.pipeline_id
		  JOIN stage_exit_criterion c ON c.stage_id = s.id
		 WHERE d.id = $1 AND c.kind = $2 AND c.archived_at IS NULL AND s.archived_at IS NULL`,
		dealID, string(CriterionLegalReviewed))
	if err != nil {
		return fmt.Errorf("read the stage's legal review criteria: %w", err)
	}
	criteria, err := pgx.CollectRows(rows, pgx.RowTo[ids.ExitCriterionID])
	if err != nil {
		return fmt.Errorf("read the stage's legal review criteria: %w", err)
	}
	for _, criterionID := range criteria {
		in := EvidenceInput{
			DealID: dealID, CriterionID: criterionID,
			SourceType: SourceLegalReview, SourceID: reviewID,
			AuthorSide: AuthorUnknown, Commitment: CommitmentAgreed, Met: true,
			ObservedAt: decidedAt, ExtractedBy: ExtractedByDeterministic,
		}
		evidenceID, inserted, err := insertEvidence(ctx, tx, in)
		if err != nil {
			return err
		}
		if !inserted {
			continue
		}
		written, err := s.readEvidence(ctx, tx, evidenceID)
		if err != nil {
			return err
		}
		if err := announceEvidence(ctx, tx, dealID, evidenceID, evidenceAfter(written)); err != nil {
			return err
		}
	}
	return nil
}

// refuteEarlierApprovals strikes out the approvals already settling this
// deal's criteria. A new request means the deal's side asks again, so legal
// answers again before a gated stage is passed.
func refuteEarlierApprovals(ctx context.Context, tx pgx.Tx, dealID ids.DealID, by ids.UUID) error {
	rows, err := tx.Query(ctx, `
		UPDATE deal_stage_evidence SET refuted_at = now(), refuted_by = $2
		 WHERE deal_id = $1 AND source_type = $3 AND refuted_at IS NULL
		RETURNING id`, dealID, by, SourceLegalReview)
	if err != nil {
		return fmt.Errorf("refute the deal's earlier legal approvals: %w", err)
	}
	refuted, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return fmt.Errorf("refute the deal's earlier legal approvals: %w", err)
	}
	for _, evidenceID := range refuted {
		auditID, err := storekit.Audit(ctx, tx, "update", evidenceEntity, evidenceID,
			map[string]any{"refuted_by": nil}, map[string]any{"refuted_by": by.String()})
		if err != nil {
			return fmt.Errorf("audit the refuted legal approval: %w", err)
		}
		if err := emitEvidenceChanged(ctx, tx, auditID, dealID); err != nil {
			return err
		}
	}
	return nil
}
