// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Ending a mark the sender check set, once the workspace corresponds with the contact.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// AutomaticallyMarkedTx reports whether the contact carries a mark that the
// sender check set. A mark a human or an agent set returns false.
// Only they may take it off.
func (s *Store) AutomaticallyMarkedTx(ctx context.Context, tx pgx.Tx, id ids.ContactID) (bool, error) {
	if err := auth.Require(ctx, entityContact, principal.ActionRead); err != nil {
		return false, err
	}
	var marked bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM contact
		                WHERE id = $1 AND archived_at IS NULL
		                  AND relationship_mark IS NOT NULL AND relationship_mark_set_by = $2)`,
		id, CaptureVerdictActor).Scan(&marked)
	if err != nil {
		return false, fmt.Errorf("contacts: reading whether the sender check marked the contact: %w", err)
	}
	return marked, nil
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
