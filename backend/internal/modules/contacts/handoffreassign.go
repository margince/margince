// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

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
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const (
	handoffVersionArgument = "version"
	tableSDRHandoff        = "sdr_handoff"
	fieldHandoffAssignee   = "assigned_to"
)

type handoffReassignmentInput struct {
	ID         ids.UUID
	Version    int64
	Receiver   principal.Principal
	FromUserID *ids.UUID
}

type handoffReassignment struct {
	id       ids.UUID
	version  int64
	previous *ids.UUID
	receiver ids.UUID
}

func prepareHandoffReassignment(ctx context.Context, tx pgx.Tx, in handoffReassignmentInput) (handoffReassignment, error) {
	var change handoffReassignment
	if err := auth.Require(ctx, HandoffObject, principal.ActionUpdate); err != nil {
		return change, err
	}
	if err := auth.Require(ctx, HandoffObject, principal.ActionRead); err != nil {
		return change, err
	}
	if in.Version < 1 {
		return change, apperrors.ErrInvalidArgument
	}
	if err := ensureHandoffSubjectWritable(ctx, tx, in.ID); err != nil {
		return change, err
	}
	if err := requireHandoffAssignee(ctx, tx, &in.Receiver.UserID); err != nil {
		return change, err
	}
	if err := requireHandoffReceiver(ctx, tx, in.ID, in.Receiver); err != nil {
		return change, err
	}
	change.id, change.receiver = in.ID, in.Receiver.UserID
	var status string
	err := tx.QueryRow(ctx, `SELECT version, status, assigned_to FROM sdr_handoff WHERE id=@id FOR UPDATE`,
		pgx.StrictNamedArgs{"id": in.ID}).Scan(&change.version, &status, &change.previous)
	if errors.Is(err, pgx.ErrNoRows) {
		return change, apperrors.ErrNotFound
	}
	if err != nil {
		return change, fmt.Errorf("contacts: reading the waiting handoff: %w", err)
	}
	if change.version != in.Version || (in.FromUserID != nil && (change.previous == nil || *change.previous != *in.FromUserID)) {
		return change, apperrors.ErrVersionSkew
	}
	if status != HandoffSubmitted {
		return change, fmt.Errorf("only a waiting handoff can be reassigned: %w", apperrors.ErrConflict)
	}
	return change, nil
}

func requireHandoffReceiver(ctx context.Context, tx pgx.Tx, id ids.UUID, receiver principal.Principal) error {
	if receiver.Type != principal.PrincipalHuman ||
		!receiver.Permissions.Allows(HandoffObject, principal.ActionRead) ||
		!receiver.Permissions.Allows(HandoffObject, principal.ActionUpdate) {
		return handoffReceiverFault()
	}

	entity, subject, err := handoffSubject(ctx, tx, id)
	if err != nil {
		return err
	}
	if !receiver.Permissions.Allows(entity, principal.ActionRead) || !receiver.Permissions.Allows(entity, principal.ActionUpdate) {
		return handoffReceiverFault()
	}
	allowed, err := auth.WritableBy(principal.WithActor(ctx, receiver), tx, entity, subject)
	if err != nil {
		return err
	}
	if !allowed {
		return handoffReceiverFault()
	}
	return nil
}

func handoffReceiverFault() error {
	return &values.ParseError{Field: fieldHandoffAssignee, Code: "not_assignable", Message: "choose an active colleague who can work this prospect"}
}

func applyHandoffReassignment(ctx context.Context, tx pgx.Tx, change handoffReassignment) error {
	patch := storekit.NewPatch()
	patch.Set(fieldHandoffAssignee, change.previous, change.receiver)
	if patch.Empty() {
		return nil
	}
	var changedAt time.Time
	if err := tx.QueryRow(ctx, `SELECT CURRENT_TIMESTAMP`).Scan(&changedAt); err != nil {
		return err
	}
	patch.SetStamp("version", change.version+1)
	patch.SetStamp("updated_at", changedAt)
	if err := patch.ApplyGuardedIn(ctx, tx, tableSDRHandoff, change.id, &change.version, storekit.NoArchiveColumn); err != nil {
		return err
	}
	audit, err := storekit.Audit(ctx, tx, "update", tableSDRHandoff, change.id, patch.Before(), patch.After())
	if err != nil {
		return err
	}
	if err := storekit.EmitEvent(ctx, tx, audit, change.id, crmcontracts.InternalEventSdrHandoffUpdated{}); err != nil {
		return err
	}
	return nil
}

// OwnedPendingHandoffsTx exposes member-work counts without disclosing prospects.
func OwnedPendingHandoffsTx(ctx context.Context, tx pgx.Tx, member ids.UUID) (int, error) {
	snapshot, err := InspectMemberHandoffsTx(ctx, tx, member, nil)
	return snapshot.Visible, err
}

// TransferPendingHandoffsTx leaves accepted handoffs and their attribution unchanged.
func TransferPendingHandoffsTx(ctx context.Context, tx pgx.Tx, member ids.UUID, receiver principal.Principal) (int, error) {
	if err := auth.Require(ctx, "user_admin", principal.ActionUpdate); err != nil {
		return 0, err
	}
	if member == receiver.UserID {
		return 0, apperrors.ErrInvalidArgument
	}
	rows, err := tx.Query(ctx, `SELECT id, version FROM sdr_handoff WHERE assigned_to=@member AND status=@waiting ORDER BY id`,
		pgx.StrictNamedArgs{"member": member, "waiting": HandoffSubmitted})
	if err != nil {
		return 0, err
	}
	type candidate struct {
		ID      ids.UUID
		Version int64
	}
	candidates, err := pgx.CollectRows(rows, pgx.RowToStructByPos[candidate])
	if err != nil {
		return 0, err
	}
	moved := 0
	for _, row := range candidates {
		change, err := prepareHandoffReassignment(ctx, tx, handoffReassignmentInput{
			ID: row.ID, Version: row.Version, FromUserID: &member, Receiver: receiver,
		})
		if handoffReassignmentRefused(err) {
			continue
		}
		if err != nil {
			return 0, err
		}
		if err := applyHandoffReassignment(ctx, tx, change); err != nil {
			return 0, err
		}
		moved++
	}
	return moved, nil
}

func handoffReassignmentRefused(err error) bool {
	if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) ||
		errors.Is(err, apperrors.ErrVersionSkew) || errors.Is(err, apperrors.ErrConflict) {
		return true
	}
	fault, ok := errors.AsType[*values.ParseError](err)
	return ok && fault.Field == fieldHandoffAssignee
}
