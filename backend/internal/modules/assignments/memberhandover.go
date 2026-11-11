// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assignments

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// OwnedMemberAssignmentsTx exposes responsibility counts without disclosing their records.
func OwnedMemberAssignmentsTx(ctx context.Context, tx pgx.Tx, member ids.UUID) (int, error) {
	snapshot, err := InspectMemberAssignmentsTx(ctx, tx, member, nil)
	return snapshot.Visible, err
}

// TransferMemberAssignmentsTx retains assignments that either party cannot change.
func TransferMemberAssignmentsTx(ctx context.Context, tx pgx.Tx, member ids.UUID, receiver principal.Principal) (int, error) {
	if err := auth.Require(ctx, "user_admin", principal.ActionUpdate); err != nil {
		return 0, err
	}
	if receiver.Type != principal.PrincipalHuman {
		return 0, apperrors.ErrPermissionDenied
	}
	if member == receiver.UserID {
		return 0, apperrors.ErrInvalidArgument
	}
	if err := auth.EnsureHandOverRecipient(ctx, tx, receiver.UserID); err != nil {
		return 0, err
	}
	rows, err := tx.Query(ctx, `SELECT id FROM record_assignment WHERE user_id=@member AND archived_at IS NULL ORDER BY id`,
		pgx.StrictNamedArgs{memberTransferArgument: member})
	if err != nil {
		return 0, err
	}
	candidates, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return 0, err
	}
	moved := 0
	for _, id := range candidates {
		changed, err := transferAssignment(ctx, tx, id, member, receiver)
		if err != nil {
			return 0, err
		}
		if changed {
			moved++
		}
	}
	return moved, nil
}

func transferAssignment(ctx context.Context, tx pgx.Tx, id, member ids.UUID, receiver principal.Principal) (bool, error) {
	deduplicate := false
	for {
		nested, err := tx.Begin(ctx)
		if err != nil {
			return false, fmt.Errorf("assignments: starting the responsibility move: %w", err)
		}
		if deduplicate {
			err = archiveEquivalentAssignment(ctx, nested, id, member, receiver)
		} else {
			err = moveAssignment(ctx, nested, id, member, receiver)
		}
		if err == nil {
			if err := nested.Commit(ctx); err != nil {
				return false, err
			}
			return true, nil
		}
		// A uniqueness race aborts its statement; roll it back before checking
		// the receiver's existing responsibility on the same record.
		if rollbackErr := nested.Rollback(ctx); rollbackErr != nil {
			return false, errors.Join(err, rollbackErr)
		}
		if !deduplicate && errors.Is(err, apperrors.ErrConflict) {
			deduplicate = true
			continue
		}
		if assignmentTransferRefused(err) {
			return false, nil
		}
		return false, err
	}
}

func moveAssignment(ctx context.Context, tx pgx.Tx, id, member ids.UUID, receiver principal.Principal) error {
	before, err := assignmentTransferSource(ctx, tx, id, member, receiver)
	if err != nil {
		return err
	}
	_, err = updateAssignmentTx(ctx, tx, before, UpdateAssignmentInput{SubjectID: &receiver.UserID})
	return err
}

func assignmentTransferSource(ctx context.Context, tx pgx.Tx, id, member ids.UUID, receiver principal.Principal) (assignmentRow, error) {
	before, err := lockAssignment(ctx, tx, id)
	if err != nil {
		return before, err
	}
	if before.SubjectKind != crmcontracts.AssignmentSubjectKindUser || before.SubjectID != member || before.ArchivedAt != nil {
		return before, apperrors.ErrVersionSkew
	}
	allowed, err := assignmentTransferAllowed(ctx, tx, before, receiver)
	if err != nil {
		return before, err
	}
	if !allowed {
		return before, apperrors.ErrPermissionDenied
	}
	return before, nil
}

func archiveEquivalentAssignment(ctx context.Context, tx pgx.Tx, id, member ids.UUID, receiver principal.Principal) error {
	before, err := assignmentTransferSource(ctx, tx, id, member, receiver)
	if err != nil {
		return err
	}
	company, deal, project := parentArms(before.RecordType, before.RecordID)
	var equivalent ids.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM record_assignment
		WHERE company_id IS NOT DISTINCT FROM @company AND deal_id IS NOT DISTINCT FROM @deal
		AND project_id IS NOT DISTINCT FROM @project AND role_id=@role AND user_id=@receiver
		AND team_id IS NULL AND archived_at IS NULL FOR UPDATE`, pgx.StrictNamedArgs{
		"company": company, "deal": deal, "project": project, "role": before.RoleID, "receiver": receiver.UserID,
	}).Scan(&equivalent)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrConflict
	}
	if err != nil {
		return err
	}
	return archiveAssignmentTx(ctx, tx, before, map[string]any{
		"handover_to_assignment_id": equivalent, "handover_to_user_id": receiver.UserID,
	})
}

func assignmentTransferRefused(err error) bool {
	if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) ||
		errors.Is(err, apperrors.ErrVersionSkew) || errors.Is(err, apperrors.ErrConflict) {
		return true
	}
	_, invalid := errors.AsType[*values.ParseError](err)
	return invalid
}
