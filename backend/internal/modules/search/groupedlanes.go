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
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// groupedPage answers a grouped request, whole or partial. It owns the page's
// one deadline, the handle's budget from now. The lanes and what follows them
// share it. None starts a fresh statement budget.
func (s *Store) groupedPage(ctx context.Context, tx pgx.Tx, in Input, query string, types []string, shape groupedShape) (Page, error) {
	var deadline time.Time
	if budget := s.db.Budget(); budget > 0 {
		deadline = time.Now().Add(budget)
	}
	lanes, err := s.groupedHits(ctx, tx, in, query, types, shape, deadline)
	if err != nil {
		return Page{}, err
	}
	page := shape.pageWith(lanes.hits, lanes.timedOut)
	if len(lanes.answered) == 0 && len(lanes.timedOut) == 0 {
		// Object RBAC denied every type: nothing was read, so nothing follows.
		return page, nil
	}
	return page, s.finishPage(ctx, tx, in, query, lanes.answered, &page, deadline)
}

// groupedLanes is what the lanes of one grouped page found. answered holds the
// types whose ranking finished, timedOut the types the budget stopped; a type
// object RBAC denied is in neither.
type groupedLanes struct {
	hits               []Hit
	answered, timedOut []string
}

// groupedHits answers a grouped page one type at a time, so a type the time
// budget stops costs the page that type alone. Each type runs as its own
// statement under its own savepoint in the one read transaction. Without the
// savepoint, the first cancelled statement would abort the transaction.
//
// Each lane gets what is left of deadline. A handle nobody bounded has no
// budget to run out of, so its lanes run bare and any error fails the page.
func (s *Store) groupedHits(ctx context.Context, tx pgx.Tx, in Input, query string, types []string, shape groupedShape, deadline time.Time) (groupedLanes, error) {
	var lanes groupedLanes
	bounded := s.db.Budget() > 0
	for _, entity := range laneOrder(types) {
		statement, args, admitted, err := groupedLaneStatement(ctx, in, query, entity, shape)
		if err != nil {
			return groupedLanes{}, err
		}
		if !admitted {
			continue
		}
		var laneHits []Hit
		run := func(pgx.Tx) (err error) {
			laneHits, err = rank(ctx, tx, statement, args)
			return err
		}
		if !bounded {
			if err := run(tx); err != nil {
				return groupedLanes{}, err
			}
			lanes.hits = append(lanes.hits, laneHits...)
			lanes.answered = append(lanes.answered, entity)
			continue
		}
		left := time.Until(deadline)
		if left < time.Millisecond {
			lanes.timedOut = append(lanes.timedOut, entity)
			continue
		}
		switch err := database.WithDegradableSavepoint(ctx, tx, left, run); {
		case err == nil:
			lanes.hits = append(lanes.hits, laneHits...)
			lanes.answered = append(lanes.answered, entity)
		case laneTimedOut(err):
			lanes.timedOut = append(lanes.timedOut, entity)
		default:
			return groupedLanes{}, err
		}
	}
	// One list per type, as the single statement ordered them.
	slices.SortStableFunc(lanes.hits, func(a, b Hit) int { return cmp.Compare(a.Type, b.Type) })
	return lanes, nil
}

// nearMatchesBefore is nearMatches inside what is left of the page's deadline.
// A page that lost a type to the budget is still an answer. A lookup the
// budget stops gives up its suggestions and leaves the page whole. Any other
// failure still fails the page. A zero deadline is a ranked page.
func nearMatchesBefore(ctx context.Context, tx pgx.Tx, deadline time.Time, types []string, query string) ([]NearMatch, error) {
	if deadline.IsZero() {
		return nearMatches(ctx, tx, types, query)
	}
	left := time.Until(deadline)
	if left < time.Millisecond {
		return nil, nil
	}
	var near []NearMatch
	err := database.WithDegradableSavepoint(ctx, tx, left, func(pgx.Tx) (err error) {
		near, err = nearMatches(ctx, tx, types, query)
		return err
	})
	switch {
	case err == nil:
		return near, nil
	case ctx.Err() == nil && everyPart(err, storekit.IsStatementTimeout):
		return nil, nil
	default:
		return nil, err
	}
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

// laneOrder puts the types that rank a bounded sample of their rows (mail) last.
// The types that answer in milliseconds then leave the sample most of the time.
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
	return everyPart(err, func(part error) bool {
		var tooBroad *QueryTooBroadError
		return errors.As(part, &tooBroad) && storekit.IsStatementTimeout(part)
	})
}

// everyPart is whether leaf holds for each error of a join, or for err itself.
func everyPart(err error, leaf func(error) bool) bool {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		parts := joined.Unwrap()
		return len(parts) > 0 && !slices.ContainsFunc(parts, func(part error) bool { return !everyPart(part, leaf) })
	}
	return leaf(err)
}
