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
	"maps"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// stageMoveCheckActor reads the own domains for the stage-move check. It is
// the principal of the evidence authorship pass, which judges the same thing.
const stageMoveCheckActor = "system:evidence_authorship"

// DealStageMoveKind is the owner's switch for Margince moving their deals.
const DealStageMoveKind = "deal_stage_move"

// VerdictDroppedStageRefused is a move the deal can no longer take. The
// target is no longer a later open stage, or an exit criterion is not
// settled, or the move's own gates refused it.
const VerdictDroppedStageRefused = "dropped:stage_refused"

// VerdictDroppedNoCriteria is a move across a stage with no required exit
// criteria. Margince moves deals only across stages that say what they ask.
const VerdictDroppedNoCriteria = "dropped:no_exit_criteria"

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
	edited, err := humanEditedAfter(ctx, tx, in.DealID, []string{stageIDColumn}, change.NewestCitedAt)
	if err != nil || edited {
		return VerdictDroppedHuman, current, err
	}
	admitted, err := s.stageMoveAdmitted(ctx, tx, in.DealID, current.StageID, change)
	if err != nil || admitted != VerdictApplied {
		return admitted, current, err
	}
	evidence := map[string]any{CorrectionKindEvidenceKey: VerdictStageMoveCorrection, basisOwnerKey: in.Owner.String()}
	maps.Copy(evidence, change.Evidence)
	moved, refused, err := s.advanceInSavepoint(ctx, tx, in.DealID, AdvanceDealInput{
		ToStageID: change.ToStageID, auditEvidence: correctionReasonEvidence(evidence, crmcontracts.ReceiptReasonKindInference),
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
// later open stage of the deal's pipeline. Each stage the deal leaves needs
// exit criteria. Each required one must be settled by the activity cited for
// it. It answers VerdictApplied or the reason to drop the move.
func (s *Store) stageMoveAdmitted(ctx context.Context, tx pgx.Tx, deal ids.DealID, from ids.UUID, change VerdictChange) (string, error) {
	left, ok, err := lockStagesMovedPast(ctx, tx, deal, from, change.ToStageID)
	if err != nil || !ok {
		return VerdictDroppedStageRefused, err
	}
	rows, err := tx.Query(ctx, `
		SELECT c.id, c.stage_id, c.kind FROM stage_exit_criterion c
		 WHERE c.stage_id = ANY($1) AND c.archived_at IS NULL AND c.required
		 ORDER BY c.id`, left)
	if err != nil {
		return "", fmt.Errorf("deals: rechecking the exit criteria a verdict moves past: %w", err)
	}
	type required struct {
		id, stage ids.UUID
		kind      string
	}
	criteria, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (required, error) {
		var r required
		return r, row.Scan(&r.id, &r.stage, &r.kind)
	})
	if err != nil {
		return "", fmt.Errorf("deals: rechecking the exit criteria a verdict moves past: %w", err)
	}
	asking := map[ids.UUID]bool{}
	for _, c := range criteria {
		asking[c.stage] = true
	}
	if len(left) == 0 || len(asking) != len(left) {
		return VerdictDroppedNoCriteria, nil
	}
	for _, c := range criteria {
		activity, ok := citedFor(change.CriteriaMet, c.id)
		if !ok {
			return VerdictDroppedStageRefused, nil
		}
		settled, err := s.activitySettlesCriterion(ctx, tx, deal, criterionAt{stage: c.stage, id: c.id, kind: CriterionKind(c.kind)}, activity)
		if err != nil || !settled {
			return VerdictDroppedStageRefused, err
		}
	}
	return VerdictApplied, nil
}

// lockStagesMovedPast holds the pipeline and the stages from the deal's own
// to the target FOR SHARE. No reorder, semantic change or criterion edit then
// commits before the move does. The pipeline row comes first and the stages
// in id order, as the ladder writers take them. It answers the stages the
// deal leaves, and false when the target is not a later open stage.
func lockStagesMovedPast(ctx context.Context, tx pgx.Tx, deal ids.DealID, from ids.UUID, to ids.StageID) ([]ids.UUID, bool, error) {
	var pipeline ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT p.id FROM deal d JOIN pipeline p ON p.id = d.pipeline_id
		 WHERE d.id = $1 AND p.archived_at IS NULL FOR SHARE OF p`, deal).Scan(&pipeline)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("deals: holding the deal's pipeline: %w", err)
	}
	stages, err := lockStageSpan(ctx, tx, pipeline, from, to)
	if err != nil {
		return nil, false, err
	}
	left, ok := stagesLeft(stages, from, to)
	return left, ok, nil
}

// heldStage is one stage a move holds FOR SHARE.
type heldStage struct {
	id       ids.UUID
	position int
	semantic string
	live     bool
}

// lockStageSpan holds the stages from the deal's own to the target, in id order.
func lockStageSpan(ctx context.Context, tx pgx.Tx, pipeline, from ids.UUID, to ids.StageID) ([]heldStage, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, position, semantic, archived_at IS NULL FROM stage
		 WHERE pipeline_id = $1 AND id = ANY($2)
		    OR (pipeline_id = $1 AND position > (SELECT position FROM stage WHERE id = $3)
		        AND position < (SELECT position FROM stage WHERE id = $4))
		 ORDER BY id FOR SHARE`, pipeline, []ids.UUID{from, to.UUID}, from, to.UUID)
	if err != nil {
		return nil, fmt.Errorf("deals: holding the stages a move passes: %w", err)
	}
	stages, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (heldStage, error) {
		var h heldStage
		return h, row.Scan(&h.id, &h.position, &h.semantic, &h.live)
	})
	if err != nil {
		return nil, fmt.Errorf("deals: holding the stages a move passes: %w", err)
	}
	return stages, nil
}

// stagesLeft answers the live stages the deal leaves, and false when the target
// is not a later open stage.
func stagesLeft(stages []heldStage, from ids.UUID, to ids.StageID) ([]ids.UUID, bool) {
	var fromAt, toAt *heldStage
	for i := range stages {
		switch stages[i].id {
		case from:
			fromAt = &stages[i]
		case to.UUID:
			toAt = &stages[i]
		}
	}
	if fromAt == nil || toAt == nil || !toAt.live || StageSemantic(toAt.semantic) != SemanticOpen ||
		toAt.position <= fromAt.position {
		return nil, false
	}
	var left []ids.UUID
	for _, stage := range stages {
		if stage.live && stage.position >= fromAt.position && stage.position < toAt.position {
			left = append(left, stage.id)
		}
	}
	return left, true
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

// criterionAt is one required criterion of a stage the deal leaves.
type criterionAt struct {
	stage, id ids.UUID
	kind      CriterionKind
}

// activitySettlesCriterion is the criterion ledger's rule for one activity.
// A held event is a meeting MeetingWasHeld accepts. A buyer milestone is mail
// a buyer of the deal wrote (buyerWrote). A custom criterion counts only as
// the ledger records it (ledgerSettles). Every other kind is settled by its
// own record, never by mail.
func (s *Store) activitySettlesCriterion(
	ctx context.Context, tx pgx.Tx, deal ids.DealID, criterion criterionAt, activity ids.UUID,
) (bool, error) {
	linked, err := holdActivityOnDeal(ctx, tx, deal, activity)
	if err != nil || !linked {
		return false, err
	}
	kind := criterion.kind
	switch {
	case kind == CriterionCustom:
		return s.ledgerSettles(ctx, tx, deal, criterion)
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

// ledgerSettles reports whether the criterion ledger counts a custom
// criterion met as DecideStageMove would move on it. That is met, agreed,
// uncontradicted and at the ledger's own confidence.
func (s *Store) ledgerSettles(ctx context.Context, tx pgx.Tx, deal ids.DealID, criterion criterionAt) (bool, error) {
	standing, err := s.readProgressionCriteria(ctx, tx, ids.StageID{UUID: criterion.stage}, deal)
	if err != nil {
		return false, err
	}
	for _, c := range standing {
		if c.CriterionID.UUID != criterion.id {
			continue
		}
		decision := DecideStageMove(StageMoveFacts{
			Criteria: []CriterionFact{c.CriterionFact}, SingleStep: true, SamePipeline: true,
		})
		return decision.Outcome == OutcomePropose, nil
	}
	return false, nil
}

// holdActivityOnDeal reports whether the cited activity is live and linked to
// the deal, and holds both rows until the move commits.
func holdActivityOnDeal(ctx context.Context, tx pgx.Tx, deal ids.DealID, activity ids.UUID) (bool, error) {
	rows, err := tx.Query(ctx, `SELECT a.id FROM activity_link l JOIN activity a ON a.id = l.activity_id
		WHERE l.deal_id = $1 AND a.id = $2 AND a.archived_at IS NULL FOR SHARE OF l, a`, deal, activity)
	if err != nil {
		return false, fmt.Errorf("deals: holding a cited activity: %w", err)
	}
	held, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return false, fmt.Errorf("deals: holding a cited activity: %w", err)
	}
	return len(held) > 0, nil
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
	// The participants are held first, so the side judged below is the one
	// the move commits on.
	if _, err := shareActivityParticipants(ctx, tx, activity); err != nil {
		return false, err
	}
	side, found, err := judgeActivityAuthor(ctx, tx, activity, domains)
	if err != nil || !found || side != AuthorBuyer {
		return false, err
	}
	// The ledger already judged the side, so the mail's direction is asked
	// no further here: this is about who sent it. The relationship the
	// answer rests on is held until the move commits.
	rows, err := tx.Query(ctx, `
		SELECT r.id FROM activity a
		  JOIN activity_participant p ON p.activity_id = a.id AND p.role = 'from'
		  JOIN deal d ON d.id = $1
		  JOIN relationship r ON r.archived_at IS NULL
		   AND ((r.kind = 'employment' AND r.company_id = d.company_id AND `+employment.IsCurrentSQL("r.ended_at")+`)
		        OR (r.kind = 'deal_stakeholder' AND r.deal_id = d.id))
		 WHERE a.id = $2 AND a.kind = 'email'
		   AND (r.contact_id = p.contact_id
		        OR EXISTS (SELECT 1 FROM contact_email ce
		                    WHERE ce.contact_id = r.contact_id AND lower(ce.email) = lower(p.address)))
		   FOR SHARE OF r`, deal, activity)
	if err != nil {
		return false, fmt.Errorf("deals: reading who wrote a cited mail: %w", err)
	}
	held, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return false, fmt.Errorf("deals: reading who wrote a cited mail: %w", err)
	}
	return len(held) > 0, nil
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
		forward, checkErr := movedForwardToOpen(ctx, savepoint, moved)
		if checkErr != nil || forward {
			if checkErr != nil {
				return AdvancedDeal{}, false, errors.Join(checkErr, savepoint.Rollback(ctx))
			}
			return moved, false, savepoint.Commit(ctx)
		}
		return AdvancedDeal{}, true, savepoint.Rollback(ctx)
	}
	if rollbackErr := savepoint.Rollback(ctx); rollbackErr != nil {
		return AdvancedDeal{}, false, fmt.Errorf("deals: rolling back a refused stage move: %w", rollbackErr)
	}
	if refusedHandOn(err) || errors.Is(err, apperrors.ErrInvalidArgument) {
		return AdvancedDeal{}, true, nil
	}
	return AdvancedDeal{}, false, err
}

// movedForwardToOpen asks the history row the move wrote whether it went
// forward to an open stage, as the move itself recorded it.
func movedForwardToOpen(ctx context.Context, tx pgx.Tx, moved AdvancedDeal) (bool, error) {
	var forward bool
	err := tx.QueryRow(ctx, `
		SELECT coalesce(semantic_at_change = $2 AND to_position_at_change > from_position_at_change, false)
		  FROM deal_stage_history WHERE id = $1`, moved.HistoryID, string(SemanticOpen)).Scan(&forward)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("deals: reading the stage move just written: %w", err)
	}
	return forward, nil
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
