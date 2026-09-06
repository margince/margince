// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package briefs

// What the queue before this one said about the same deals.
//
// A daily brief that cannot see its own last edition describes a deal stuck for
// a week exactly as it describes one that arrived overnight. The difference is
// most of what a reader wants, and it is a deterministic fact — a rank written
// into a row before anything read it — so it is served WITH the queue rather
// than fetched by whoever is reading it. That is the ground the dismissal
// lineage is served on too: a record of what happened, not something an agent
// wrote, so a run carrying it is not a loop reading its own output.
//
// It is deliberately NOT a diff, and the absence is the reason. "Not in the
// previous run" and "new" are different statements — a deal is absent because
// it did not rank, which happens to deals that have existed for months — so the
// only claim made here is the one the rows support: this deal stood at that
// position on that day.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// previousRanking is what the run before `day` recorded for this rep: the local
// day it was for, and the position each deal held in it.
//
// A zero day means there is no earlier run at all. That is a different state
// from an earlier run in which a deal did not appear, and both are different
// from a morning where nothing moved — a caller that collapsed them would tell
// a rep on their first day that nothing had changed.
func previousRanking(
	ctx context.Context, tx pgx.Tx, userID ids.UUID, day time.Time,
) (time.Time, map[ids.UUID]int, error) {
	var runID ids.UUID
	var was time.Time
	err := tx.QueryRow(ctx, `
		SELECT id, local_day FROM brief_run
		 WHERE user_id = $1 AND local_day < $2
		 ORDER BY local_day DESC
		 LIMIT 1`, userID, day).Scan(&runID, &was)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, nil, nil
	}
	if err != nil {
		return time.Time{}, nil, fmt.Errorf("briefs: reading the previous run: %w", err)
	}
	// Scoped like every other read of a persisted deal reference: re-checked
	// when it is SERVED rather than trusted from the night it was written. The
	// ranks reach only deals today's read already answered with, so on today's
	// shape this clause refuses nothing — it is here because the rule is
	// uniform, and a read that decided for itself that it was the exception
	// would be the one place the next caller inherits an unscoped door from.
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	runPos := arg(runID)
	scope, err := auth.ScopeClauseFor(ctx, "deal", "d", arg)
	if err != nil {
		return time.Time{}, nil, err
	}
	q := fmt.Sprintf(`
		SELECT bi.deal_id, bi.rank
		  FROM brief_item bi
		  JOIN deal d ON d.id = bi.deal_id
		 WHERE bi.brief_run_id = $%d`, runPos)
	if scope != "" {
		q += " AND " + scope
	}
	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return time.Time{}, nil, fmt.Errorf("briefs: reading the previous ranking: %w", err)
	}
	defer rows.Close()
	ranks := make(map[ids.UUID]int)
	for rows.Next() {
		var dealID ids.UUID
		var rank int
		if err := rows.Scan(&dealID, &rank); err != nil {
			return time.Time{}, nil, fmt.Errorf("briefs: reading the previous ranking: %w", err)
		}
		ranks[dealID] = rank
	}
	if err := rows.Err(); err != nil {
		return time.Time{}, nil, fmt.Errorf("briefs: reading the previous ranking: %w", err)
	}
	return was, ranks, nil
}

// carryPreviousRanks stamps each item with where its deal stood in the previous
// run, and leaves the rest untouched.
func carryPreviousRanks(items []BriefRunItem, ranks map[ids.UUID]int) {
	for i := range items {
		rank, ok := ranks[items[i].DealID]
		if !ok {
			continue
		}
		items[i].PreviousRank = &rank
	}
}
