// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// HandoffAssignmentTx reads current assignment state through the prospect's scope.
func HandoffAssignmentTx(ctx context.Context, tx pgx.Tx, id ids.UUID) (crmcontracts.HandoffAssignee, error) {
	var result crmcontracts.HandoffAssignee
	if err := auth.Require(ctx, HandoffObject, principal.ActionRead); err != nil {
		return result, err
	}
	if err := readableHandoffSubject(ctx, tx, id); err != nil {
		return result, err
	}
	err := tx.QueryRow(ctx, `SELECT id,version,status,assigned_to FROM sdr_handoff WHERE id=@id`, pgx.StrictNamedArgs{"id": id}).Scan(&result.Id, &result.Version, &result.Status, &result.AssignedTo)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, apperrors.ErrNotFound
	}
	return result, err
}

// HandoffReassignableTx does not widen the prospect's existing write scope.
func HandoffReassignableTx(ctx context.Context, tx pgx.Tx, id ids.UUID) (bool, error) {
	err := authorizeHandoffReassignment(ctx, tx, id)
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
		return false, nil
	}
	return err == nil, err
}

func authorizeHandoffReassignment(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	if err := auth.Require(ctx, HandoffObject, principal.ActionUpdate); err != nil {
		return err
	}
	return ensureHandoffSubjectWritable(ctx, tx, id)
}

// EnsureHandoffRecipientTx locks liveness before composition resolves the receiver's grants.
func EnsureHandoffRecipientTx(ctx context.Context, tx pgx.Tx, receiver ids.UUID) error {
	return requireHandoffAssignee(ctx, tx, &receiver)
}

// ReassignHandoffTx shares the same guarded writer as member handover.
func ReassignHandoffTx(ctx context.Context, tx pgx.Tx, id ids.UUID, version int64, receiver principal.Principal) error {
	change, err := prepareHandoffReassignment(ctx, tx, handoffReassignmentInput{ID: id, Version: version, Receiver: receiver})
	if err != nil {
		return err
	}
	return applyHandoffReassignment(ctx, tx, change)
}
