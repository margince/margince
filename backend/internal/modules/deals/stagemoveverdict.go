// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Margince moving an open deal forward for its owner, from the evidence the
// nightly verdict cites. The move writes the stage history like any other.
// Undo moves the deal back through a second move that names the first.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// DealStageMoveKind is the owner's switch for Margince moving their deals.
const DealStageMoveKind = "deal_stage_move"

// VerdictDroppedStageRefused is a move the deal can no longer take. The
// target is no longer a later open stage, or an exit criterion is not
// settled, or the move's own gates refused it.
const VerdictDroppedStageRefused = "dropped:stage_refused"

// stageMoveMovedSince refuses an Undo once the deal moved after Margince's
// move: moving it back then would undo a colleague's move.
const stageMoveMovedSince = "this deal has been moved since Margince moved it, so there is nothing automatic to put back"

// stageMoveFirst orders the changes so a stage move applies first. The move
// sets the probability to the new stage's, and a probability the same
// verdict decided then lands on top of it.
func stageMoveFirst(changes []VerdictChange) []int {
	order := make([]int, 0, len(changes))
	for i, change := range changes {
		if change.Correction == VerdictStageMoveCorrection {
			order = append(order, i)
		}
	}
	for i, change := range changes {
		if change.Correction != VerdictStageMoveCorrection {
			order = append(order, i)
		}
	}
	return order
}

// applyVerdictStageMove moves the locked deal and answers the deal's values
// after the move, for the changes that follow it.
func (s *Store) applyVerdictStageMove(
	ctx context.Context, tx pgx.Tx, in VerdictInput, current VerdictValues, change VerdictChange,
	active []fieldcatalog.Column,
) (string, VerdictValues, error) {
	allowed, err := in.Allowed(ctx, tx, change.Correction)
	if err != nil || !allowed {
		return VerdictDroppedSwitch, current, err
	}
	installationOn, err := config.StoredApplyTx(ctx, tx, config.Bound(), StageAutopilotEnabled)
	if err != nil || !installationOn {
		return VerdictDroppedSwitch, current, err
	}
	edited, err := humanEditedAfter(ctx, tx, in.DealID, []string{"stage_id"}, change.NewestCitedAt)
	if err != nil || edited {
		return VerdictDroppedHuman, current, err
	}
	admitted, err := s.stageMoveAdmitted(ctx, tx, in.DealID, current.StageID, change)
	if err != nil || !admitted {
		return VerdictDroppedStageRefused, current, err
	}
	evidence := map[string]any{CorrectionKindEvidenceKey: VerdictStageMoveCorrection, "basis_owner": in.Owner.String()}
	for key, value := range change.Evidence {
		evidence[key] = value
	}
	moved, refused, err := s.advanceInSavepoint(ctx, tx, in.DealID, AdvanceDealInput{
		ToStageID: change.ToStageID, auditEvidence: evidence,
		// The target is an open stage, so the move claims no win.
		WonWithoutContractReason: nil,
	}, active)
	if err != nil || refused {
		return VerdictDroppedStageRefused, current, err
	}
	if _, err := recordStageCorrection(ctx, tx, in.DealID, moved, VerdictStageMoveCorrection); err != nil {
		return "", current, err
	}
	after, err := readVerdictValues(ctx, tx, in.DealID)
	return VerdictApplied, after, err
}

// stageMoveAdmitted asks under the row lock whether the target is still a
// later open stage of the deal's pipeline. Each required exit criterion of
// each stage the deal leaves must be settled by the activity cited for it.
func (s *Store) stageMoveAdmitted(ctx context.Context, tx pgx.Tx, deal ids.DealID, from ids.UUID, change VerdictChange) (bool, error) {
	var forward bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM deal d JOIN stage cur ON cur.id = $2 JOIN stage tgt ON tgt.id = $3
		                WHERE d.id = $1 AND tgt.pipeline_id = d.pipeline_id AND cur.pipeline_id = d.pipeline_id
		                  AND tgt.archived_at IS NULL AND tgt.semantic = $4 AND tgt.position > cur.position)`,
		deal, from, change.ToStageID, string(SemanticOpen)).Scan(&forward)
	if err != nil || !forward {
		return false, err
	}
	var required []ids.UUID
	var kinds []string
	err = tx.QueryRow(ctx, `
		SELECT coalesce(array_agg(c.id ORDER BY c.id), '{}'), coalesce(array_agg(c.kind ORDER BY c.id), '{}')
		  FROM stage cur JOIN stage tgt ON tgt.id = $2
		  JOIN stage st ON st.pipeline_id = cur.pipeline_id AND st.archived_at IS NULL
		               AND st.position >= cur.position AND st.position < tgt.position
		  JOIN stage_exit_criterion c ON c.stage_id = st.id AND c.archived_at IS NULL AND c.required
		 WHERE cur.id = $1`, from, change.ToStageID).Scan(&required, &kinds)
	if err != nil {
		return false, fmt.Errorf("deals: rechecking the exit criteria a verdict moves past: %w", err)
	}
	for i, criterion := range required {
		activity, ok := citedFor(change.CriteriaMet, criterion)
		if !ok {
			return false, nil
		}
		settled, err := s.activitySettlesCriterion(ctx, tx, deal, CriterionKind(kinds[i]), activity)
		if err != nil || !settled {
			return false, err
		}
	}
	return true, nil
}

func citedFor(met []CriterionCitation, criterion ids.UUID) (ids.UUID, bool) {
	for _, cite := range met {
		if cite.CriterionID == criterion {
			return cite.ActivityID, true
		}
	}
	return ids.Nil, false
}

// CriterionCitation is the activity a stage move says meets one criterion.
type CriterionCitation struct {
	CriterionID ids.UUID
	ActivityID  ids.UUID
}

// activitySettlesCriterion is the criterion ledger's rule for one activity.
// A held event is a meeting MeetingWasHeld accepts. A buyer milestone is mail
// a buyer of the deal wrote (buyerWrote). A custom criterion rests on the
// cited words. Every other kind is settled by its own record, never by mail.
func (s *Store) activitySettlesCriterion(
	ctx context.Context, tx pgx.Tx, deal ids.DealID, kind CriterionKind, activity ids.UUID,
) (bool, error) {
	linked, err := activityOnDeal(ctx, tx, deal, activity)
	if err != nil || !linked {
		return false, err
	}
	switch {
	case kind == CriterionCustom:
		return true, nil
	case kind == CriterionEventHeld:
		if s.ownDomains == nil {
			return false, nil
		}
		held, found, err := meetingStillProvesHeld(principal.SystemActing(ctx, stageMoveCheckActor), tx, activity, s.ownDomains)
		return found && held, err
	case kind.BuyerMilestone():
		return s.buyerWrote(ctx, tx, deal, activity)
	}
	return false, nil
}

func activityOnDeal(ctx context.Context, tx pgx.Tx, deal ids.DealID, activity ids.UUID) (bool, error) {
	var linked bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM activity_link l JOIN activity a ON a.id = l.activity_id
		WHERE l.deal_id = $1 AND a.id = $2 AND a.archived_at IS NULL)`, deal, activity).Scan(&linked)
	if err != nil {
		return false, fmt.Errorf("deals: reading a cited activity: %w", err)
	}
	return linked, nil
}

// buyerWrote reports whether the activity is inbound mail the ledger judges
// a buyer wrote. Its sender must also be a contact of the deal's company or a
// stakeholder of the deal.
func (s *Store) buyerWrote(ctx context.Context, tx pgx.Tx, deal ids.DealID, activity ids.UUID) (bool, error) {
	if s.ownDomains == nil {
		return false, nil
	}
	domains, err := s.ownDomains.Domains(principal.SystemActing(ctx, stageMoveCheckActor), tx)
	if err != nil {
		return false, err
	}
	side, found, err := judgeActivityAuthor(ctx, tx, activity, domains)
	if err != nil || !found || side != AuthorBuyer {
		return false, err
	}
	var buyer bool
	// The ledger already judged the side, so the mail's direction is asked
	// no further here: this is about who sent it.
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM activity a
		  JOIN activity_participant p ON p.activity_id = a.id AND p.role = 'from'
		  JOIN deal d ON d.id = $1
		  JOIN relationship r ON r.archived_at IS NULL
		   AND ((r.kind = 'employment' AND r.company_id = d.company_id AND `+employment.IsCurrentSQL("r.ended_at")+`)
		        OR (r.kind = 'deal_stakeholder' AND r.deal_id = d.id))
		 WHERE a.id = $2 AND a.kind = 'email'
		   AND (r.contact_id = p.contact_id
		        OR EXISTS (SELECT 1 FROM contact_email ce
		                    WHERE ce.contact_id = r.contact_id AND lower(ce.email) = lower(p.address))))`,
		deal, activity).Scan(&buyer)
	if err != nil {
		return false, fmt.Errorf("deals: reading who wrote a cited mail: %w", err)
	}
	return buyer, nil
}

// advanceInSavepoint moves the deal, and rolls back only the move when one of
// its own gates refuses it. Any other failure fails the caller.
func (s *Store) advanceInSavepoint(
	ctx context.Context, tx pgx.Tx, deal ids.DealID, in AdvanceDealInput, active []fieldcatalog.Column,
) (AdvancedDeal, bool, error) {
	savepoint, err := tx.Begin(ctx)
	if err != nil {
		return AdvancedDeal{}, false, fmt.Errorf("deals: opening a savepoint for a stage move: %w", err)
	}
	moved, err := s.advanceOnTx(ctx, savepoint, deal, in, active)
	if err == nil {
		return moved, false, savepoint.Commit(ctx)
	}
	if rollbackErr := savepoint.Rollback(ctx); rollbackErr != nil {
		return AdvancedDeal{}, false, fmt.Errorf("deals: rolling back a refused stage move: %w", rollbackErr)
	}
	if refusedHandOn(err) || errors.Is(err, apperrors.ErrInvalidArgument) {
		return AdvancedDeal{}, true, nil
	}
	return AdvancedDeal{}, false, err
}

// readVerdictValues reads the fields a verdict may change, under a lock the
// caller holds.
func readVerdictValues(ctx context.Context, tx pgx.Tx, deal ids.DealID) (VerdictValues, error) {
	var out VerdictValues
	err := tx.QueryRow(ctx, `
		SELECT expected_close_date, close_date_provisional, amount_minor, currency::text,
		       win_probability_override, win_probability_set_by, stage_id
		  FROM deal WHERE id = $1`, deal).
		Scan(&out.CloseDate, &out.Provisional, &out.AmountMinor, &out.Currency,
			&out.Probability, &out.ProbabilitySource, &out.StageID)
	if err != nil {
		return out, fmt.Errorf("deals: reading the deal after a stage move: %w", err)
	}
	return out, nil
}
