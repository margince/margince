// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"strings"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
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

// timelinePageSQL renders the first attempt at the page read: the bounded fold.
func timelinePageSQL(ctx context.Context, in ListActivitiesInput, limit int) (timelinePage, error) {
	return timelinePageAt(ctx, in, limit, FoldBounds(limit + 1)[0])
}

// timelinePageAt renders the page read with the fold bounded to foldRows
// candidates, or unbounded at zero. A collapsed page filters candidates inside
// SeatCopyFrom and pages after it, so the keyset applies to the row shown.
func timelinePageAt(ctx context.Context, in ListActivitiesInput, limit, foldRows int) (timelinePage, error) {
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
		if in.canFoldCalendarMail() {
			candidates += " AND " + InvitationFoldSQL(candidates, content)
		}
		if in.WithoutCopiesOf != nil {
			// The activity joins the candidates so its copies group with it; the
			// group is then left out, so the activity itself is never shown.
			pos := arg(*in.WithoutCopiesOf)
			candidates = sprintf("((%s) OR a.id = $%d)", candidates, pos)
			without = sprintf("NOT ($%d = ANY(copies.copy_ids))", pos)
		}
		keyset, err := timelineKeyset(in, sorted, arg)
		if err != nil {
			return timelinePage{}, err
		}
		bound, err := recencyFoldBound(in, sorted, order, foldRows, arg)
		if err != nil {
			return timelinePage{}, err
		}
		from = SeatCopyFrom(ctx, candidates, content, bound, arg)
		content, trailing, where = SeatCopyReadable, SeatCopyColumns, []string{without}
		if keyset != "" {
			where = append(where, keyset)
		}
	}
	if !in.CollapseSeatCopies {
		// Bound the candidate rows before the fenced content decision.
		from = BoundActivityPage(from, strings.Join(where, " AND "), order, limit+1)
		where = []string{sqlTrue}
	}
	from, content = ActivityContentFrom(from, content)
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

// recencyFoldBound is the bound a newest-first page may fold under. Any other
// order, or a cursor with no timestamp, folds every candidate.
func recencyFoldBound(in ListActivitiesInput, sorted *storekit.ListSort, order string, foldRows int, arg func(any) int) (connector.FoldBound, error) {
	if foldRows == 0 || order != sorted.OrderBy() || !sorted.Orders("occurred_at", true) {
		return connector.FoldBound{}, nil
	}
	bound := connector.FoldBound{Rows: foldRows}
	if in.Cursor == nil || *in.Cursor == "" {
		return bound, nil
	}
	cursor, err := storekit.DecodeCursor("activities", *in.Cursor)
	if err != nil {
		return connector.FoldBound{}, err
	}
	if cursor.SortKey == nil {
		return connector.FoldBound{}, nil
	}
	// The keyset applies to the row shown, whose copies can happen up to a second after it.
	bound.Ceiling = sprintf("(date_trunc('second', $%d::timestamptz) + interval '1 second')", arg(*cursor.SortKey))
	return bound, nil
}
