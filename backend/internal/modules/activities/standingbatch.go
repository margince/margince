// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// StandingActivities answers which of these activities still stand for THIS
// caller: live, and within the caller's own discover scope. Ids that are
// archived, outside that scope, or gone are simply absent from the set.
//
// It exists for advice that was STORED and is replayed later. A suggestion
// written days ago cites records that can be archived afterwards, and a
// finding whose record is gone is a card quoting something the reader can no
// longer open. Answering that needs a question about the ROW — is it still
// there — and nothing about its content.
//
// The gate is auth.ActivityDiscoverClause, deliberately, and this is the whole
// reason the probe is separate from EmailSummariesByIDBatch. That reader is
// narrower twice over: it admits only `kind = 'email'`, and it gates on
// ActivityContentClause because it prints a subject and a body preview. A
// caller asking "does this row still stand" through it would read a live
// MEETING as gone, and a live email whose CONTENT this reader may not see as
// gone too — retracting advice that is perfectly valid. Existence is the
// discover question, so it takes the discover gate.
//
// Never an error for an id the caller may not reach: the same rule the summary
// batch follows, because a page enriching many rows must not fail over one the
// reader was never entitled to.
func StandingActivities(
	ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID,
) (map[ids.UUID]bool, error) {
	standing := map[ids.UUID]bool{}
	if len(activityIDs) == 0 {
		return standing, nil
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		// A seat with no activity.read stands no activity, and the caller's
		// page still renders. Same silent narrowing the summary batch gives a
		// denied branch.
		return standing, nil
	}
	args := []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }

	scope, err := auth.ActivityDiscoverClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	where := []string{fmt.Sprintf("a.id = ANY($%d)", arg(activityIDs)), activityLive}
	if scope != "" {
		where = append(where, scope)
	}

	rows, err := tx.Query(ctx,
		"SELECT a.id FROM activity a WHERE "+strings.Join(where, " AND "), args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading which cited activities still stand: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("activities: scanning a standing activity: %w", err)
		}
		standing[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("activities: reading which cited activities still stand: %w", err)
	}
	return standing, nil
}
