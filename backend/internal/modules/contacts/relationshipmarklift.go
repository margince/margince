// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Ending a mark the sender check set, once the workspace corresponds with the contact.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// LockAutomaticMarkTx locks the contact and returns when it was made, if it
// carries a mark the sender check set and still holds this address.
//
// The sender check sets the mark only when it makes the contact, so the
// creation time is when the mark was set. A mark a human or an agent set
// reports false, and so does a contact the address has moved away from.
// The caller decides and clears under this lock.
func (s *Store) LockAutomaticMarkTx(ctx context.Context, tx pgx.Tx, id ids.ContactID, email string) (time.Time, bool, error) {
	if err := auth.Require(ctx, entityContact, principal.ActionRead); err != nil {
		return time.Time{}, false, err
	}
	var madeAt time.Time
	err := tx.QueryRow(ctx, `
		SELECT c.created_at FROM contact c
		 WHERE c.id = $1 AND c.archived_at IS NULL
		   AND c.relationship_mark IS NOT NULL AND c.relationship_mark_set_by = $2
		   AND EXISTS (SELECT 1 FROM contact_email ce
		                WHERE ce.contact_id = c.id AND ce.archived_at IS NULL
		                  AND lower(ce.email) = lower($3))
		   FOR UPDATE OF c`, id, CaptureVerdictActor, email).Scan(&madeAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("contacts: locking the contact the sender check marked: %w", err)
	}
	return madeAt, true, nil
}

// LiftAutomaticMarkTx clears the mark on a contact when the sender check set
// it, and reports whether it did.
//
// The setter pin in the UPDATE is the whole rule and the concurrency guard.
// A second pass, or a human who set the mark in between, matches no row and
// writes nothing. A cleared mark is audited and announced like any contact edit.
func (s *Store) LiftAutomaticMarkTx(ctx context.Context, tx pgx.Tx, id ids.ContactID) (bool, error) {
	if err := auth.Require(ctx, entityContact, principal.ActionUpdate); err != nil {
		return false, err
	}
	var was string
	err := tx.QueryRow(ctx, `
		UPDATE contact c SET relationship_mark = NULL, relationship_mark_set_by = NULL
		  FROM (SELECT id, relationship_mark FROM contact WHERE id = $1 FOR UPDATE) old
		 WHERE c.id = old.id AND c.archived_at IS NULL
		   AND c.relationship_mark IS NOT NULL AND c.relationship_mark_set_by = $2
		RETURNING old.relationship_mark`, id, CaptureVerdictActor).Scan(&was)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("contacts: clearing the mark the sender check set: %w", err)
	}
	auditID, err := storekit.Audit(ctx, tx, "update", entityContact, id.UUID,
		map[string]any{fieldRelationshipMark: was}, map[string]any{fieldRelationshipMark: nil})
	if err != nil {
		return false, fmt.Errorf("contacts: recording the cleared mark: %w", err)
	}
	return true, storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventContactUpdated{
		ChangedFields: map[string]any{fieldRelationshipMark: nil},
	})
}
