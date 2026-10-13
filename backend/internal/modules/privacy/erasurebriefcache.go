// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// purgeSubjectBriefCache destroys every reader's cached relationship brief for
// a subject the caller has just anonymized.
//
// contact_brief.payload is the generated brief itself, and what the generator is
// shown is what it holds: the claims extracted from the subject's
// conversations, what changed about the relationship, and a one-line summary of
// each recent message. That is the subject's prose and the reader's notes about
// them in one jsonb column.
//
// The ON DELETE CASCADE on contact_id does not fire, for the reason the
// duplicate-pair snapshot one file over outlives its act: the erasure
// ANONYMIZES the contact row rather than deleting it, so a cascade keyed to the
// row's disappearance never runs and the cache keeps serving the name the
// erasure just destroyed.
//
// The ROW is deleted rather than emptied. payload is NOT NULL with no empty
// shape to write, nothing references the cache, and it is rebuilt on demand
// from whatever the record says then — so absence is the correct state for a
// subject there is no longer a brief to write about.
//
// Keyed by (user_id, contact_id), so this reaches every COLLEAGUE's copy and
// not merely the one belonging to whoever asked for the erasure.
//
// One subject rather than a set: the cache's only subject key is contact_id, so
// the lead twins the same erasure wipes have no row here to reach.
func purgeSubjectBriefCache(ctx context.Context, tx pgx.Tx, subject ids.ContactID) error {
	if _, err := tx.Exec(ctx,
		`DELETE FROM contact_brief WHERE contact_id = $1`, subject); err != nil {
		return fmt.Errorf("privacy: destroying the subject's cached relationship brief: %w", err)
	}
	return nil
}
