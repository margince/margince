// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Which of a set of deals carries no open next step.
//
// The at-risk sweep's goal names three signals — no activity in 14+ days,
// stakeholders gone quiet, and MISSING NEXT STEPS. The first two had tools that
// answered them; the third had none, so an agent asked the question the goal
// told it to ask and got no instrument. It was covered by attaching
// review_commitments, which answers "what did somebody promise" — a different
// question, and one that says nothing about a deal nobody has promised anything
// about.
//
// Absence is the answer here, which is why it needs its own read: the deal list
// carries what a deal HAS, and no field on it says what it lacks.

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// dealsWithNoOpenNextStep answers which of the given deals carry none.
//
// WHAT COUNTS AS AN OPEN NEXT STEP, decided here because absence has edge cases
// that decide the answer and this is the kind of predicate that ends up spelled
// three different ways:
//
//   - A task DUE IN THE PAST but still open counts. It is a step somebody
//     agreed to take and has not; that it is late is a different complaint, and
//     the sweep's own momentum reasons already carry lateness. Reading an
//     overdue task as no-step would report "nobody has agreed a next step" about
//     a deal where somebody plainly did.
//   - A task ASSIGNED TO SOMEBODY WHO HAS LEFT counts. The deal has a step;
//     whether its owner is still here is a question about staffing, and
//     answering it inside this predicate would make one signal quietly report
//     two.
//   - A task on the COMPANY rather than the deal does NOT count. This tool
//     answers about a deal, and a step filed against the account is not a step
//     on this opportunity — counting it would silence the signal for every deal
//     at a company where anything at all is open.
//   - An ARCHIVED task does not count, for the reason every read here excludes
//     archived rows: it is retired, and a retired step is not a step.
//
// One statement over all the candidate ids rather than a probe per deal: the
// sweep already reads its candidates in two bounded queries, and a third that
// grew with the set would make the lane's cost quadratic in a page.
func dealsWithNoOpenNextStep(
	ctx context.Context, pool *pgxpool.Pool, candidates []ids.UUID,
) (map[ids.UUID]bool, error) {
	none := map[ids.UUID]bool{}
	if len(candidates) == 0 {
		return none, nil
	}
	for _, id := range candidates {
		none[id] = true
	}
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT DISTINCT l.deal_id
			  FROM activity a
			  JOIN activity_link l ON l.activity_id = a.id
			 WHERE l.deal_id = ANY($1)
			   AND a.kind = 'task'
			   AND a.is_done = false
			   AND a.archived_at IS NULL`, candidates)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var dealID ids.UUID
			if err := rows.Scan(&dealID); err != nil {
				return err
			}
			delete(none, dealID)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return none, nil
}
