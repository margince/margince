// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// A pending request follows the owner of the record it names.
//
// contact_share and ownership_transfer name the owner read when they were
// staged. Once the record changes hands, or its owner leaves, that member may
// no longer answer, so the request is withdrawn on the writer's transaction.
// Withdrawn is not declined: the colleague may ask the new owner.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	contactShareOwnerMoved = "the contact changed owner; the previous owner may no longer share it"
	contactShareOwnerLeft  = "the contact's owner left; their share requests go with them"
	transferOwnerMoved     = "the record changed owner; the colleague it names no longer holds it"
)

// FollowContactOwnerInTx withdraws every pending share request on the contact
// that names an owner other than to. to is nil for a contact nobody owns.
func FollowContactOwnerInTx(ctx context.Context, tx pgx.Tx, contactID ids.UUID, to *ids.UUID) (int, error) {
	withdrawn, err := withdrawPending(ctx, tx, approvalsubject.KindContactShare, contactShareOwnerMoved, func(arg func(any) int) string {
		return fmt.Sprintf(`target_entity_type = 'contact' AND target_entity_id = $%d
			AND proposed_change->>'owner_id' IS DISTINCT FROM $%d::text`, arg(contactID), arg(nullUUIDText(to)))
	})
	return len(withdrawn), err
}

// WithdrawSharesDecidedByInTx withdraws every pending share request put to
// member, for the handover of a member who left.
func WithdrawSharesDecidedByInTx(ctx context.Context, tx pgx.Tx, member ids.UUID) (int, error) {
	withdrawn, err := withdrawPending(ctx, tx, approvalsubject.KindContactShare, contactShareOwnerLeft, func(arg func(any) int) string {
		return fmt.Sprintf(`proposed_change->>'owner_id' = $%d::text`, arg(member.String()))
	})
	return len(withdrawn), err
}

// WithdrawnRequest is one pending request a follow withdrew, with the payload
// that says who staged it.
type WithdrawnRequest struct {
	ID             ids.ApprovalID
	ProposedChange json.RawMessage
}

// FollowTransferOwnerInTx withdraws every pending transfer of one record whose
// giver is no longer its owner, and answers which it withdrew. to is nil for a
// record nobody owns.
func FollowTransferOwnerInTx(ctx context.Context, tx pgx.Tx, recordType string, recordID ids.UUID, to *ids.UUID) ([]WithdrawnRequest, error) {
	return withdrawPending(ctx, tx, approvalsubject.KindOwnershipTransfer, transferOwnerMoved, func(arg func(any) int) string {
		return fmt.Sprintf(`target_entity_type = $%d AND target_entity_id = $%d
			AND proposed_change->>'from_id' IS DISTINCT FROM $%d::text`, arg(recordType), arg(recordID), arg(nullUUIDText(to)))
	})
}

// withdrawPending locks the pending requests of kind which selects, in the
// canonical order, and withdraws each. which places its own arguments.
func withdrawPending(ctx context.Context, tx pgx.Tx, kind, reason string, which func(arg func(any) int) string) ([]WithdrawnRequest, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	selects := fmt.Sprintf(`kind = $%d AND status = 'pending' AND %s`, arg(kind), which(arg))
	rows, err := tx.Query(ctx, `
		SELECT id, proposed_change FROM approval
		 WHERE `+selects+`
		 `+lockOrder+`
		 FOR UPDATE`, args...)
	if err != nil {
		return nil, fmt.Errorf("lock the %s requests following a record's owner: %w", kind, err)
	}
	pending, err := pgx.CollectRows(rows, pgx.RowToStructByPos[WithdrawnRequest])
	if err != nil {
		return nil, fmt.Errorf("lock the %s requests following a record's owner: %w", kind, err)
	}
	var withdrawn []WithdrawnRequest
	for _, request := range pending {
		done, err := withdrawInTx(ctx, tx, request.ID, reason)
		if err != nil {
			return withdrawn, err
		}
		if done {
			withdrawn = append(withdrawn, request)
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
