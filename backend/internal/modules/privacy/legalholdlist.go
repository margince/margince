// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The controller's view of what a litigation hold is preserving.
//
// A read rather than a write, and so it lives here beside the restricted-list
// read it mirrors: privacy owns neither the five record tables nor the writes
// to them — each owning module does, and compose binds them — but the question
// "what is held" is a compliance question and belongs with the rest of them.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// HeldRecord is one record a hold is preserving, named well enough to
// recognise. Why, when and by whom are in the audit row the write committed —
// no column carries them, so nothing here can disagree with it.
type HeldRecord struct {
	EntityType string
	RecordID   ids.UUID
	Label      string
	CreatedAt  time.Time
}

// HeldPage is one page of held records.
type HeldPage struct {
	Records    []HeldRecord
	NextCursor string
	HasMore    bool
}

// heldRecordsSQL unions the five holdable tables.
//
// Every arm reads `legal_hold AND archived_at IS NULL`: an archived record is
// still held and still un-erasable, but it is not what a controller is asked to
// review, and a list mixing the two would read as live work. Each names its own
// label column — a company has a display_name, a deal a name — and the lead's
// is coalesced because a lead may legitimately have neither name nor email, and
// a blank row in this list would be a held record nobody can identify.
const heldRecordsSQL = `
	  SELECT 'contact' AS entity_type, id, full_name AS label, created_at FROM contact WHERE legal_hold AND archived_at IS NULL
	UNION ALL
	  SELECT 'company', id, display_name, created_at FROM company WHERE legal_hold AND archived_at IS NULL
	UNION ALL
	  SELECT 'deal', id, name, created_at FROM deal WHERE legal_hold AND archived_at IS NULL
	UNION ALL
	  SELECT 'lead', id, coalesce(full_name, email, company_name, id::text), created_at FROM lead WHERE legal_hold AND archived_at IS NULL
	UNION ALL
	  SELECT 'project', id, name, created_at FROM project WHERE legal_hold AND archived_at IS NULL`

// ListLegalHolds answers which records a hold is preserving, oldest first.
func ListLegalHolds(ctx context.Context, db *database.DB, cursor *string, limit *int) (HeldPage, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return HeldPage{}, fmt.Errorf("human-only compliance read: %w", apperrors.ErrPermissionDenied)
	}
	if err := auth.Require(ctx, retentionPolicyObject, principal.ActionRead); err != nil {
		return HeldPage{}, err
	}
	size := storekit.ClampLimit(limit)
	where, args, err := heldListWhere(cursor, size)
	if err != nil {
		return HeldPage{}, err
	}
	var page HeldPage
	err = db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT entity_type, id, label, created_at
			  FROM (`+heldRecordsSQL+`) held
			 WHERE TRUE `+where+`
			 ORDER BY created_at ASC, id ASC
			 LIMIT $`+fmt.Sprint(len(args)), args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r HeldRecord
			if err := rows.Scan(&r.EntityType, &r.RecordID, &r.Label, &r.CreatedAt); err != nil {
				return err
			}
			page.Records = append(page.Records, r)
		}
		return rows.Err()
	})
	if err != nil {
		return HeldPage{}, err
	}
	if len(page.Records) > size {
		page.Records = page.Records[:size]
		last := page.Records[len(page.Records)-1]
		next, err := storekit.EncodeCursor(last.CreatedAt, last.RecordID)
		if err != nil {
			return HeldPage{}, err
		}
		page.NextCursor = next
		page.HasMore = true
	}
	return page, nil
}

// heldListWhere renders the keyset predicate and the LIMIT arg (limit+1, so the
// page knows whether another follows).
func heldListWhere(cursor *string, size int) (string, []any, error) {
	where := ""
	args := []any{}
	if cursor != nil && *cursor != "" {
		c, err := storekit.DecodeCursor(*cursor)
		if err != nil {
			return "", nil, err
		}
		args = append(args, c.CreatedAt, c.ID)
		where = " AND (created_at, id) > ($1, $2)"
	}
	args = append(args, size+1)
	return where, args, nil
}
