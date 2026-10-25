// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The way back from withdrawing one of a seat's own addresses.
//
// A withdrawal moves the whole row aside for ten minutes, keyed by its own id.
// The restore then puts back the id, source and created_at it had. The value is
// a private address, so nothing keeps it past the undo.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// withdrawnIdentityKept is how long a withdrawn identity can be put back.
const withdrawnIdentityKept = 10 * time.Minute

// IdentityRestoreError is a restore the identity's later history refuses. It
// answers 409.
type IdentityRestoreError struct{ code, message string }

// The refusals a restore answers besides not-found.
var (
	ErrRemovalMovedOn = &IdentityRestoreError{
		"removal_moved_on", "This address was withdrawn again since. Put back the newer withdrawal instead.",
	}
	ErrIdentityDeclaredAgain = &IdentityRestoreError{
		"identity_declared_again", "This address was declared again after it was withdrawn, so it is already yours.",
	}
)

func (e *IdentityRestoreError) Error() string { return "capture: " + e.message }

// MessageFault names why the identity cannot come back.
func (e *IdentityRestoreError) MessageFault() (code, message string) { return e.code, e.message }

func (e *IdentityRestoreError) Unwrap() error { return apperrors.ErrConflict }

// withdrawnIdentity is a removed row with the columns the list never shows.
type withdrawnIdentity struct {
	OwnerIdentity
	createdBy   string
	auditID     ids.UUID
	withdrawnBy string
	withdrawnAt time.Time
}

// keepWithdrawalTx moves the removed row aside. A learned alias is also refused
// for good: its sightings are still stored, and the next delivery to it would
// claim it straight back.
func keepWithdrawalTx(ctx context.Context, tx pgx.Tx, w withdrawnIdentity) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO capture_owner_identity_withdrawn
		       (id, audit_id, withdrawn_by, user_id, kind, value, source, created_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		w.ID, w.auditID, w.withdrawnBy, w.UserID, w.Kind, w.Value, w.Source, w.createdBy, w.CreatedAt); err != nil {
		return fmt.Errorf("capture: keeping a withdrawn owner identity: %w", err)
	}
	if w.Source != IdentitySourceDeliveredTo {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO capture_alias_refused (user_id, value) VALUES ($1, $2)
		ON CONFLICT (user_id, value) DO NOTHING`, w.UserID, w.Value); err != nil {
		return fmt.Errorf("capture: refusing a withdrawn alias: %w", err)
	}
	return nil
}

// purgeExpiredWithdrawalsTx deletes every withdrawal past its undo. It runs on
// each remove and restore, the table's only writers, so no job is needed for a
// table that holds a handful of rows.
func purgeExpiredWithdrawalsTx(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, `
		DELETE FROM capture_owner_identity_withdrawn
		 WHERE withdrawn_at <= now() - make_interval(secs => $1)`, withdrawnIdentityKept.Seconds()); err != nil {
		return fmt.Errorf("capture: purging withdrawn owner identities: %w", err)
	}
	return nil
}

// RestoreRemoval puts back the identity the caller withdrew, as it was, and
// does what declaring it does.
func (s *OwnerIdentityStore) RestoreRemoval(ctx context.Context, id ids.UUID) (OwnerIdentity, error) {
	actor, err := seatItself(ctx)
	if err != nil {
		return OwnerIdentity{}, err
	}
	var out OwnerIdentity
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := purgeExpiredWithdrawalsTx(ctx, tx); err != nil {
			return err
		}
		withdrawn, err := ownWithdrawalTx(ctx, tx, actor.ID, actor.UserID, id)
		if err != nil {
			return err
		}
		if err := refuseMovedOnTx(ctx, tx, withdrawn); err != nil {
			return err
		}
		out, err = reinstateTx(ctx, tx, withdrawn)
		if err != nil {
			return err
		}
		if err := retirePendingForIdentityTx(ctx, tx, actor.UserID, out); err != nil {
			return err
		}
		ws := storekit.MustWorkspace(ctx)
		_, err = storekit.AuditEvent(storekit.WithReversal(ctx, captureSettingsObject, ws, withdrawn.auditID), tx,
			"update", captureSettingsObject, ws, ownerIdentityAuditImage(out))
		return err
	})
	return out, err
}

// ownWithdrawalTx answers not-found for anything but the caller's own withdrawal
// still inside its undo; an id says nothing about whose it is. The row lock
// makes a second concurrent restore find it gone and answer not-found too.
func ownWithdrawalTx(ctx context.Context, tx pgx.Tx, actorID string, user, id ids.UUID) (withdrawnIdentity, error) {
	var w withdrawnIdentity
	err := tx.QueryRow(ctx, `
		SELECT id, user_id, kind, value, source, created_by, created_at, audit_id, withdrawn_by, withdrawn_at
		  FROM capture_owner_identity_withdrawn
		 WHERE id = $1 AND user_id = $2 AND withdrawn_by = $3
		   FOR UPDATE`, id, user, actorID).
		Scan(&w.ID, &w.UserID, &w.Kind, &w.Value, &w.Source, &w.createdBy, &w.CreatedAt,
			&w.auditID, &w.withdrawnBy, &w.withdrawnAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return withdrawnIdentity{}, apperrors.ErrNotFound
	}
	if err != nil {
		return withdrawnIdentity{}, fmt.Errorf("capture: reading the withdrawal to restore: %w", err)
	}
	return w, nil
}

// refuseMovedOnTx refuses when the same address was declared and withdrawn
// again after this withdrawal: putting this row back would overwrite that
// change. Ids are minted at write time, so they also order a row whose
// transaction began first.
func refuseMovedOnTx(ctx context.Context, tx pgx.Tx, w withdrawnIdentity) error {
	var moved bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM capture_owner_identity_withdrawn later
		 WHERE later.id <> $1 AND later.user_id = $2 AND later.kind = $3 AND later.value = $4
		   AND (later.withdrawn_at > $5 OR later.audit_id > $6))`,
		w.ID, w.UserID, w.Kind, w.Value, w.withdrawnAt, w.auditID).Scan(&moved)
	if err != nil {
		return fmt.Errorf("capture: checking the identity for later changes: %w", err)
	}
	if moved {
		return ErrRemovalMovedOn
	}
	return nil
}

// reinstateTx writes the withdrawn row back, and lifts the refusal only for a
// learned alias, as keepWithdrawalTx set it. An address declared again in the
// meantime conflicts and is refused.
func reinstateTx(ctx context.Context, tx pgx.Tx, w withdrawnIdentity) (OwnerIdentity, error) {
	if w.Kind == IdentityKindAddress {
		if err := storekit.LockSeatAddressClaim(ctx, tx, w.Value); err != nil {
			return OwnerIdentity{}, err
		}
	}
	var out OwnerIdentity
	err := tx.QueryRow(ctx, `
		INSERT INTO capture_owner_identity (id, user_id, kind, value, source, created_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT DO NOTHING
		RETURNING id, user_id, kind, value, source, created_at`,
		w.ID, w.UserID, w.Kind, w.Value, w.Source, w.createdBy, w.CreatedAt).
		Scan(&out.ID, &out.UserID, &out.Kind, &out.Value, &out.Source, &out.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return OwnerIdentity{}, ErrIdentityDeclaredAgain
	}
	if err != nil {
		return OwnerIdentity{}, fmt.Errorf("capture: restoring an owner identity: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM capture_owner_identity_withdrawn WHERE id = $1`, w.ID); err != nil {
		return OwnerIdentity{}, fmt.Errorf("capture: restoring an owner identity: %w", err)
	}
	if w.Source != IdentitySourceDeliveredTo {
		return out, nil
	}
	if _, err := tx.Exec(ctx, `DELETE FROM capture_alias_refused WHERE user_id = $1 AND value = $2`,
		w.UserID, w.Value); err != nil {
		return OwnerIdentity{}, fmt.Errorf("capture: restoring an owner identity: %w", err)
	}
	return out, nil
}
