// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"strings"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// timelinePage is the one statement ListActivitiesTx runs, with what its scan needs.
type timelinePage struct {
	sql       string
	args      []any
	sorted    *storekit.ListSort
	collapsed bool
}

// copyDest is where the scan puts SeatCopyColumns, when the page selects them.
func (p timelinePage) copyDest(copies *SeatCopies) []any {
	if !p.collapsed {
		return nil
	}
	return copies.dest()
}

// timelinePageSQL renders the page read. A collapsed page filters candidates
// inside SeatCopyFrom and pages after it, so the keyset applies to the row shown.
func timelinePageSQL(ctx context.Context, in ListActivitiesInput, limit int) (timelinePage, error) {
	filtered := in
	if in.CollapseSeatCopies {
		filtered.Cursor = nil
	}
	where, content, sorted, args, err := listActivitiesFilter(ctx, filtered)
	if err != nil {
		return timelinePage{}, err
	}
	arg := func(v any) int { args = append(args, v); return len(args) }
	order := sorted.OrderBy()
	if in.RequestReviewAsOf != nil || in.RequestCurrentAsOf != nil {
		order = requestPriorityOrder
	}
	from, trailing := "activity a", ""
	if in.CollapseSeatCopies {
		from = SeatCopyFrom(ctx, strings.Join(where, " AND "), content, arg)
		content, trailing, where = SeatCopyReadable, SeatCopyColumns, []string{sqlTrue}
		keyset, err := timelineKeyset(in, sorted, arg)
		if err != nil {
			return timelinePage{}, err
		}
		if keyset != "" {
			where = append(where, keyset)
		}
	}
	columns, err := activityColumns(ctx, content, arg)
	if err != nil {
		return timelinePage{}, err
	}
	return timelinePage{
		sql: `SELECT ` + columns + sorted.CursorKeySuffix() + trailing +
			` FROM ` + from + ` WHERE ` + strings.Join(where, " AND ") +
			order + sprintf(" LIMIT %d", limit+1),
		args: args, sorted: sorted, collapsed: in.CollapseSeatCopies,
	}, nil
}
