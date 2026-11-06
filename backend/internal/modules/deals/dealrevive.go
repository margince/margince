// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Margince reopening a lost deal when Deal Scout finds new buying evidence at
// its company, and the owner taking that back. The reopen is a stage move to
// the pipeline's first open stage. Undo closes the deal again with the reason,
// code and close day it had.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// DealReviveCorrection is the correction a reopened deal records, and
// DealReviveKind the owner's switch for it.
const (
	DealReviveCorrection = "deal_revive"
	DealReviveKind       = "deal_revive"
)

// SuggestionKindRevive is the kind a suggestion takes once Margince reopened
// a lost deal from it.
const SuggestionKindRevive = "revive"

// humanCloseHold is how long a deal a colleague closed as lost stays closed
// whatever arrives. A default: a colleague's close is not overruled at once.
const humanCloseHold = 30 * 24 * time.Hour

// reviveMovedSince refuses an Undo once the deal moved after the reopen.
const reviveMovedSince = "this deal has been moved since Margince reopened it, so there is nothing automatic to put back"

// EvidenceArrival is when the evidence reached Margince: the earliest and the
// newest server time of the records it rests on.
type EvidenceArrival struct {
	Earliest, Latest time.Time
}

// ReviveLostDealInput is one reopen Margince decided on.
type ReviveLostDealInput struct {
	SuggestionID ids.UUID
	DealID       ids.DealID
	// Owner is the deal's owner when the reopen was decided.
	Owner ids.UUID
	// Basis is the one line the owner's receipt shows.
	Basis string
	// SwitchOn asks again, under the deal lock, whether the owner lets
	// Margince reopen deals. The switch is not this module's.
	SwitchOn func(ctx context.Context, tx pgx.Tx) (bool, error)
	// EvidenceHolds locks every cited item and asks whether the whole
	// workspace may still read it. The visibility rule is Deal Scout's.
	EvidenceHolds func(ctx context.Context, tx pgx.Tx, evidence []SuggestionEvidence) (bool, error)
	// Arrival answers when the cited evidence arrived. Which records an item
	// rests on is Deal Scout's.
	Arrival func(ctx context.Context, tx pgx.Tx, evidence []SuggestionEvidence) (EvidenceArrival, bool, error)
	// Record writes the entry on the deal's timeline, in the same transaction.
	Record func(ctx context.Context, tx pgx.Tx, deal ids.DealID, name string) error
	// AsOwner binds the owner's agent as of the reopen's transaction, so a
	// role change or suspension that committed before it stops the reopen.
	AsOwner OwnerAuthorityTx
}

// ReviveLostDeal reopens the lost deal for its owner and reports whether it
// did. A deal that is no longer the company's latest loss, changed owner, or
// whose evidence no longer stands is left alone: false, no error.
func (s *Store) ReviveLostDeal(ctx context.Context, in ReviveLostDealInput) (bool, error) {
	if err := RequireSuggestionDecision(ctx, true); err != nil {
		return false, err
	}
	active, err := s.activeColumns(ctx)
	if err != nil {
		return false, err
	}
	var revived bool
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		ctx, may, err := actAsOwnerNow(ctx, tx, in.AsOwner, func(ctx context.Context) error {
			return RequireSuggestionDecision(ctx, true)
		})
		if err != nil || !may {
			return err
		}
		current, pipeline, ready, err := s.revivableDeal(ctx, tx, in)
		if err != nil || !ready {
			return err
		}
		_, first, err := BirthStageTx(ctx, tx, &pipeline, nil)
		if errors.Is(err, apperrors.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		moved, err := s.advanceOnTx(ctx, tx, in.DealID, AdvanceDealInput{
			ToStageID: first, WonWithoutContractReason: nil,
			auditEvidence: map[string]any{
				basisKey: in.Basis, basisOwnerKey: in.Owner.String(), CorrectionKindEvidenceKey: DealReviveCorrection,
			},
		}, active)
		if err != nil {
			return err
		}
		if _, _, err := s.applyAcceptEffects(ctx, tx, current, in.DealID.UUID); err != nil {
			return err
		}
		if err := recordRevived(ctx, tx, current.ID, in.DealID.UUID); err != nil {
			return err
		}
		if _, err := recordStageCorrection(ctx, tx, in.DealID, moved, DealReviveCorrection); err != nil {
			return err
		}
		revived = true
		return in.Record(ctx, tx, in.DealID, moved.Name)
	})
	return revived, err
}

// revivableDeal locks the suggestion, its evidence, the company and the deal,
// and asks again what the decision rested on. The deal is the company's last
// closed deal and lost, with the same live owner. readLostDeal states the
// rest. The evidence arrived after the close and after the last Undo.
func (s *Store) revivableDeal(ctx context.Context, tx pgx.Tx, in ReviveLostDealInput) (Suggestion, ids.UUID, bool, error) {
	current, err := openSuggestionForDecision(ctx, tx, in.SuggestionID)
	if errors.As(err, new(*SuggestionDecidedError)) || errors.Is(err, apperrors.ErrNotFound) {
		return Suggestion{}, ids.Nil, false, nil
	}
	if err != nil || current.Kind != SuggestionKindOpenDeal {
		return Suggestion{}, ids.Nil, false, err
	}
	// The evidence is held first. The time checks below then read rows that
	// nobody can change before the write.
	evidence := citedEvidence(current)
	if stands, err := s.suggestionStands(ctx, tx, current, in, evidence); err != nil || !stands {
		return Suggestion{}, ids.Nil, false, err
	}
	if _, err := storekit.LockRow(ctx, tx, dealTable, in.DealID.UUID, storekit.LiveOnly); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return Suggestion{}, ids.Nil, false, nil
		}
		return Suggestion{}, ids.Nil, false, err
	}
	lost, err := readLostDeal(ctx, tx, in.DealID, current.CompanyID, in.Owner, s.clock().Add(-humanCloseHold))
	if err != nil || lost.pipeline.IsZero() {
		return Suggestion{}, ids.Nil, false, err
	}
	arrived, ok, err := in.Arrival(ctx, tx, evidence)
	if err != nil || !ok {
		return Suggestion{}, ids.Nil, false, err
	}
	if arrivedTooEarly(arrived, lost) {
		return Suggestion{}, ids.Nil, false, supersedeTx(ctx, tx, current.ID, map[string]any{"arrived_before": "the deal's loss or its reopen's Undo"})
	}
	on, err := in.SwitchOn(ctx, tx)
	return current, lost.pipeline, on, err
}

// suggestionStands holds the cited evidence and asks whether the suggestion
// still rests on it. A change retires the suggestion and answers false.
func (s *Store) suggestionStands(ctx context.Context, tx pgx.Tx, current Suggestion, in ReviveLostDealInput, evidence []SuggestionEvidence) (bool, error) {
	if held, err := in.EvidenceHolds(ctx, tx, evidence); err != nil || !held {
		return false, err
	}
	for _, superseded := range []func(context.Context, pgx.Tx, Suggestion) (bool, error){
		supersedeIfEvidenceChanged, s.supersedeIfBuyerChanged, supersedeIfCompanyTaken,
	} {
		if gone, err := superseded(ctx, tx, current); err != nil || gone {
			return false, err
		}
	}
	return true, nil
}

// arrivedTooEarly asks whether evidence predates the decision to reopen.
// Evidence from before the loss was about the deal that was lost, and evidence
// from before an Undo is what the owner already turned down. Either retires
// the suggestion, so newer evidence can raise its own.
func arrivedTooEarly(arrived EvidenceArrival, lost lostDeal) bool {
	return !arrived.Earliest.After(lost.closedAt) || (lost.undoneAt != nil && !arrived.Latest.After(*lost.undoneAt))
}

// lostDeal is what a reopen reads of the locked deal.
type lostDeal struct {
	pipeline ids.UUID
	closedAt time.Time
	// undoneAt is the last Undo of a reopen of this deal, nil when none.
	undoneAt *time.Time
}

// readLostDeal answers the locked deal when it may be reopened, or a zero
// pipeline when it may not. It must be lost, not on legal hold, and not closed
// by a colleague after heldSince.
func readLostDeal(ctx context.Context, tx pgx.Tx, deal ids.DealID, company, owner ids.UUID, heldSince time.Time) (lostDeal, error) {
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return lostDeal{}, err
	}
	var out lostDeal
	var latest bool
	err := tx.QueryRow(ctx, `
		SELECT d.pipeline_id, d.closed_at,
		       NOT EXISTS (SELECT 1 FROM deal later WHERE later.company_id = d.company_id AND later.id <> d.id
		                     AND later.archived_at IS NULL AND later.status <> 'open' AND later.closed_at > d.closed_at),
		       (SELECT max(c.reversed_at) FROM deal_correction c WHERE c.deal_id = d.id AND c.correction = $4)
		  FROM deal d JOIN app_user u ON u.id = d.owner_id AND `+workforce.LiveMemberSQL("u")+`
		 WHERE d.id = $1 AND d.company_id = $2 AND d.owner_id = $3 AND d.status = 'lost'
		   AND NOT d.legal_hold AND d.closed_at IS NOT NULL
		   AND NOT (d.closed_at > $5 AND EXISTS (SELECT 1 FROM deal_stage_history closing
		         WHERE closing.id = (SELECT h.id FROM deal_stage_history h WHERE h.deal_id = d.id
		                              ORDER BY h.changed_at DESC, h.id DESC LIMIT 1)
		           AND starts_with(closing.changed_by, $6)))
		   FOR SHARE OF u`, deal, company, owner, DealReviveCorrection, heldSince, principal.HumanIDPrefix).Scan(&out.pipeline, &out.closedAt, &latest, &out.undoneAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return lostDeal{}, nil
	}
	if err != nil {
		return lostDeal{}, fmt.Errorf("deals: reading the lost deal to reopen: %w", err)
	}
	if !latest {
		return lostDeal{}, nil
	}
	return out, nil
}

// suggestionKindKey is the kind in a suggestion's audit image.
const suggestionKindKey = "kind"

// recordRevived settles the suggestion as the reopen of the deal. Its kind
// says so, so no reader takes it for the suggestion the deal was opened from.
func recordRevived(ctx context.Context, tx pgx.Tx, suggestion, deal ids.UUID) error {
	actor, err := storekit.Actor(ctx)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE deal_suggestion SET state = $2, kind = $3, accepted_deal_id = $4, decided_by = $5, decided_at = now()
		 WHERE id = $1 AND state = 'open'`,
		suggestion, SuggestionAccepted, SuggestionKindRevive, deal, storekit.UUIDOrNil(actor.UserID))
	if err != nil {
		return fmt.Errorf("deals: recording the reopen on the suggestion: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &SuggestionDecidedError{State: "decided"}
	}
	auditID, err := storekit.Audit(ctx, tx, "update", suggestionEntity, suggestion,
		map[string]any{columnState: SuggestionOpen, suggestionKindKey: SuggestionKindOpenDeal},
		map[string]any{columnState: SuggestionAccepted, suggestionKindKey: SuggestionKindRevive, "accepted_deal_id": deal})
	if err != nil {
		return fmt.Errorf("deals: auditing the reopen on the suggestion: %w", err)
	}
	if err := storekit.EmitPipelinePayload(ctx, tx, auditID, crmcontracts.InternalEventDealSuggestionAccepted{}); err != nil {
		return fmt.Errorf("deals: emitting the reopen on the suggestion: %w", err)
	}
	return nil
}

// revertDealRevive closes the reopened deal as lost again, in the caller's
// transaction, with the note, code and close day the reopen took off it. A
// code an admin retired since is left out when a note still says why. Work
// the team added since needs a confirmation naming that work, as the Undo of
// an opened deal does.
func (s *Store) revertDealRevive(
	ctx context.Context, tx pgx.Tx, c DealCorrection, ifVersion *int64, evidence map[string]any,
	active []fieldcatalog.Column, confirmedFor string,
) error {
	if err := storekit.EnsureIfMatch(ctx, tx, dealTable, c.DealID.UUID, ifVersion); err != nil {
		return err
	}
	lostStage, err := stageMoveToReverse(ctx, tx, c, reviveMovedSince)
	if err != nil {
		return err
	}
	work, err := teamWorkSince(ctx, tx, c)
	if err != nil {
		return err
	}
	if work.Notes+work.Tasks+work.Edits > 0 && work.Work != confirmedFor {
		return &work
	}
	loss, err := priorLoss(ctx, tx, c)
	if err != nil {
		return err
	}
	history := *c.StageHistoryID
	closed, err := s.advanceOnTx(ctx, tx, c.DealID, AdvanceDealInput{
		ToStageID: lostStage, ReversalOf: &history, auditEvidence: evidence,
		LostReason: loss.note, LostReasonCode: loss.code, closedAt: loss.closedAt,
		// The deal goes back to the lost stage it left, so the move claims no win.
		WonWithoutContractReason: nil,
	}, active)
	if err != nil {
		return err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	stamped, err := markReversed(ctx, tx, c.ID, closed.AuditLogID, by)
	if err != nil {
		return err
	}
	if !stamped {
		return &CorrectionReversalError{Reason: alreadyTakenBack}
	}
	return nil
}

// reopenedLoss is the loss a reopen took off the deal.
type reopenedLoss struct {
	note, code *string
	closedAt   *time.Time
}

// priorLoss reads the loss from the reopen's own audit row.
func priorLoss(ctx context.Context, tx pgx.Tx, c DealCorrection) (reopenedLoss, error) {
	before, err := priorImage(ctx, tx, c)
	if err != nil {
		return reopenedLoss{}, err
	}
	var out reopenedLoss
	if err := decodeOptional(before["lost_reason"], &out.note); err != nil {
		return reopenedLoss{}, err
	}
	if err := decodeOptional(before[lossReasonCodeField], &out.code); err != nil {
		return reopenedLoss{}, err
	}
	if err := decodeOptional(before["closed_at"], &out.closedAt); err != nil {
		return reopenedLoss{}, err
	}
	if out.code == nil || out.note == nil {
		return out, nil
	}
	var choosable bool
	err = tx.QueryRow(ctx, `SELECT active FROM deal_loss_reason WHERE key = $1`, *out.code).Scan(&choosable)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !choosable) {
		out.code = nil
		return out, nil
	}
	if err != nil {
		return reopenedLoss{}, fmt.Errorf("deals: reading the loss reason to put back: %w", err)
	}
	return out, nil
}
