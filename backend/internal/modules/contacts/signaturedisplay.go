// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// followSignatureDisplay sets the display name to the name as the signature
// printed it, once both name columns landed and the display is still capture's
// guess. A display a colleague typed is left alone. Without this the display
// refresh would rebuild it as "first last", which reverses a family-name-first
// name.
func followSignatureDisplay(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, fields []SignatureField, applied []string) error {
	if !slices.Contains(applied, columnFirstName) || !slices.Contains(applied, columnLastName) {
		return nil
	}
	var first, last, display string
	for _, f := range fields {
		switch f.Name {
		case columnFirstName:
			first, display = strings.TrimSpace(f.Value), strings.TrimSpace(f.Display)
		case columnLastName:
			last = strings.TrimSpace(f.Value)
		}
	}
	if display == "" {
		return nil
	}
	var previous string
	var storedFirst, storedLast *string
	if err := tx.QueryRow(ctx,
		`SELECT full_name, first_name, last_name FROM contact WHERE id = $1 FOR UPDATE`, contactID).
		Scan(&previous, &storedFirst, &storedLast); err != nil {
		return fmt.Errorf("contacts: reading the name contact %s shows: %w", contactID, err)
	}
	// A colleague's pair leaves the columns unchanged. Then the signature has no
	// say over the display.
	if previous == display || derefString(storedFirst) != first || derefString(storedLast) != last {
		return nil
	}
	follows, err := displayNameFollowsSignatureTx(ctx, tx, contactID)
	if err != nil || !follows {
		return err
	}
	_, err = rewriteFullNameTx(ctx, tx, contactID, previous, display)
	return err
}
