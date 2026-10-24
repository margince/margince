// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// A share request follows the contact's owner.
//
// contact_share is decided by the owner its payload names, read live when it
// was staged. Once the contact changes hands, or its owner leaves, that member
// may no longer share it, so the request is withdrawn on the writer's own
// transaction. Withdrawn is not declined: the colleague's next create asks the
// new owner.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	contactShareOwnerMoved = "the contact changed owner; the previous owner may no longer share it"
	contactShareOwnerLeft  = "the contact's owner left; their share requests go with them"
)

// FollowContactOwnerInTx withdraws every pending share request on the contact
// that names an owner other than to. to is nil for a contact nobody owns.
func FollowContactOwnerInTx(ctx context.Context, tx pgx.Tx, contactID ids.UUID, to *ids.UUID) (int, error) {
	return withdrawContactShares(ctx, tx, contactShareOwnerMoved, func(arg func(any) int) string {
		return fmt.Sprintf(`target_entity_type = 'contact' AND target_entity_id = $%d
			AND proposed_change->>'owner_id' IS DISTINCT FROM $%d::text`, arg(contactID), arg(nullUUIDText(to)))
	})
}

// WithdrawSharesDecidedByInTx withdraws every pending share request put to
// member, for the handover of a member who left.
func WithdrawSharesDecidedByInTx(ctx context.Context, tx pgx.Tx, member ids.UUID) (int, error) {
	return withdrawContactShares(ctx, tx, contactShareOwnerLeft, func(arg func(any) int) string {
		return fmt.Sprintf(`proposed_change->>'owner_id' = $%d::text`, arg(member.String()))
	})
}

// withdrawContactShares locks the pending share requests which selects, in
// the canonical order, and withdraws each. which places its own arguments.
func withdrawContactShares(ctx context.Context, tx pgx.Tx, reason string, which func(arg func(any) int) string) (int, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	kind := arg(approvalsubject.KindContactShare)
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT id FROM approval
		 WHERE kind = $%d AND status = 'pending' AND %s
		 %s
		 FOR UPDATE`, kind, which(arg), lockOrder), args...)
	if err != nil {
		return 0, fmt.Errorf("lock the share requests following a contact's owner: %w", err)
	}
	pending, err := pgx.CollectRows(rows, pgx.RowTo[ids.ApprovalID])
	if err != nil {
		return 0, fmt.Errorf("lock the share requests following a contact's owner: %w", err)
	}
	withdrawn := 0
	for _, id := range pending {
		done, err := withdrawInTx(ctx, tx, id, reason)
		if err != nil {
			return withdrawn, err
		}
		if done {
			withdrawn++
		}
	}
	return withdrawn, nil
}

func nullUUIDText(id *ids.UUID) *string {
	if id == nil {
		return nil
	}
	text := id.String()
	return &text
}
