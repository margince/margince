// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// releaseOrphanedSuggestions returns a suggestion to `unmatched` once its
// contact is archived, which includes the source of a contact merge.
//
// The pending read and both match arms need a live contact, so such a row can
// never be proposed or decided. Every sweep still enumerates it. Once released, the matcher judges it against the surviving contact.
//
// owner is ids.Nil for every member's rows, as in the match itself.
func releaseOrphanedSuggestions(ctx context.Context, tx pgx.Tx, owner ids.UUID) error {
	rows, err := tx.Query(ctx, `
		UPDATE linkedin_connection c
		   SET match_status = 'unmatched', matched_contact_id = NULL, updated_at = now()
		  FROM linkedin_connection was
		 WHERE was.id = c.id AND c.tombstoned_at IS NULL
		   AND c.match_status = 'suggested'
		   AND ($1::uuid IS NULL OR c.owner_user_id = $1)
		   AND EXISTS (SELECT 1 FROM contact p
		                WHERE p.id = c.matched_contact_id AND p.archived_at IS NOT NULL)
		RETURNING c.id, was.matched_contact_id`, nullableOwner(owner))
	if err != nil {
		return fmt.Errorf("contacts: releasing LinkedIn suggestions whose contact is archived: %w", err)
	}
	type released struct {
		id      ids.UUID
		contact *ids.UUID
	}
	var done []released
	for rows.Next() {
		var r released
		if err := rows.Scan(&r.id, &r.contact); err != nil {
			rows.Close()
			return err
		}
		done = append(done, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, r := range done {
		images := matchImages(matchSuggested, r.contact, "unmatched", nil)
		if _, err := storekit.Audit(ctx, tx, "update", "linkedin_connection", r.id, images.before, images.after); err != nil {
			return err
		}
	}
	return nil
}

// relinkLinkedInConnections points the connections matched to the merged-away
// contact at the survivor, whatever their status.
//
// A merge archives the source, so a connection left on it would read as an
// orphan and lose a suggestion the survivor deserves. Moving it here keeps one
// place that moves a contact's references.
func relinkLinkedInConnections(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.ContactID) error {
	rows, err := tx.Query(ctx, `
		UPDATE linkedin_connection SET matched_contact_id = $2, updated_at = now()
		 WHERE matched_contact_id = $1 AND tombstoned_at IS NULL
		RETURNING id, match_status`, sourceID.UUID, targetID.UUID)
	if err != nil {
		return fmt.Errorf("contacts: moving LinkedIn connections to the survivor: %w", err)
	}
	type moved struct {
		id     ids.UUID
		status string
	}
	var done []moved
	for rows.Next() {
		var m moved
		if err := rows.Scan(&m.id, &m.status); err != nil {
			rows.Close()
			return err
		}
		done = append(done, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, m := range done {
		images := matchImages(m.status, &sourceID.UUID, m.status, &targetID.UUID)
		if _, err := storekit.Audit(ctx, tx, "update", "linkedin_connection", m.id, images.before, images.after); err != nil {
			return err
		}
	}
	return nil
}
