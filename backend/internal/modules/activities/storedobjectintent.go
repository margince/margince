// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The intent ledger: a stored object says it is provisional until its row
// arrives.
//
// Storing a file is two writes to two systems that cannot be made atomic, and
// both writers here put the bytes FIRST — an unreferenced object is a better
// failure than a row promising bytes that are not there. The cost of that
// choice is the object left behind when the transaction after the put fails,
// which nothing references and nothing could ever delete: an erasure finds an
// attachment's bytes by reading storage_key off its row, so an object with no
// row is one the erasure cannot reach.
//
// So the key is recorded BEFORE the put and cleared in the SAME transaction
// that writes the row. A key still recorded after the grace period is an object
// whose row never arrived, and the sweep deletes it — but only ever a key
// something declared provisional, which is what makes a bug here unable to
// reach a live file.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// recordStoredObjectIntent declares a key provisional.
//
// It commits on its OWN transaction, and that is the whole point rather than an
// oversight: the row has to survive the failure of the transaction that was
// going to write the attachment. Recorded inside it, it would roll back with
// the row it exists to catch the absence of.
func (s *Store) recordStoredObjectIntent(ctx context.Context, key string) error {
	// Bound to the workspace from the CONTEXT rather than from whatever this
	// store was constructed against, and bound to the same one the key's own
	// prefix carries (blobstore.WorkspaceKey reads it from here too). The
	// capture path reaches this from an installation-wide handle, where a
	// store-bound resolve has no single workspace to find.
	return s.db.ForWorkspace(workspaceID(ctx)).Tx(ctx, func(tx pgx.Tx) error {
		// ON CONFLICT because a retried upload of the same id is not an error
		// worth failing a caller's file over, and the row it would insert is
		// the row already there.
		_, err := tx.Exec(ctx, `
			INSERT INTO stored_object_intent (storage_key) VALUES ($1)
			ON CONFLICT (storage_key) DO NOTHING`, key)
		if err != nil {
			return fmt.Errorf("record a provisional object: %w", err)
		}
		return nil
	})
}

// clearStoredObjectIntent retires a key, on the caller's transaction.
//
// ON THE CALLER'S, so the clear and the attachment row commit together: a clear
// that committed separately could land while the row's transaction then failed,
// which is the orphan this ledger exists to catch, re-created one step along.
func clearStoredObjectIntent(ctx context.Context, tx pgx.Tx, key string) error {
	if _, err := tx.Exec(ctx,
		`DELETE FROM stored_object_intent WHERE storage_key = $1`, key); err != nil {
		return fmt.Errorf("retire a provisional object: %w", err)
	}
	return nil
}

// ProvisionalObjectGrace is how long a key may stay provisional before the
// sweep treats it as an orphan.
//
// Generous against the window it has to clear: the transaction between the put
// and the row is a handful of statements, and the capture path holds one open
// across an object-store write bounded at capturedFileStoreTimeout. An hour is
// far past both, and the cost of being generous is an orphan living an hour
// longer — where the cost of being tight is deleting the object of an upload
// that was still in flight.
const ProvisionalObjectGrace = time.Hour

// OrphanedObject is one key the sweep may delete: provisional past the grace
// period, and referenced by no attachment row.
type OrphanedObject struct {
	StorageKey string
	RecordedAt time.Time
}

// ListOrphanedObjects answers the keys a reaper may delete, oldest first.
//
// THE ATTACHMENT JOIN IS THE SECOND LOCK, not the first. The first is that a
// key is here at all — nothing but a writer's own declaration puts one in this
// table. The join exists because the two are different failures: a clear that
// did not run leaves a live file's key provisional, and a reaper that trusted
// the table alone would delete the bytes of a document somebody can still see
// in the product.
//
// `limit` bounds one pass. A sweep that could run unbounded over a large
// backlog is one that holds a worker for as long as the backlog is deep.
func (s *Store) ListOrphanedObjects(ctx context.Context, before time.Time, limit int) ([]OrphanedObject, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	var out []OrphanedObject
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT i.storage_key, i.recorded_at
			  FROM stored_object_intent i
			 WHERE i.recorded_at < $1
			   AND NOT EXISTS (SELECT 1 FROM attachment a WHERE a.storage_key = i.storage_key)
			 ORDER BY i.recorded_at
			 LIMIT $2`, before, limit)
		if err != nil {
			return fmt.Errorf("list provisional objects: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var o OrphanedObject
			if err := rows.Scan(&o.StorageKey, &o.RecordedAt); err != nil {
				return fmt.Errorf("scan a provisional object: %w", err)
			}
			out = append(out, o)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// RetireOrphanedObject removes one key from the ledger once its bytes are gone.
//
// Called AFTER the object-store delete, so a failed delete leaves the key here
// and the next pass tries again. The other order would forget an object that is
// still there, which is the state this whole ledger exists to make impossible.
func (s *Store) RetireOrphanedObject(ctx context.Context, key string) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		return clearStoredObjectIntent(ctx, tx, key)
	})
}
