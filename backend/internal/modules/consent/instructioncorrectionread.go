// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"encoding/hex"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// WordingCorrectionContext binds the editor to the approving sender's observed snapshot.
type WordingCorrectionContext struct {
	InstructionID                           ids.UUID
	Version                                 int64
	Subject, Body, HTMLBody, WordingDigest  string
	ReasonCode, Explanation, WarningVersion string
}

func wordingCorrectionForReviewTx(ctx context.Context, tx pgx.Tx, review ids.UUID) (WordingCorrectionContext, bool, error) {
	var absent WordingCorrectionContext
	actor, present := principal.Actor(ctx)
	if !present || actor.Type != principal.PrincipalHuman ||
		!auth.AllowsWrite(ctx, entityCommunicationException, principal.ActionCreate) ||
		!auth.Allows(ctx, "activity", principal.ActionRead) || !auth.AllowsWrite(ctx, "activity", principal.ActionUpdate) {
		return absent, false, nil
	}
	inst, err := WordingCorrectorForReview(review).instruction(ctx, tx, ids.Nil, false)
	if errors.Is(err, apperrors.ErrNotFound) {
		return absent, false, nil
	}
	if err != nil {
		return absent, false, err
	}
	message, err := instructionMessageTx(ctx, tx, inst.IntentID)
	if err != nil {
		return absent, false, err
	}
	if message.Status != "held" || message.HeldReason != "send_refused" || message.ScheduledBy != initiatingSeat(ctx) || !message.matchesApproval(inst) {
		return absent, false, nil
	}
	return WordingCorrectionContext{
		InstructionID: inst.ID, Version: message.Version,
		Subject: message.Subject, Body: message.Body, HTMLBody: message.HTMLBody,
		WordingDigest: hex.EncodeToString(message.Wording[:]), ReasonCode: inst.Reason,
		Explanation: inst.Explanation, WarningVersion: inst.Warning,
	}, true, nil
}
