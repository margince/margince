// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
)

// groupedHits answers a grouped page one type at a time, so a type the time
// budget stops costs the page that type alone. Each type runs as its own
// statement under its own savepoint in the one read transaction: the first
// cancelled statement aborts a transaction that has no savepoint to return to.
//
// The page has one deadline, the handle's budget from now, and each lane gets
// what is left of it. A handle nobody bounded has no budget to run out of, so
// its lanes run bare and any error fails the page.
func (s *Store) groupedHits(ctx context.Context, tx pgx.Tx, in Input, query string, types []string, shape groupedShape) (hits []Hit, timedOut []string, err error) {
	budget := s.db.Budget()
	deadline := time.Now().Add(budget)
	for _, entity := range laneOrder(types) {
		statement, args, admitted, err := groupedLaneStatement(ctx, in, query, entity, shape)
		if err != nil {
			return nil, nil, err
		}
		if !admitted {
			continue
		}
		var laneHits []Hit
		run := func(pgx.Tx) (err error) {
			laneHits, err = rank(ctx, tx, statement, args)
			return err
		}
		if budget <= 0 {
			if err := run(tx); err != nil {
				return nil, nil, err
			}
			hits = append(hits, laneHits...)
			continue
		}
		left := time.Until(deadline)
		if left < time.Millisecond {
			timedOut = append(timedOut, entity)
			continue
		}
		switch err := database.WithDegradableSavepoint(ctx, tx, left, run); {
		case err == nil:
			hits = append(hits, laneHits...)
		case laneTimedOut(err):
			timedOut = append(timedOut, entity)
		default:
			return nil, nil, err
		}
	}
	// One list per type, as the single statement ordered them.
	slices.SortStableFunc(hits, func(a, b Hit) int { return cmp.Compare(a.Type, b.Type) })
	return hits, timedOut, nil
}

// groupedLaneStatement builds the grouped statement of one type, with its own
// argument list: a placeholder no SQL references fails the whole statement.
// admitted is false when object RBAC refuses the type, which is an empty lane
// and not a slow one.
func groupedLaneStatement(ctx context.Context, in Input, query, entity string, shape groupedShape) (statement string, args []any, admitted bool, err error) {
	arg := func(v any) int { args = append(args, v); return len(args) }
	statement, err = rankedStatement(ctx, in, query, []string{entity}, shape, arg)
	return statement, args, statement != "", err
}

// laneOrder puts the types that rank a bounded sample of their rows (mail) last,
// so the types that answer in milliseconds leave the sample the most of the
// page's time.
func laneOrder(types []string) []string {
	sampled := func(entity string) bool {
		branch, found := branchFor(entity)
		return found && branch.rankSample > 0
	}
	return slices.SortedStableFunc(slices.Values(types), func(a, b string) int {
		return cmp.Compare(boolRank(sampled(a)), boolRank(sampled(b)))
	})
}

func boolRank(b bool) int {
	if b {
		return 1
	}
	return 0
}

// laneTimedOut is whether a lane failed only because the budget stopped it.
// WithDegradableSavepoint joins a failed rollback to the lane's own error, and
// a rollback that failed leaves the transaction unusable, so that fails the
// page.
func laneTimedOut(err error) bool {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		parts := joined.Unwrap()
		return len(parts) > 0 && !slices.ContainsFunc(parts, func(part error) bool { return !laneTimedOut(part) })
	}
	var tooBroad *QueryTooBroadError
	return errors.As(err, &tooBroad)
}
