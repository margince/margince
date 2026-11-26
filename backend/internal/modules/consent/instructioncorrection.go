// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// InstructionWordingCorrector keeps decision checks in the message owner's transaction.
type InstructionWordingCorrector struct{ reviewID ids.UUID }

// WordingCorrectorForReview binds the correction authority to a named review.
func WordingCorrectorForReview(reviewID ids.UUID) InstructionWordingCorrector {
	return InstructionWordingCorrector{reviewID: reviewID}
}

type correctionInstruction struct {
	ID, IntentID                        ids.UUID
	Approved                            []byte
	Scope, Reason, Explanation, Warning string
}

func (c InstructionWordingCorrector) instruction(ctx context.Context, tx pgx.Tx, intent ids.UUID, lock bool) (correctionInstruction, error) {
	if err := requireAHumanAtTheKeyboard(ctx); err != nil {
		return correctionInstruction{}, err
	}
	if err := auth.Require(ctx, entityCommunicationException, principal.ActionCreate); err != nil {
		return correctionInstruction{}, err
	}
	query := `SELECT i.id, r.delivery_intent_id, coalesce(i.corrected_wording, i.acknowledged_wording),
		coalesce(i.acknowledged_scope, ''), i.reason_code, i.explanation, i.warning_version
		FROM communication_instruction i JOIN communication_review r ON r.id=i.review_id
		WHERE r.id=@review AND (@intent::uuid IS NULL OR r.delivery_intent_id=@intent)
		AND r.delivery_intent_id IS NOT NULL AND r.resolved_at IS NULL
		AND i.status='directed' AND i.directed_by=@actor`
	if lock {
		query += ` FOR UPDATE OF i`
	}
	var out correctionInstruction
	err := tx.QueryRow(ctx, query, pgx.StrictNamedArgs{
		"review": c.reviewID, "intent": zeroUUIDAsNull(intent), "actor": initiatingSeat(ctx),
	}).Scan(&out.ID, &out.IntentID, &out.Approved, &out.Scope, &out.Reason, &out.Explanation, &out.Warning)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, apperrors.ErrNotFound
	}
	if err != nil {
		return out, fmt.Errorf("consent: reading the original wording approval: %w", err)
	}
	return out, nil
}

// AuthorizeWordingCorrectionTx checks authority before private message content is read.
func (c InstructionWordingCorrector) AuthorizeWordingCorrectionTx(ctx context.Context, tx pgx.Tx, intent ids.UUID) error {
	_, err := c.instruction(ctx, tx, intent, false)
	return err
}

// RecordWordingCorrectionTx keeps approval evidence atomic with the message owner's edit.
func (c InstructionWordingCorrector) RecordWordingCorrectionTx(
	ctx context.Context, tx pgx.Tx, intent ids.UUID, in commsauthz.WordingCorrection,
) (commsauthz.WordingCorrectionReceipt, error) {
	var out commsauthz.WordingCorrectionReceipt
	if !in.MeaningUnchanged {
		return out, fmt.Errorf("confirm that meaning is unchanged: %w", apperrors.ErrInvalidArgument)
	}
	inst, err := c.instruction(ctx, tx, intent, true)
	if err != nil {
		return out, err
	}
	message, err := instructionMessageTx(ctx, tx, intent)
	if err != nil {
		return out, err
	}
	before := message.Wording
	if !message.matchesApproval(inst) {
		return out, &InstructionStaleError{Why: "the approved message or its scope changed; approve this message again"}
	}
	beforeText := hex.EncodeToString(before[:])
	if in.ExpectedWording != beforeText || message.Version != in.Version {
		return out, apperrors.ErrVersionSkew
	}
	after := SendingDigest(in.Subject, in.Body, in.HTMLBody)
	if before == after {
		return out, fmt.Errorf("the wording has not changed: %w", apperrors.ErrInvalidArgument)
	}
	afterText := hex.EncodeToString(after[:])
	tag, err := tx.Exec(ctx, `UPDATE communication_instruction SET corrected_wording=@corrected_digest WHERE id=@id AND status='directed'`,
		pgx.StrictNamedArgs{"corrected_digest": after[:], "id": inst.ID})
	if err != nil {
		return out, fmt.Errorf("consent: retaining the decision for corrected wording: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return out, apperrors.ErrVersionSkew
	}
	audit, err := storekit.AuditWithEvidence(ctx, tx, "update", "communication_instruction", inst.ID,
		map[string]any{"wording_digest": beforeText}, map[string]any{"wording_digest": afterText},
		map[string]any{"correction_kind": "spelling_punctuation", "meaning_unchanged": in.MeaningUnchanged, instructionAuthorityEvidenceKey: entityCommunicationException})
	if err != nil {
		return out, err
	}
	if err := storekit.EmitEvent(ctx, tx, audit, inst.ID, crmcontracts.InternalEventInstructionWordingCorrected{}); err != nil {
		return out, err
	}
	return commsauthz.WordingCorrectionReceipt{InstructionID: inst.ID, BeforeDigest: beforeText, AfterDigest: afterText}, nil
}
