// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
)

// sectionQuery is one section statement. A collapsed one selects
// activities.SeatCopyColumns after the projection.
type sectionQuery struct {
	sql       string
	args      []any
	collapsed bool
}

// readSection runs a section read, caps it at sectionCap and finishes the rows
// the way /activities does, the links of every copy included.
func readSection(ctx context.Context, tx pgx.Tx, q sectionQuery, columns activities.CustomColumns, now time.Time) ([]crmcontracts.Activity, bool, error) {
	rows, err := tx.Query(ctx, q.sql, q.args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := make([]crmcontracts.Activity, 0, sectionCap+1)
	var copies []activities.SeatCopies
	for rows.Next() {
		if !q.collapsed {
			a, err := activities.ScanActivityRow(ctx, rows)
			if err != nil {
				return nil, false, err
			}
			out = append(out, a)
			continue
		}
		a, group, err := activities.ScanSeatCopyRow(ctx, rows)
		if err != nil {
			return nil, false, err
		}
		out, copies = append(out, a), append(copies, group)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(out) > sectionCap
	if hasMore {
		out = out[:sectionCap]
	}
	// After the cap, so the statements count the rows this page returns rather
	// than the extra one read to detect hasMore.
	if err := activities.FinishActivityRows(ctx, tx, columns, out, now); err != nil {
		return nil, false, err
	}
	return out, hasMore, activities.AttachCopyLinks(ctx, tx, out, copies)
}
