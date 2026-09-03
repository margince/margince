// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The one-time link material behind a controller delivery: a live capability
// sitting in the key vault, referenced by a row Art. 17 is about to scrub.
//
// Its own file beside erasure_attachments.go, which solves the same problem for
// attachment bytes and in the same order — and the order is the whole point.
// The reference lives in the row. Clearing the row first would leave a working
// confirmation link in the vault with no key left to find it, so the material
// is destroyed FIRST and the row scrubbed after: any failure rolls the
// transaction back with the reference intact, and a retry destroys it again
// idempotently.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// PayloadPurger destroys one piece of stored link material. Compose supplies it
// over the key vault; privacy neither mints nor reads these values.
type PayloadPurger interface {
	Delete(ctx context.Context, ref string) error
}

// erasePayloads destroys the link material behind the subject's controller
// deliveries and clears the references that named it.
//
// A subject is reached through their DELIVERIES rather than through a person
// column, because comms_outbound carries no subject id: the tie is the activity,
// which is exactly how the rest of the delivery scrub finds these rows.
func erasePayloads(ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID, payloads PayloadPurger) error {
	if len(activityIDs) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `
		SELECT payload_ref FROM comms_outbound
		 WHERE activity_id = ANY($1) AND payload_ref IS NOT NULL`, activityIDs)
	if err != nil {
		return fmt.Errorf("privacy: reading the subject's link material: %w", err)
	}
	var refs []string
	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			rows.Close()
			return fmt.Errorf("privacy: reading the subject's link material: %w", err)
		}
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("privacy: reading the subject's link material: %w", err)
	}
	if len(refs) == 0 {
		return nil
	}
	if payloads == nil {
		// Loud, like the attachment purge beside it: material exists and this
		// eraser cannot destroy it, so certifying the erasure would be a false
		// statement about a live credential.
		return fmt.Errorf("privacy: %d link secret(s) to destroy but no payload vault is configured", len(refs))
	}
	for _, ref := range refs {
		if err := payloads.Delete(ctx, ref); err != nil {
			return fmt.Errorf("privacy: destroying the subject's link material: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE comms_outbound SET payload_ref = NULL
		 WHERE activity_id = ANY($1) AND payload_ref IS NOT NULL`, activityIDs); err != nil {
		return fmt.Errorf("privacy: clearing the subject's link references: %w", err)
	}
	return nil
}
