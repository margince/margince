// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func disconnectMemberGrantTx(ctx context.Context, tx pgx.Tx, before MemberGrant, reason string) error {
	if before.Status == "disconnected" {
		return nil
	}
	tag, err := tx.Exec(ctx, "UPDATE chat_member_grant SET status='disconnected',consent_version=consent_version+1 WHERE id=@id AND consent_version=@version", pgx.StrictNamedArgs{"id": before.ID, versionKey: before.Version})
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return apperrors.ErrVersionSkew
	}
	after := before
	after.Status, after.Version = "disconnected", before.Version+1
	audit, err := storekit.AuditWithEvidence(ctx, tx, updateKey, "chat_member_grant", before.ID, grantImage(before), grantImage(after), map[string]any{reasonKey: reason})
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, audit, before.ID, crmcontracts.InternalEventChatMemberGrantDisconnected{})
}
