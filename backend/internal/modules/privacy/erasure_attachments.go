// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The attachment half of Art. 17 erasure: the subject's uploaded BYTES, not
// only the rows that name them. It lives beside erasure.go rather than inside
// it so the cascade file stays under the file-length cap; erasureCascadeFiles
// in the PII-coverage gate lists both, so extracting this scrub here does not
// take the attachment table out of that gate's sight.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// subjectAttachmentsWhere selects the attachments Art. 17 erasure removes for
// a contact: those hung off the contact and those on the contact's destroyable
// subject-only activities (floor-shielded correspondence keeps its
// attachments too). $1 is the contact id; $2/$3 are the statutory floor policy
// (FloorPolicy.sqlArgs).
var subjectAttachmentsWhere = `(entity_type = 'contact' AND entity_id = $1)
	   OR (entity_type = 'activity' AND entity_id IN (` + subjectOnlyDestroyable + `))`

// eraseAttachments purges the objects only the matched attachments name, then
// deletes their rows, in the caller's transaction. The keys live in the rows.
// A failure, such as a store error or no store, rolls back with the keys
// intact. A retry purges again, and no bytes are left without their key.
// Erasure is rare, so holding object-store I/O under the transaction is fine.
func (e *Eraser) eraseAttachments(ctx context.Context, tx pgx.Tx, reason, cause, where string, args ...any) error {
	keys, err := soleHeldObjects(ctx, tx, where, args...)
	if err != nil {
		return err
	}
	if len(keys) > 0 {
		if e.blob == nil {
			return fmt.Errorf("privacy: %d attachment object(s) to purge but no object store is configured", len(keys))
		}
		for _, key := range keys {
			if err := e.blob.Delete(ctx, key); err != nil {
				return fmt.Errorf("privacy: purging attachment object: %w", err)
			}
		}
		destroyedOutside(tx, armAttachmentObject)
	}
	// Keep the deleted IDs so their retained audit and event copies go too.
	scrubbed, err := scrubbedIDs(ctx, tx, `DELETE FROM attachment WHERE `+where+` RETURNING id`, args...)
	if err != nil {
		return err
	}
	return tombstoneCollateralScrubs(ctx, tx, "attachment", scrubbed, reason, cause)
}

// soleHeldObjects locks the matched rows and answers the objects only they
// name. Identical files share one object, and an object another row still
// names stays. A withheld file's row has no object.
//
// Two statements, because a statement waiting on a row lock keeps its own
// snapshot. A capture holding a source row `FOR SHARE` may commit a new row
// naming the object meanwhile, and only the second statement sees it.
func soleHeldObjects(ctx context.Context, tx pgx.Tx, where string, args ...any) ([]string, error) {
	locked, err := tx.Query(ctx, `SELECT id FROM attachment WHERE `+where+` FOR UPDATE`, args...)
	if err != nil {
		return nil, err
	}
	erased, err := pgx.CollectRows(locked, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("privacy: locking the attachments to erase: %w", err)
	}
	if len(erased) == 0 {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT e.storage_key FROM attachment e
		 WHERE e.id = ANY($1::uuid[]) AND NOT e.bytes_withheld AND e.storage_key <> ''
		   AND NOT EXISTS (SELECT 1 FROM attachment o
		                    WHERE o.storage_key = e.storage_key AND o.id <> ALL($1::uuid[]))`, erased)
	if err != nil {
		return nil, err
	}
	keys, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("privacy: reading the attachment objects to purge: %w", err)
	}
	return keys, nil
}

// scrubbedIDs runs a statement that returns the ids it scrubbed.
func scrubbedIDs(ctx context.Context, tx pgx.Tx, sql string, args ...any) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var scrubbed []ids.UUID
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		scrubbed = append(scrubbed, id)
	}
	return scrubbed, rows.Err()
}
