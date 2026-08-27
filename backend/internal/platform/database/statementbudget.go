// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

// CallerPredicateBudget is how long a statement whose predicate the CALLER
// wrote may hold a connection: a saved filter's tree, an agent's query plan, a
// builder's preview.
//
// One number, because it answers one question and it was answered twice. The
// filter preview and the search plan each declared five seconds with its own
// paragraph arguing for it — two writers of one invariant, each reasoning from
// the same fact and neither able to see the other move. The reason is not a
// property of either surface: somebody is WAITING on the answer, and a glance
// that takes five seconds has already failed at being a glance, exactly as an
// agent still waiting after five seconds has already lost the turn it asked in.
//
// It bounds the plans that cannot be answered at all rather than the ones that
// are merely large — an indexed plan with a page limit answers an SMB-sized
// corpus in milliseconds. A background path that nobody is waiting on wants its
// own ceiling and should say so with its own constant and its own reason.
//
// Held by: TestTheCallerPredicateBudgetIsDeclaredOnce
// (backend/gates/onecallerpredicatebudget_test.go)
const CallerPredicateBudget = 5 * time.Second

// BoundStatement gives every statement that follows it in tx a wall-clock
// ceiling. It is the ONE spelling of "this statement has a budget", for the
// same reason this package owns the workspace GUC: a store that builds its own
// timeout is a store whose ceiling nobody can find by grepping for one name.
//
// It exists for the paths where the CALLER writes the predicate — a saved
// filter's tree, an agent's query plan. Those cannot be bounded by a page
// limit, because the limit bounds the rows returned and not the rows visited: a
// predicate that matches almost nothing still makes the planner walk the table
// to prove it. Without a ceiling the only bound left is how long the caller is
// willing to hold a pool connection, which is not a bound at all.
//
// Parameterized set_config, never a string-built SET LOCAL — the same rule the
// workspace binding above follows, and it holds here even though no budget is a
// caller's value, because the next budget to be threaded through might be.
func BoundStatement(ctx context.Context, tx pgx.Tx, budget time.Duration) error {
	// Postgres reads statement_timeout = 0 as "no timeout", so a budget that
	// rounds to zero milliseconds would silently UNBOUND the statement this call
	// exists to bound — the one outcome worse than never having called it. A
	// sub-millisecond budget is therefore honoured as the shortest ceiling
	// Postgres can express, and a non-positive one is a programming error rather
	// than a request for no ceiling.
	if budget <= 0 {
		return fmt.Errorf("pg: a statement budget of %s bounds nothing; "+
			"pass the caller's own ceiling, and note that Postgres reads zero as no timeout", budget)
	}
	milliseconds := strconv.FormatInt(max(budget.Milliseconds(), 1), 10)
	if _, err := tx.Exec(ctx,
		`SELECT set_config('statement_timeout', $1, true)`, milliseconds); err != nil {
		return fmt.Errorf("pg: bounding the statement to %s: %w", budget, err)
	}
	return nil
}
