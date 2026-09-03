// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Which of these contacts still has money resting on them.
//
// The question the other direction from Stakeholders: that one asks who sits on
// a deal, this asks whose silence costs something. The attention feed's decay
// lane needs it to tell a lapsed champion from a lapsed cc — the two read
// identically today, and a rep who cannot tell them apart stops reading the
// lane.
//
// Batched over the caller's whole candidate set rather than asked per person.
// The lane derives at most decayCandidateCap contacts, and a per-person read
// would be the N+1 the feed's own rules forbid.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// OpenDealPeople answers which of the given contacts sit on an open deal the
// caller can see.
//
// It carries BOTH admissions, because they answer different questions and
// neither implies the other. The edge grant asks whether this caller may read
// stakeholder pairs at all — a seat is an edge, and knowing a deal does not
// license learning who is on it. The deal row scope asks WHICH deals count:
// without it the answer would leak the existence of a deal through a contact
// the caller can read, which is the side door the edge gate alone leaves open.
//
// A contact absent from the answer is one with no open deal OR one whose deal
// this caller cannot see, and the caller cannot tell those apart. That is the
// same answer every other row-scoped read gives, and it is the right one here:
// the alternative discloses the deal by the shape of the refusal.
func OpenDealPeople(ctx context.Context, tx pgx.Tx, people []ids.PersonID) (map[ids.UUID]bool, error) {
	if len(people) == 0 {
		return map[ids.UUID]bool{}, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	peoplePos := arg(people)
	bound, err := edgeBound(ctx, "r", arg)
	if err != nil {
		return nil, err
	}
	scope, err := auth.ScopeClauseFor(ctx, dealTable, "d", arg)
	if err != nil {
		return nil, err
	}
	visible := predicateAlways
	if scope != "" {
		visible = scope
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT DISTINCT r.person_id
		  FROM relationship r
		  JOIN deal d ON d.id = r.deal_id AND d.archived_at IS NULL
		 WHERE r.kind = 'deal_stakeholder'
		   AND r.person_id = ANY($%d)
		   AND r.archived_at IS NULL
		   AND d.status = 'open'
		   AND (%s)
		   AND (%s)`, peoplePos, bound, visible), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[ids.UUID]bool, len(people))
	for rows.Next() {
		var person ids.UUID
		if err := rows.Scan(&person); err != nil {
			return nil, err
		}
		out[person] = true
	}
	return out, rows.Err()
}
