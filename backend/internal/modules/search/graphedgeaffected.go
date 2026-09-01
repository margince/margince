// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

// Which colleague-to-contact pairs a change can touch.
//
// Its own file because it is its own question, and the hard half of it: the
// fold in graphedge.go recomputes whatever it is given, and everything that can
// go silently wrong goes wrong HERE, in the naming. A pair this misses keeps a
// row that its evidence no longer supports, and nothing downstream can notice.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// pair is one (user, person) key.
type pair struct {
	user   ids.UUID
	person ids.UUID
}

// affectedPairs resolves which edges the named activities can affect: every
// (user, person) combination appearing on their participant rows, plus every
// EXISTING edge either end of which is still on those activities.
//
// The second arm is the RELINK case, and it is the reason this is not simply a
// self-join. A relink repoints a participant row before the activity.updated
// event is consumed, so the pair the activity used to belong to cannot be named
// from the rows at all — the contact is no longer among them. Its colleague
// still is, and every edge that colleague holds is refolded from the base
// tables, at which point the one that lost its evidence is deleted. Without
// this arm the stale edge stands until the nightly RebuildEdges, which is a
// colleague being recommended for an introduction they can no longer make.
//
// affectedContactPairs closes the same gap the same way for the contact↔contact
// projection. Both ride this one entry point, so a relink that is honest in one
// and stale in the other is the disagreement worth not having.
//
// It is a WIDER target than the rows alone, and deliberately: over-including a
// pair costs a refold that writes back what was already there, while omitting
// one leaves a claim nobody can see is false. The lookup is served by
// idx_graph_edge_user and idx_graph_edge_person, and recomputePairs folds the
// whole target set in one statement rather than one per pair.
func affectedPairs(ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID) ([]pair, error) {
	rows, err := tx.Query(ctx, `
		WITH present_users AS (
		    SELECT DISTINCT user_id
		      FROM activity_participant
		     WHERE activity_id = ANY($1) AND user_id IS NOT NULL
		),
		present_people AS (
		    SELECT DISTINCT person_id
		      FROM activity_participant
		     WHERE activity_id = ANY($1) AND person_id IS NOT NULL
		)
		SELECT DISTINCT u.user_id, p.person_id
		  FROM activity_participant u
		  JOIN activity_participant p ON p.activity_id = u.activity_id
		 WHERE u.activity_id = ANY($1)
		   AND u.user_id IS NOT NULL
		   AND p.person_id IS NOT NULL
		 UNION
		SELECT e.user_id, e.person_id
		  FROM graph_interaction_edge e
		 WHERE e.user_id IN (SELECT user_id FROM present_users)
		    OR e.person_id IN (SELECT person_id FROM present_people)`, activityIDs)
	if err != nil {
		return nil, fmt.Errorf("search: resolving the pairs an activity touches: %w", err)
	}
	defer rows.Close()
	var out []pair
	for rows.Next() {
		var pr pair
		if err := rows.Scan(&pr.user, &pr.person); err != nil {
			return nil, err
		}
		out = append(out, pr)
	}
	return out, rows.Err()
}
