// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// leadMemberships names the Shortlist rows of one lead, in the form the archive
// paths of the other record types use for theirs.
const leadMemberships = "entity_type = 'lead' AND entity_id = @record"

// dropShortlistMemberships removes the record's Shortlist memberships and records
// each removal in list_member_event as caused by the archive. A membership that
// outlived its record would render as a gap in the list. memberships selects the
// record's rows and binds @record. Company, contact and lead retirement share
// this statement so a Shortlist never keeps a retired record.
func dropShortlistMemberships(
	ctx context.Context, tx pgx.Tx, cascade *storekit.ArchiveCascade, memberships string, id ids.UUID,
) error {
	return cascade.DropMemberships(ctx, tx,
		`WITH gone AS (
			DELETE FROM list_member WHERE `+memberships+`
			RETURNING list_id, entity_type, entity_id, added_by, created_at, note),
		logged AS (
			INSERT INTO list_member_event (list_id, entity_type, entity_id, action, reason, actor)
			SELECT list_id, entity_type, entity_id, 'removed', 'record_archived', @actor FROM gone)
		SELECT list_id, added_by, created_at, note FROM gone`, id)
}
