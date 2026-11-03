// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// "How many contacts at the account carry this deal?", asked of many deals
// at once, for the Account coverage section. It counts what engagedAmong
// answers, so the deal page, the morning queue and the list agree on who
// counts.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
	// Engagement is read off activity, so the activity grant is asked before
	// a count that would disclose whether anybody wrote.
	for _, object := range []string{"deal", "activity"} {
		if err := auth.Require(ctx, object, principal.ActionRead); err != nil {
			return nil, err
		}
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
	engaged, err := engagedAmong(ctx, tx, dealIDs, now)
	if err != nil {
		return nil, err
	}
	for pair := range engaged {
		seat := out[pair.deal]
		seat.Engaged++
		out[pair.deal] = seat
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
