// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"maps"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// keptFromAnAgent adds to corrected every field a card states and the contact
// already holds, when an agent uploaded the card. A user holding the card may
// correct a record with it. An agent's card fills blanks and replaces no answer.
func keptFromAnAgent(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, stated []string, corrected map[string]bool) (map[string]bool, error) {
	if actor, ok := principal.Actor(ctx); !ok || actor.Type != principal.PrincipalAgent {
		return corrected, nil
	}
	kept := maps.Clone(corrected)
	if kept == nil {
		kept = map[string]bool{}
	}
	for _, field := range stated {
		holds, err := contactHoldsField(ctx, tx, contactID, field)
		if err != nil {
			return nil, err
		}
		kept[field] = kept[field] || holds
	}
	var hasPhone bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM contact_phone WHERE contact_id = $1 AND archived_at IS NULL)`, contactID).Scan(&hasPhone); err != nil {
		return nil, fmt.Errorf("contacts: reading whether the contact holds a phone: %w", err)
	}
	kept[fieldPhone] = kept[fieldPhone] || hasPhone
	return kept, nil
}

// contactHoldsField reads a field from its sidecar row or its mirrored column,
// under the read grant fillAddressFromCard asks the same card for.
func contactHoldsField(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, field string) (bool, error) {
	if err := auth.Require(ctx, entityContact, principal.ActionRead); err != nil {
		return false, err
	}
	held, err := heldFieldValue(ctx, tx, contactID, field)
	if err != nil || held != "" {
		return held != "", err
	}
	column, mirrored := observedFieldColumn(field)
	if !mirrored {
		return false, nil
	}
	var inColumn string
	if err := tx.QueryRow(ctx, `SELECT coalesce(`+column+`::text, '') FROM contact WHERE id = $1`, contactID).Scan(&inColumn); err != nil {
		return false, fmt.Errorf("contacts: reading the contact's %s: %w", field, err)
	}
	return inColumn != "", nil
}
