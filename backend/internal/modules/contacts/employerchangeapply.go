// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ApplyEmployerChangeTx writes an accepted employer change as the decider. The
// new company becomes the current primary employer. The old employment ends as
// former, dated by the mail when that date is after its start. It answers false
// and writes nothing when the old employment is no longer the current one.
func (s *Store) ApplyEmployerChangeTx(ctx context.Context, tx pgx.Tx, change approvalsubject.EmployerChange) (bool, error) {
	if err := auth.HoldWritableLive(ctx, tx, contactEntity, change.ContactID); err != nil {
		return false, err
	}
	if err := storekit.LockWriteIdentity(ctx, tx, employmentKind, change.ContactID.String()); err != nil {
		return false, err
	}
	if err := requireContactOwner(ctx, tx, change); err != nil {
		return false, err
	}
	var current ids.UUID
	var started *time.Time
	err := tx.QueryRow(ctx, `SELECT r.id, r.started_at FROM relationship r WHERE r.contact_id = $1 AND `+
		employment.LiveCurrentPrimarySQL("r"), change.ContactID).Scan(&current, &started)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && current != change.FromRelationshipID) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("contacts: reading the employment an employer change ends: %w", err)
	}
	if err := auth.EnsureLinkTarget(ctx, tx, companyEntity, change.ToCompanyID); err != nil {
		return false, err
	}
	live, err := liveEmploymentAt(ctx, tx, change.ContactID, change.ToCompanyID)
	if err != nil {
		return false, err
	}
	if err := requireEmployerWrite(ctx, live); err != nil {
		return false, err
	}
	if err := auth.Require(ctx, tableRelationship, principal.ActionUpdate); err != nil {
		return false, err
	}
	if _, _, err := s.employAt(ctx, tx, change.ContactID, change.ToCompanyID, "", employerChangeSource); err != nil {
		return false, err
	}
	return true, s.endEmployment(ctx, tx, change, started)
}

// endEmployment marks the old employment former. The mail's day is the latest
// the old job can have ended, so it is the end date unless it falls before the
// start.
func (s *Store) endEmployment(ctx context.Context, tx pgx.Tx, change approvalsubject.EmployerChange, started *time.Time) error {
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	former, primary := employmentFormer, false
	patch := UpdateRelationshipInput{
		EmploymentStatus: &former, IsCurrentPrimary: &primary,
		Evidence: map[string]any{auditKeySource: employerChangeSource, "activity_id": change.ActivityID},
	}
	if ended, err := time.Parse(time.DateOnly, change.ObservedOn); err == nil && (started == nil || !ended.Before(*started)) {
		patch.EndedAt = &ended
	}
	_, err = s.updateRelationshipInTx(ctx, tx, change.FromRelationshipID, patch, capturedBy)
	return err
}

// employerChangeSource is the source of an employment an accepted employer
// change starts, and the audit evidence on the one it ends.
const employerChangeSource = approvalsubject.KindEmployerChange

// requireContactOwner refuses a decider who no longer owns the contact. An
// owner change withdraws the pending question (approvals.FollowContactOwnerInTx);
// this asks again under the contact lock, for a decision already in flight.
func requireContactOwner(ctx context.Context, tx pgx.Tx, change approvalsubject.EmployerChange) error {
	decider, ok := principal.Actor(ctx)
	if !ok {
		return apperrors.ErrPermissionDenied
	}
	var owner *ids.UUID
	if err := tx.QueryRow(ctx, `SELECT owner_id FROM contact WHERE id = $1`, change.ContactID).Scan(&owner); err != nil {
		return fmt.Errorf("contacts: reading the owner an employer change is put to: %w", err)
	}
	if owner == nil || *owner != decider.UserID || *owner != change.OwnerID {
		return fmt.Errorf("only the contact's owner accepts an employer change: %w", apperrors.ErrPermissionDenied)
	}
	return nil
}
