// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Asking whether anybody INSIDE each account is carrying the deal, for the
// deals that reached the morning queue.
//
// Its own file because this lane fact costs a second database read after the
// candidate sweep, and because the judgement in noChampionOf is a privacy rule
// rather than a rendering detail: three inputs arrive and only one of them may
// become a sentence a rep acts on.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// championCover asks the deals module whether anybody inside each account is
// carrying the deal.
//
// Over the SURVIVORS, in one batch, after the candidate sweep has filtered.
// The queue weighs up to fifty drifting deals and the per-deal coverage read
// is two statements each, so asking it row by row would put a hundred round
// trips on the morning assembly for one boolean per row.
//
// A nil pool means this seam was built without one, which is how the lane's
// own tests bind it: the champion question goes unanswered and every row says
// nothing, rather than the lane failing whole.
func (a attentionAtRisk) championCover(
	ctx context.Context, candidates []agents.SlippingDeal, now time.Time,
) (map[ids.UUID]deals.ChampionCover, error) {
	if a.pool == nil || len(candidates) == 0 {
		return map[ids.UUID]deals.ChampionCover{}, nil
	}
	dealIDs := make([]ids.UUID, 0, len(candidates))
	for _, deal := range candidates {
		dealIDs = append(dealIDs, deal.DealID)
	}
	var cover map[ids.UUID]deals.ChampionCover
	err := database.WithWorkspaceTx(ctx, a.pool, func(tx pgx.Tx) error {
		var readErr error
		cover, readErr = deals.ChampionCoverFor(ctx, tx, dealIDs, now)
		return readErr
	})
	if err != nil {
		return nil, err
	}
	return cover, nil
}

// noChampionOf turns the module's two flags into the lane's one nullable fact.
//
// Absent unless the answer is BOTH known and negative. A withheld committee is
// absent because a champion the reader may not read is still a champion, and a
// deal with no seats at all is absent because it has no coverage gap — it has
// no committee. Reporting either as "nobody is carrying this" would put a
// finding on a deal that does not have it, which is the one direction this
// claim must never fail in: a rep told nobody is arguing for their deal goes
// and finds somebody to argue for it.
func noChampionOf(cover map[ids.UUID]deals.ChampionCover, deal ids.UUID) *bool {
	// ABSENT from the map, not its zero value: a deal with no live seats never
	// appears in the answer, and its zero value reads Covered=false —
	// indistinguishable from a committee that genuinely has no champion. Asked
	// the wrong way this claimed nobody was carrying every untouched deal in
	// the pipeline, which is a warning that is always on.
	found, ok := cover[deal]
	if !ok || found.Withheld || found.Covered {
		return nil
	}
	uncovered := true
	return &uncovered
}
