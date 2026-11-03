// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// "How many people inside the account carry this deal?", asked of many deals
// at once, for the Account coverage section. It runs EngagedStakeholders'
// predicate over a set, so the deal page and the list agree on who counts.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SeatCount is one deal's committee as a reader sees it: the seats named,
// the seats engaged within the window, and whether a seat was withheld. A
// withheld committee makes Engaged unsafe to judge, for ChampionCover's
// reason.
type SeatCount struct {
	Named    int
	Engaged  int
	Withheld bool
}

// EngagedSeatCounts answers the committee of each given deal. A deal absent
// from the result has no live seat the reader may see and none withheld.
func EngagedSeatCounts(ctx context.Context, tx pgx.Tx, dealIDs []ids.UUID, now time.Time) (map[ids.UUID]SeatCount, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	out := make(map[ids.UUID]SeatCount, len(dealIDs))
	if len(dealIDs) == 0 {
		return out, nil
	}
	seats, err := StakeholderSeats(ctx, tx, dealIDs)
	if err != nil {
		return nil, err
	}
	for deal, named := range seats {
		out[deal] = SeatCount{Named: len(named)}
	}
	engaged, err := engagedSeatCounts(ctx, tx, dealIDs, now)
	if err != nil {
		return nil, err
	}
	for deal, count := range engaged {
		seat := out[deal]
		seat.Engaged = count
		out[deal] = seat
	}
	withheld, err := withheldSeats(ctx, tx, dealIDs)
	if err != nil {
		return nil, err
	}
	for deal := range withheld {
		seat := out[deal]
		seat.Withheld = true
		out[deal] = seat
	}
	return out, nil
}

func engagedSeatCounts(ctx context.Context, tx pgx.Tx, dealIDs []ids.UUID, now time.Time) (map[ids.UUID]int, error) {
	windowStart := now.AddDate(0, 0, -healthEngagementWindowDays)
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	dealsPos := arg(dealIDs)
	windowPos := arg(windowStart)
	bound, err := edgeBound(ctx, "r", arg)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT r.deal_id, count(DISTINCT r.contact_id) FROM relationship r
		WHERE r.kind = 'deal_stakeholder' AND r.deal_id = ANY($%[1]d) AND r.archived_at IS NULL
		  AND (%[3]s)
		  AND EXISTS (
			SELECT 1 FROM activity a
			JOIN activity_link l ON l.activity_id = a.id AND l.contact_id = r.contact_id
			WHERE %[4]s AND a.archived_at IS NULL`+auth.AudienceWorkspaceOnly("a")+`
			  AND %[2]s AND a.direction = 'inbound')
		  AND EXISTS (
			SELECT 1 FROM activity a
			JOIN activity_link l ON l.activity_id = a.id AND l.contact_id = r.contact_id
			WHERE %[4]s AND a.archived_at IS NULL`+auth.AudienceWorkspaceOnly("a")+`
			  AND %[2]s AND a.direction = 'outbound')
		GROUP BY r.deal_id`, dealsPos, occurredday.FromSQL("a", fmt.Sprintf("$%d", windowPos)), bound, healthInteraction), args...)
	if err != nil {
		return nil, fmt.Errorf("deals: counting the engaged seats on a set of deals: %w", err)
	}
	defer rows.Close()
	out := make(map[ids.UUID]int, len(dealIDs))
	for rows.Next() {
		var deal ids.UUID
		var count int
		if err := rows.Scan(&deal, &count); err != nil {
			return nil, fmt.Errorf("deals: reading a deal's engaged seat count: %w", err)
		}
		out[deal] = count
	}
	return out, rows.Err()
}
