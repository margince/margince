// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Putting a name the record already knows onto the page.
//
// completeContactName beside this does the same thing as it LEARNS a name, under
// a predicate demanding both split columns be empty — it completes a record, and
// a record whose parts are already filled is not its business. This is for the
// contacts that learned their name before the display followed it: the parts are
// filled and correct, and only what a reader sees is stale.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RefreshDisplayNameTx sets one contact's display name to the name its own
// columns already carry, and answers whether it moved.
//
// Only where displayNameIsStaleTx holds, asked under the row lock. A repair
// pass selects a page of ids and then writes them one at a time. A colleague who
// renames one of those contacts in between keeps their name.
func RefreshDisplayNameTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) (bool, error) {
	// Locked before it is read, for the reason completeContactName states: the
	// value recorded as the audit's before must be the one this write replaces,
	// and a human editing the name between an unlocked read and the write would
	// have their value recorded as the after of a change they did not make.
	var previous, first, last string
	err := tx.QueryRow(ctx, `
		SELECT full_name, coalesce(first_name, ''), coalesce(last_name, '')
		  FROM contact WHERE id = $1 FOR UPDATE`, contactID).Scan(&previous, &first, &last)
	if errors.Is(err, pgx.ErrNoRows) {
		// Erased or deleted. No name left to show.
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("contacts: reading the name contact %s shows: %w", contactID, err)
	}
	stale, err := displayNameIsStaleTx(ctx, tx, contactID)
	if err != nil {
		return false, err
	}
	learned := strings.TrimSpace(first) + " " + strings.TrimSpace(last)
	if !stale {
		var ok bool
		if learned, ok, err = initialsTargetTx(ctx, tx, contactID, previous, first, last); err != nil || !ok {
			return false, err
		}
	}
	return rewriteFullNameTx(ctx, tx, contactID, previous, learned)
}

// initialsTargetTx answers the local part that replaces the initials capture
// guessed, for a contact with no split columns. It asks the same statement as
// the selector, under the row lock, so a name typed since the select stays.
func initialsTargetTx(
	ctx context.Context, tx pgx.Tx, contactID ids.ContactID, previous, first, last string,
) (string, bool, error) {
	if first != "" || last != "" || utf8.RuneCountInString(previous) > maxInitialsDisplayLen {
		return "", false, nil
	}
	rule := displayRule{unsplitMaxLen: maxInitialsDisplayLen}
	found, err := captureGuessedDisplayNamesTx(ctx, tx, &contactID, nil, rule, 1)
	if err != nil || len(found) != 1 {
		return "", false, err
	}
	candidate, ok, err := initialsCandidateTx(ctx, tx, contactID)
	return candidate.Proposed, ok, err
}
