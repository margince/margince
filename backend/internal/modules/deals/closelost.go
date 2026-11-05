// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Margince closing a deal as lost by itself, and its owner taking that back.
//
// The close is a stage move, so one field restore cannot undo it: the move
// writes the stage history and the closed_at. The correction row names the
// history row the close wrote, and Undo reopens the deal through a second move
// that says it reverses that one.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// CloseLostCorrection is the correction a close as lost records.
const CloseLostCorrection = "close_lost"

// DealCloseLostKind is the owner's switch for Margince closing their deals.
const DealCloseLostKind = "deal_close_lost"

// IsCorrectionReceiptKind reports whether a receipt of this kind is a
// correction Margince applied to a deal. A correction carries its own Undo; a
// decided approval does not.
func IsCorrectionReceiptKind(kind string) bool {
	return kind == CloseDateCorrectionKind || kind == DealCloseLostKind
}

// LossReasonWentSilent is the catalog key a close for silence records.
const LossReasonWentSilent = "went_silent"

// LossReasonOther is the catalog key a close for a refusal records: the rules
// cannot tell which reason the customer gave.
const LossReasonOther = "other"

// CloseLostInput is one close as lost Margince decided on.
type CloseLostInput struct {
	DealID ids.DealID
	// Owner is who the deal belonged to when the close was decided. A deal that
	// changed hands since is left alone.
	Owner      ids.UUID
	ReasonCode string
	// Reason is the one line the deal's lost_reason and the owner's receipt show.
	Reason string
	// StillDue asks again, under the deal row lock, whether the deal should
	// close. The evidence it reads is not this module's.
	StillDue func(ctx context.Context, tx pgx.Tx) (bool, error)
	// Record writes the entry on the deal's timeline, in the close's transaction.
	Record func(ctx context.Context, tx pgx.Tx, correction ids.UUID) error
}

// CloseAsLost closes one open deal as lost for its owner, and reports whether
// it did. A deal that closed, moved owner, went on legal hold or stopped being
// due since the decision is not an error: it reports false.
func (s *Store) CloseAsLost(ctx context.Context, in CloseLostInput) (bool, error) {
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return false, err
	}
	active, err := s.activeColumns(ctx)
	if err != nil {
		return false, err
	}
	var closed bool
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		due, err := s.closeStillDue(ctx, tx, in)
		if err != nil || !due {
			return err
		}
		lost, code, err := lostStageAndCode(ctx, tx, in)
		if err != nil || lost.IsZero() {
			return err
		}
		reason := in.Reason
		// The target is a lost stage, so the move claims no win.
		moved, err := s.advanceOnTx(ctx, tx, in.DealID, AdvanceDealInput{
			ToStageID: lost, LostReason: &reason, LostReasonCode: code, WonWithoutContractReason: nil,
			auditEvidence: map[string]any{
				"basis": in.Reason, "basis_owner": in.Owner.String(), CorrectionKindEvidenceKey: CloseLostCorrection,
			},
		}, active)
		if err != nil {
			return err
		}
		correction, err := recordCloseLost(ctx, tx, in.DealID, moved)
		if err != nil {
			return err
		}
		closed = true
		return in.Record(ctx, tx, correction)
	})
	return closed, err
}

// CorrectionKindEvidenceKey names, in a Margince change's audit evidence, which
// correction wrote it.
const CorrectionKindEvidenceKey = "margince_change"

// closeStillDue locks the deal and re-asks every condition the decision rested
// on. The decision and this write are separate transactions.
func (s *Store) closeStillDue(ctx context.Context, tx pgx.Tx, in CloseLostInput) (bool, error) {
	if _, err := storekit.LockRow(ctx, tx, dealTable, in.DealID.UUID, storekit.LiveOnly); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	var status string
	var owner *ids.UUID
	var legalHold bool
	if err := tx.QueryRow(ctx, `SELECT status, owner_id, legal_hold FROM deal WHERE id = $1`, in.DealID).
		Scan(&status, &owner, &legalHold); err != nil {
		return false, fmt.Errorf("deals: reading the deal to close: %w", err)
	}
	if DealStatus(status) != DealOpen || legalHold || owner == nil || *owner != in.Owner {
		return false, nil
	}
	on, err := config.StoredApplyTx(ctx, tx, config.Bound(), MaintenanceWritesEnabled)
	if err != nil || !on {
		return false, err
	}
	return in.StillDue(ctx, tx)
}

// lostStageAndCode is the lost stage of the deal's pipeline and the catalog
// key to record. A pipeline with no lost stage cannot close, and reports a zero
// stage. A key an admin retired is left out, and the note alone says why.
func lostStageAndCode(ctx context.Context, tx pgx.Tx, in CloseLostInput) (ids.StageID, *string, error) {
	var lost ids.StageID
	err := tx.QueryRow(ctx, `
		SELECT st.id FROM deal d JOIN stage st ON st.pipeline_id = d.pipeline_id
		 WHERE d.id = $1 AND st.semantic = $2 AND st.archived_at IS NULL
		 ORDER BY st.position LIMIT 1`, in.DealID, string(SemanticLost)).Scan(&lost)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.StageID{}, nil, nil
	}
	if err != nil {
		return ids.StageID{}, nil, fmt.Errorf("deals: finding the lost stage: %w", err)
	}
	var active bool
	err = tx.QueryRow(ctx, `SELECT active FROM deal_loss_reason WHERE key = $1`, in.ReasonCode).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !active) {
		return lost, nil, nil
	}
	if err != nil {
		return ids.StageID{}, nil, fmt.Errorf("deals: reading the loss reason: %w", err)
	}
	code := in.ReasonCode
	return lost, &code, nil
}

// recordCloseLost writes the correction row for a close, naming every field
// the move's audit row changed and the history row it wrote.
func recordCloseLost(ctx context.Context, tx pgx.Tx, deal ids.DealID, moved AdvancedDeal) (ids.UUID, error) {
	var fields []string
	if err := tx.QueryRow(ctx,
		`SELECT array(SELECT jsonb_object_keys(after) ORDER BY 1) FROM audit_log WHERE id = $1`,
		moved.AuditLogID).Scan(&fields); err != nil {
		return ids.Nil, fmt.Errorf("deals: reading what the close changed: %w", err)
	}
	history := moved.HistoryID
	recorded, err := recordCorrection(ctx, tx, DealCorrection{
		DealID: deal, AuditLogID: moved.AuditLogID, Correction: CloseLostCorrection,
		Fields: fields, StageHistoryID: &history,
	}, nil)
	return recorded.ID, err
}

// closeMovedSince refuses an Undo once the deal has moved after the close:
// reopening it then would undo a colleague's move, not Margince's.
const closeMovedSince = "this deal has been moved since Margince closed it, so there is nothing automatic to put back"

// revertCloseLost reopens a deal Margince closed, in the caller's transaction.
//
// It reopens to the stage the close moved the deal from, with no time window.
// A field a colleague changed after the close stays as they wrote it. Only the
// probability the close cleared is put back.
func (s *Store) revertCloseLost(
	ctx context.Context, tx pgx.Tx, c DealCorrection, ifVersion *int64, evidence map[string]any,
	active []fieldcatalog.Column,
) error {
	if err := storekit.EnsureIfMatch(ctx, tx, dealTable, c.DealID.UUID, ifVersion); err != nil {
		return err
	}
	from, err := closeToReopen(ctx, tx, c)
	if err != nil {
		return err
	}
	history := *c.StageHistoryID
	reopened, err := s.advanceOnTx(ctx, tx, c.DealID, AdvanceDealInput{
		ToStageID: from, IsExplicitUndo: true, ReversalOf: &history, auditEvidence: evidence,
		// The deal reopens from lost, so the move claims no win.
		WonWithoutContractReason: nil,
	}, active)
	if err != nil {
		return err
	}
	if err := s.restoreClosedProbability(ctx, tx, c, evidence); err != nil {
		return err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	stamped, err := markReversed(ctx, tx, c.ID, reopened.AuditLogID, by)
	if err != nil {
		return err
	}
	if !stamped {
		return &CorrectionReversalError{Reason: alreadyTakenBack}
	}
	return nil
}

// closeToReopen is the stage the close moved the deal from, while the close is
// still the deal's newest move.
func closeToReopen(ctx context.Context, tx pgx.Tx, c DealCorrection) (ids.StageID, error) {
	if c.StageHistoryID == nil {
		return ids.StageID{}, &CorrectionReversalError{Reason: closeMovedSince}
	}
	var newest ids.UUID
	var from *ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT latest.id, closing.from_stage_id
		  FROM deal_stage_history closing,
		       LATERAL (SELECT h.id FROM deal_stage_history h WHERE h.deal_id = closing.deal_id
		                 ORDER BY h.changed_at DESC, h.id DESC LIMIT 1) latest
		 WHERE closing.id = $1 AND closing.deal_id = $2`, *c.StageHistoryID, c.DealID).Scan(&newest, &from)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.StageID{}, &CorrectionReversalError{Reason: closeMovedSince}
	}
	if err != nil {
		return ids.StageID{}, fmt.Errorf("deals: reading the close to undo: %w", err)
	}
	if newest != *c.StageHistoryID || from == nil {
		return ids.StageID{}, &CorrectionReversalError{Reason: closeMovedSince}
	}
	return ids.StageID{UUID: *from}, nil
}

// restoreClosedProbability puts back a probability someone set by hand before
// the close. Closing cleared it, and the reopen sets the stage's value.
func (s *Store) restoreClosedProbability(
	ctx context.Context, tx pgx.Tx, c DealCorrection, evidence map[string]any,
) error {
	before, err := priorImage(ctx, tx, c)
	if err != nil {
		return err
	}
	var override *int
	var setBy *string
	if err := decodeOptional(before["win_probability_override"], &override); err != nil {
		return err
	}
	if err := decodeOptional(before["win_probability_set_by"], &setBy); err != nil {
		return err
	}
	if override == nil {
		return nil
	}
	lock, err := storekit.LockRow(ctx, tx, dealTable, c.DealID.UUID, storekit.LiveOnly)
	if err != nil {
		return err
	}
	patch := storekit.NewPatch()
	patch.Set("win_probability_override", nil, *override)
	patch.Set("win_probability_set_by", nil, setBy)
	if err := applyDealPatchLocked(ctx, tx, patch, lock); err != nil {
		return fmt.Errorf("restore the probability the close cleared: %w", err)
	}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", "deal", c.DealID.UUID,
		patch.Before(), patch.After(), evidence)
	if err != nil {
		return fmt.Errorf("audit the restored probability: %w", err)
	}
	return storekit.EmitEvent(ctx, tx, auditID, c.DealID.UUID,
		crmcontracts.PublicEventDealUpdated{ChangedFields: patch.After()})
}

// decodeOptional reads one audit image value; an absent key leaves out nil.
func decodeOptional[T any](raw json.RawMessage, out **T) error {
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode the prior image: %w", err)
	}
	return nil
}
