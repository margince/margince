// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package briefs

// A deal the rep dismissed, come back.
//
// WHAT THE SUPPRESSION RULE MAKES TRUE. briefCandidates holds a dismissed deal
// out of every later queue until a linked activity occurs after the mark. So a
// deal that reappears is always a deal the rep waved away and that has since
// moved — never one the ranker simply changed its mind about. That is what
// makes the sentence honest: "you dismissed this, and here is the thing that
// happened since" is not a guess, it is the rule that put the deal back.
//
// It also bounds what the sentence may claim. Only an ACTIVITY can return a
// dismissed deal, so a line naming anything else — an offer expiring, a stage
// moving — would describe a reason that cannot be why the deal is here. Those
// are real facts and a richer "since then" is worth building, but it is a
// change to the suppression rule first and a change to this sentence second.
// Shipping the sentence alone would produce a line that can never fire.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// dealLineage is why one candidate is back: the day the rep dismissed it, and
// the activity that re-qualified it.
type dealLineage struct {
	dismissedOn  time.Time
	returnedWith time.Time
}

// briefLineage reads, for every candidate, whether it is a returning dismissal.
//
// ONE query for the whole rank rather than one per deal. The candidate loop
// already runs up to three statements per deal; a fourth would make an N-query
// loop a 4N one for an answer the whole set can be asked for at once.
//
// A deal with no dismissal, or one dismissed and never re-qualified, simply has
// no row here — the map is the answer, and its absence is the ordinary case.
func briefLineage(
	ctx context.Context, tx pgx.Tx, userID ids.UUID, order []ids.UUID, now time.Time,
) (map[ids.UUID]dealLineage, error) {
	out := make(map[ids.UUID]dealLineage)
	if len(order) == 0 {
		return out, nil
	}
	// The day is derived from the DISMISSAL's own instant, in the same zone the
	// run's local_day is stamped in. Not br.local_day: that is the day the
	// brief was FOR, and a rep who opens Tuesday's brief on Wednesday and waves
	// a deal away is told "flagged Tuesday, you dismissed it" about a Wednesday
	// act. The sentence says what they did, so it is dated when they did it.
	zone, err := identity.TimezoneOf(ctx, tx)
	if err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return nil, fmt.Errorf("brief: the installation timezone %q does not resolve: %w", zone, err)
	}

	// The most recent mark per deal, whatever it is — then lineage only when
	// that mark is a dismissal.
	//
	// Filtering to dismissals BEFORE picking the latest would cite an obsolete
	// one: a deal dismissed, returned, then acted on has a dismissal in its
	// history, but the act is what the rep last said about it and the card must
	// not reopen an argument they already closed.
	//
	// `bi.state <> 'new'` is kept deliberately: it is what idx_brief_item_deal
	// is partial on, so dropping it for readability would fall off the index
	// this query is shaped around.
	rows, err := tx.Query(ctx, `
		WITH last_mark AS (
			SELECT DISTINCT ON (bi.deal_id)
			       bi.deal_id, bi.state, bi.state_at
			  FROM brief_item bi
			  JOIN brief_run br ON br.id = bi.brief_run_id
			 WHERE bi.deal_id = ANY($1)
			   AND br.user_id = $2
			   AND bi.state <> 'new'
			 ORDER BY bi.deal_id, bi.state_at DESC
		)
		SELECT m.deal_id, m.state_at, MIN(a.occurred_at)
		  FROM last_mark m
		  JOIN activity_link l ON l.deal_id = m.deal_id
		  JOIN activity a ON a.id = l.activity_id
		 WHERE m.state = 'dismissed'
		   AND a.archived_at IS NULL
		   AND a.occurred_at > m.state_at
		   -- Not after the run's own instant. A future-dated activity has not
		   -- happened yet, and the candidate query bounds itself the same way,
		   -- so the two agree about which deals are returning — disagreement
		   -- would put a deal back with no line explaining it.
		   AND a.occurred_at <= $3
		 GROUP BY m.deal_id, m.state_at`,
		order, userID, now.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var dealID ids.UUID
		var dismissedAt, returnedWith time.Time
		if err := rows.Scan(&dealID, &dismissedAt, &returnedWith); err != nil {
			return nil, err
		}
		local := dismissedAt.In(loc)
		out[dealID] = dealLineage{
			dismissedOn:  time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC),
			returnedWith: returnedWith,
		}
	}
	return out, rows.Err()
}
