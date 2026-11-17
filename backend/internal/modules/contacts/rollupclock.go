// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The date the open-pipeline rollup converts at.
//
// It used to be the database's CURRENT_DATE, which made two clocks: company360
// samples Go's and honours an injected one, and this read asked Postgres. Two
// readers of one account's pipeline could therefore select different FX rates in
// the same response — a whole day apart at midnight in the database's zone.
//
// The consequence is rare and, worse, unreproducible: reported once, impossible
// to reproduce, closed as the reporter's mistake. A money figure that is wrong
// unreproducibly is the worst kind, because the product's own defence against
// the report is indistinguishable from the bug.
//
// So the date is bound by the caller, from ONE clock: the store's. Each company
// read samples it once and pins the day on its context, because the sort
// expression that reads it is a free function with no store in hand.

import (
	"context"
	"time"
)

type rollupAsOfKey struct{}

// withRollupAsOf pins the day this store's clock reads now for everything
// built from ctx, so the count, the total and the sort agree on one FX day.
func (s *Store) withRollupAsOf(ctx context.Context) context.Context {
	return context.WithValue(ctx, rollupAsOfKey{}, rollupDay(s.clock()))
}

// rollupAsOf is the pinned day, or today for a caller that pinned none.
func rollupAsOf(ctx context.Context) time.Time {
	if pinned, ok := ctx.Value(rollupAsOfKey{}).(time.Time); ok {
		return pinned
	}
	return rollupDay(time.Now())
}

// rollupDay is the date the conversion is asked for at instant now.
//
// UTC, deliberately. The rate table is keyed by date and the alternative is the
// server's local zone, which would make the same installation convert
// differently depending on where its process happens to run.
func rollupDay(now time.Time) time.Time {
	return now.UTC().Truncate(24 * time.Hour)
}
