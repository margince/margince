// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// A restriction always archives its row (privacy/erasure_restrict.go's
// restrictShieldedTimeline, privacy/restrictionoverride.go's PinToFloor), so
// a write that resolves the row storekit.LiveOnly first cannot tell a
// genuinely missing row from a held one — both answer apperrors.ErrNotFound,
// and the activity_restricted_immutable CHECK that would answer 423 sits
// behind an UPDATE the caller never reaches. privacy.pinRefusalFor draws the
// same distinction for the restriction override's own refusal path; this is
// the write side of it.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// activityRetentionHeld reports whether id names a currently existing
// activity under a statutory retention hold. Meaningful only read under a
// lock already held on the row: an unlocked read here is stale the instant
// a concurrent lift commits, which is exactly the bug lockActivityForWrite,
// its caller below, was rewritten to stop making. A row it cannot find at
// all answers false, not an error, for a caller that has not locked one and
// only wants to know whether a just-vanished id was ever held.
func activityRetentionHeld(ctx context.Context, tx pgx.Tx, id ids.UUID) (bool, error) {
	var held bool
	err := tx.QueryRow(ctx,
		`SELECT restricted_at IS NOT NULL FROM activity WHERE id = $1`, id).Scan(&held)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return held, err
}

// lockActivityForWrite is storekit.LockRow against the activity table,
// except that a row LiveOnly cannot see is LOCKED before being classified
// gone or held, never the other way round. Reading restricted_at first and
// locking second — this function's first shipped version — is exactly the
// TOCTOU LockRow's own doc warns against: a lift (privacy's
// liftAndEraseHeldRecord, run by the expiry sweep or an admin release) can
// clear restricted_at between an unlocked read and a later lock, leaving
// held stale-true for a row that is now merely archived, ordinary, and
// entitled to nothing more than the 404 it always got. Locking first with
// IncludeArchived and reading restricted_at under that lock closes the
// window: nothing else can move the column again before this transaction
// commits, so the classification this returns is the row's true state for
// every read and write that follows in the same transaction.
//
// A held row is deliberately locked WITH archived rows included, and the
// caller is told so: every read of the same row that follows must resolve
// it with storekit.IncludeArchived too, and the caller's own
// auth.EnsureActivityWritable check must be told the row isn't live either
// (auth.EnsureActivityWritableIn(..., live=false)) — so the write reaches
// the row and activity_refuse_restricted_mutation, not this call or that
// one, is what refuses it, as 423 rather than a 404 that denies the row is
// even there.
func lockActivityForWrite(ctx context.Context, tx pgx.Tx, id ids.UUID) (held bool, err error) {
	if _, err := storekit.LockRow(ctx, tx, "activity", id, storekit.LiveOnly); err == nil {
		return false, nil
	} else if !errors.Is(err, apperrors.ErrNotFound) {
		return false, err
	}
	if _, err := storekit.LockRow(ctx, tx, "activity", id, storekit.IncludeArchived); err != nil {
		return false, err
	}
	held, err = activityRetentionHeld(ctx, tx, id)
	if err != nil {
		return false, err
	}
	if !held {
		return false, apperrors.ErrNotFound
	}
	return true, nil
}

// activityArchivedFilter is the storekit.ArchivedFilter a caller must resolve
// the rest of its transaction's reads and writes against, given the held
// witness lockActivityForWrite returned.
func activityArchivedFilter(held bool) storekit.ArchivedFilter {
	if held {
		return storekit.IncludeArchived
	}
	return storekit.LiveOnly
}

// readActivityForWrite reads the row a write path already resolved through
// lockActivityForWrite, using readHeldActivity's DISCOVER-gate bypass only
// when held is true — the same dispatch activityArchivedFilter makes for the
// archived filter, kept as its own function so a call site never has to
// choose between readActivity and readHeldActivity by hand.
func readActivityForWrite(ctx context.Context, tx pgx.Tx, id ids.ActivityID, held bool) (crmcontracts.Activity, error) {
	if held {
		return readHeldActivity(ctx, tx, id, storekit.IncludeArchived)
	}
	return readActivity(ctx, tx, id, storekit.LiveOnly)
}
