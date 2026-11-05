// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func renewClaimDeadline(ctx context.Context, tx pgx.Tx, in ClaimInput, claim crmcontracts.ConversationClaim, by string) (crmcontracts.ConversationClaim, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalSystem || claim.Status != crmcontracts.ConversationClaimStatusOpen || claim.CorrectedAt != nil || claim.NeedsReview || in.DueAt == nil {
		return claim, nil
	}
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	query := fmt.Sprintf(`UPDATE conversation_claim SET due_at = $%[1]d
 WHERE id = $%[2]d AND archived_at IS NULL AND captured_by = $%[3]d AND due_at IS DISTINCT FROM $%[1]d`,
		arg(in.DueAt), arg(claim.Id), arg(by))
	tag, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return claim, fmt.Errorf("renew the claim's deadline: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return claim, nil
	}
	auditID, err := storekit.Audit(ctx, tx, "update", "contact", in.ContactID.UUID,
		map[string]any{claimIDKey: claim.Id.String(), "due_at": claim.DueAt},
		map[string]any{claimIDKey: claim.Id.String(), "due_at": in.DueAt})
	if err != nil {
		return claim, err
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, in.ContactID.UUID,
		crmcontracts.PublicEventConversationClaimChanged{ClaimId: claim.Id, Status: string(claim.Status)}); err != nil {
		return claim, err
	}
	claim.DueAt = in.DueAt
	return claim, nil
}
