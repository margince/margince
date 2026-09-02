// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// HOW MANY promises each named person has already missed.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// OverdueLoad is how many open, already-due tasks one person holds.
type OverdueLoad struct {
	// OwnerID is zero for the open overdue tasks nobody is assigned.
	OwnerID ids.UUID
	Overdue int
}

// overdueLoadSQL counts open tasks past their due moment, per assignee.
//
// The task predicate is the one listActivitiesFilter builds for OpenAndDueBy,
// restated here because this is an aggregate rather than a page and cannot go
// through that builder. It matches term for term — kind, done-ness, a due
// moment, strictly before the instant — and `< $` is the half of it most worth
// keeping: the bound a caller passes is the END of a day, so `<=` would count a
// task due at exactly tomorrow midnight as late today.
//
// A count over an unbounded page rather than a paged read, and that is the
// reason this exists: the lane reading tasks for one person's day is capped at
// twelve, so counting its rows would report every busy rep as holding exactly
// twelve and a board built that way would say the whole team is equally loaded.
//
// DISCOVER-gated, matching the list this counts. The board publishes a number
// and no content, and a task's existence under a record the reader may open is
// what discover already admits.
const overdueLoadSQL = `
	SELECT COALESCE(a.assignee_id, '00000000-0000-0000-0000-000000000000'::uuid), count(*)
	  FROM activity a
	 WHERE a.kind = 'task' AND NOT a.is_done
	   AND a.archived_at IS NULL
	   AND a.due_at IS NOT NULL AND a.due_at < $%[1]d
	   AND %[2]s
	 GROUP BY 1`

// OverdueLoadByAssignee counts each person's open tasks already past due.
func (s *Store) OverdueLoadByAssignee(ctx context.Context, asOf time.Time) ([]OverdueLoad, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	var load []OverdueLoad
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		args := []any{}
		arg := func(v any) int { args = append(args, v); return len(args) }
		instant := arg(asOf)
		scope, err := auth.ActivityDiscoverClause(ctx, "a", arg)
		if err != nil {
			return err
		}
		if scope == "" {
			scope = scopeUnbounded
		}
		rows, err := tx.Query(ctx, fmt.Sprintf(overdueLoadSQL, instant, scope), args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		load = []OverdueLoad{}
		for rows.Next() {
			var row OverdueLoad
			if err := rows.Scan(&row.OwnerID, &row.Overdue); err != nil {
				return err
			}
			load = append(load, row)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("activities: counting overdue tasks per assignee: %w", err)
	}
	return load, nil
}
