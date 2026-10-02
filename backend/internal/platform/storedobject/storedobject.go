// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package storedobject owns the intent ledger: a stored object says it is
// provisional until the row that names it arrives.
//
// Storing a file is two writes to two systems that cannot be made atomic, and
// every writer puts the bytes FIRST — an unreferenced object is a better
// failure than a row promising bytes that are not there. The cost of that
// choice is the object left behind when the transaction after the put fails,
// which nothing references and nothing could ever delete: an erasure finds an
// object by reading its key off the row, so an object with no row is one the
// erasure cannot reach.
//
// So the key is recorded BEFORE the put and cleared in the SAME transaction
// that writes the row. A key still recorded after its kind's grace period is an
// object whose row never arrived, and the reap deletes it — but only ever a key
// something declared provisional, of a kind some module declared the columns
// for, which is what makes a bug here unable to reach a live file.
//
// It writes stored_object_intent and nothing else, and records no audit row:
// the ledger is bookkeeping about bytes, and every row that names those bytes
// is audited by the module that writes it.
package storedobject

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Record declares a key provisional, before its bytes are put, and answers
// ErrExpired for a key the reap condemned: its bytes are gone or going.
//
// It commits on its OWN transaction, and that is the whole point rather than an
// oversight: the row has to survive the failure of the transaction that was
// going to write the referencing row. Recorded inside it, it would roll back
// with the row it exists to catch the absence of — which is also why an ambient
// read snapshot is detached rather than joined.
func Record(ctx context.Context, db *database.DB, key string) error {
	// Bound to the workspace from the CONTEXT rather than from whatever the
	// handle was constructed against, and so to the same one the key's own
	// prefix carries (blobstore.WorkspaceKey is minted from it too). A capture
	// path reaches this from an installation-wide handle, where a handle-bound
	// resolve has no single workspace to find.
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return fmt.Errorf("record a provisional object: %w", database.ErrNoWorkspace)
	}
	ctx = database.Detached(ctx)
	return db.ForWorkspace(ids.From[ids.WorkspaceKind](ws)).Tx(ctx, func(tx pgx.Tx) error {
		// A key already recorded restarts its grace, so a retried put is not
		// reaped on the first attempt's clock; a stamped one matches nothing.
		tag, err := tx.Exec(ctx, `
			INSERT INTO stored_object_intent (storage_key) VALUES ($1)
			ON CONFLICT (storage_key) DO UPDATE SET recorded_at = now()
			 WHERE stored_object_intent.reaping_since IS NULL`, key)
		if err != nil {
			return fmt.Errorf("record a provisional object: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrExpired
		}
		return nil
	})
}

// ErrExpired is a key the reap has already committed to deleting: the bytes
// are gone or going, and the caller has to store them again.
var ErrExpired = fmt.Errorf("the stored file has expired; upload it again: %w", apperrors.ErrNotFound)

// Claim restarts a provisional key's grace, for a writer whose row follows the
// put after an open-ended wait (an import source is staged only once somebody
// has mapped its columns). It answers ErrExpired for a key the reap condemned.
//
// Called BEFORE the claimant reads the bytes, on its OWN transaction for the
// reason Record gives. The row lock orders it against Ledger.Condemn: a claim
// that commits first leaves the key inside its grace when the reap re-checks
// it, and a condemnation that commits first is what this refuses. A key with
// no row is already referenced (its clear ran) or predates the ledger; neither
// is the reap's to delete, so there is nothing to hold.
func Claim(ctx context.Context, db *database.DB, key string) error {
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return fmt.Errorf("claim a provisional object: %w", database.ErrNoWorkspace)
	}
	ctx = database.Detached(ctx)
	return db.ForWorkspace(ids.From[ids.WorkspaceKind](ws)).Tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE stored_object_intent SET recorded_at = now()
			 WHERE storage_key = $1 AND reaping_since IS NULL`, key)
		if err != nil {
			return fmt.Errorf("claim a provisional object: %w", err)
		}
		if tag.RowsAffected() == 1 {
			return nil
		}
		return refuseCondemned(ctx, tx, key)
	})
}

// Clear retires a key, on the caller's transaction, and answers ErrExpired for
// a key the reap condemned, so that transaction aborts rather than commit a
// row naming bytes being deleted.
//
// ON THE CALLER'S, so the clear and the referencing row commit together: a
// clear that committed separately could land while the row's transaction then
// failed, which is the orphan this ledger exists to catch, re-created one step
// along. The stamp matters because the reap's re-check cannot see a row not yet
// committed, so a writer that outlives its kind's grace can be condemned.
func Clear(ctx context.Context, tx pgx.Tx, key string) error {
	tag, err := tx.Exec(ctx,
		`DELETE FROM stored_object_intent WHERE storage_key = $1 AND reaping_since IS NULL`, key)
	if err != nil {
		return fmt.Errorf("retire a provisional object: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	return refuseCondemned(ctx, tx, key)
}

// refuseCondemned answers ErrExpired when key is still in the ledger after a
// statement that matched only an unstamped row, since the row left is stamped.
// No row at all is a key already referenced or older than the ledger, and
// neither is the reap's to delete.
func refuseCondemned(ctx context.Context, tx pgx.Tx, key string) error {
	var condemned bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM stored_object_intent WHERE storage_key = $1)`, key,
	).Scan(&condemned); err != nil {
		return fmt.Errorf("read a provisional object: %w", err)
	}
	if condemned {
		return ErrExpired
	}
	return nil
}
