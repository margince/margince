// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The message a mailed card arrived on, and whether it may still be read.
//
// Its own file because it is its own concept: the rest of the import is about
// turning a card into a person, and this is about whether the card may be acted
// on at all. It exists only for cards that ARRIVED somewhere — a browser upload
// is a human holding a file, with no source row whose audience can change under
// it.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// vcardSourceNarrowed is what a card gets when its message may no longer be
// read here. Worded for the reader of an import report rather than for a log:
// it says what happened to the message, because that is the thing they can go
// and look at.
const vcardSourceNarrowed = "the message this card arrived on is no longer readable here"

// sourceLendsItsCard answers whether the message a card arrived on may still be
// read, under a lock that lasts as long as the caller's transaction.
//
// FOR SHARE, and taken as the first statement of the card's own write
// transaction. That placement is the guard: a check made before the transaction
// reads an open message, returns, and the write then happens in a later
// transaction — so a narrowing committing in between lands the card anyway.
// Held here, there is no "between".
//
// A message that is GONE lends nothing. Erased or never written, both mean there
// is no source to import from, and both are answers rather than faults.
// sourceIsNarrowed is the caller-facing shape: a card with NO source (a browser
// upload) is never narrowed, so the caller does not repeat that test at each of
// its arms.
func sourceIsNarrowed(ctx context.Context, tx pgx.Tx, source ids.UUID) (bool, error) {
	if source.IsZero() {
		return false, nil
	}
	open, err := sourceLendsItsCard(ctx, tx, source)
	return !open, err
}

func sourceLendsItsCard(ctx context.Context, tx pgx.Tx, source ids.UUID) (bool, error) {
	var open bool
	err := tx.QueryRow(ctx, `
		SELECT audience = 'workspace'
		       AND restricted_at IS NULL
		       AND archived_at IS NULL
		  FROM activity WHERE id = $1 FOR SHARE`, source).Scan(&open)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("people: reading the message a card arrived on: %w", err)
	}
	return open, nil
}
