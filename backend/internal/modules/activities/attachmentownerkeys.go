// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// AttachmentOwnerPage separates rows repaired from older owner kinds that need review.
type AttachmentOwnerPage struct {
	IDs         []ids.UUID
	NeedsReview []ids.UUID
}

// KeyAttachmentOwnersTx fills one bounded page of owner keys. Missing owners
// remove the attachment as CASCADE would; its delete trigger queues the bytes.
// Unsupported old kinds are kept intact and reported for a human decision.
func KeyAttachmentOwnersTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (AttachmentOwnerPage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return AttachmentOwnerPage{}, err
	}
	rows, err := tx.Query(ctx, `WITH page AS (
 SELECT a.id, a.entity_type NOT IN ('contact','company','deal','lead','project','activity') AS unsupported,
 NOT COALESCE(CASE a.entity_type
 WHEN 'contact' THEN EXISTS (SELECT 1 FROM contact r WHERE r.id=a.entity_id)
 WHEN 'company' THEN EXISTS (SELECT 1 FROM company r WHERE r.id=a.entity_id)
 WHEN 'deal' THEN EXISTS (SELECT 1 FROM deal r WHERE r.id=a.entity_id)
 WHEN 'lead' THEN EXISTS (SELECT 1 FROM lead r WHERE r.id=a.entity_id)
 WHEN 'project' THEN EXISTS (SELECT 1 FROM project r WHERE r.id=a.entity_id)
 WHEN 'activity' THEN EXISTS (SELECT 1 FROM activity r WHERE r.id=a.entity_id)
 END,false) AS gone
 FROM attachment a WHERE a.owner_unkeyed AND a.id > $1 ORDER BY a.id LIMIT $2 FOR UPDATE
 ) , judged AS (
 SELECT p.*, p.unsupported OR (p.gone AND (
 EXISTS (SELECT 1 FROM deal_room_document d WHERE d.attachment_id=p.id)
 OR EXISTS (SELECT 1 FROM deal_room_thread t WHERE t.attachment_id=p.id))) AS needs_review FROM page p
 ), dropped AS (
 DELETE FROM attachment a USING judged p WHERE a.id=p.id AND p.gone AND NOT p.needs_review RETURNING a.id
 ), keyed AS (
 UPDATE attachment a SET owner_unkeyed=false FROM judged p WHERE a.id=p.id AND NOT p.gone AND NOT p.needs_review RETURNING a.id
 ) SELECT id, needs_review FROM judged ORDER BY id`, after, limit)
	if err != nil {
		return AttachmentOwnerPage{}, fmt.Errorf("activities: keying attachment owners: %w", err)
	}
	defer rows.Close()
	var out AttachmentOwnerPage
	for rows.Next() {
		var id ids.UUID
		var unsupported bool
		if err := rows.Scan(&id, &unsupported); err != nil {
			return AttachmentOwnerPage{}, err
		}
		out.IDs = append(out.IDs, id)
		if unsupported {
			out.NeedsReview = append(out.NeedsReview, id)
		}
	}
	return out, rows.Err()
}

// EstimateUnkeyedAttachmentOwners counts a bounded slice for update progress.
func EstimateUnkeyedAttachmentOwners(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM attachment WHERE owner_unkeyed LIMIT $1) remaining`, maximum+1).Scan(&count)
	return min(count, maximum), count > maximum, err
}
