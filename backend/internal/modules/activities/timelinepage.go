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
		candidates, without := strings.Join(where, " AND "), sqlTrue
		if in.WithoutCopiesOf != nil {
			// The activity joins the candidates so its copies group with it; the
			// group is then left out, so the activity itself is never shown.
			pos := arg(*in.WithoutCopiesOf)
			candidates = sprintf("((%s) OR a.id = $%d)", candidates, pos)
			without = sprintf("NOT ($%d = ANY(copies.copy_ids))", pos)
		}
		from = SeatCopyFrom(ctx, candidates, content, arg)
		content, trailing, where = SeatCopyReadable, SeatCopyColumns, []string{without}
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
