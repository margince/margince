// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The instant the open-pipeline rollup converts at.
//
// Company360 samples Go's clock and honours an injected one, so this read binds
// its instant from the same kind of clock instead of asking Postgres. Two
// readers of one account's pipeline could otherwise select different FX rates in
// the same response.
//
// The rollup is asked for the installation's day at that instant. The SQL cuts
// the day from it (occurredday.TodaySQL), so no reader here picks a calendar day
// itself.
//
// So the caller binds the instant from one clock, the store's. Each company
// read samples it once and pins it on its context. The sort expression that
// reads it is a free function with no store in hand.

import (
	"context"
	"time"
)

type rollupAsOfKey struct{}

// withRollupAsOf pins the instant, by this store's clock, for everything built
// from ctx. The count, the total and the sort then agree on one FX day.
func (s *Store) withRollupAsOf(ctx context.Context) context.Context {
	return context.WithValue(ctx, rollupAsOfKey{}, s.clock())
}

// rollupAsOf is the pinned instant, or now for a caller that pinned none.
func rollupAsOf(ctx context.Context) time.Time {
	if pinned, ok := ctx.Value(rollupAsOfKey{}).(time.Time); ok {
		return pinned
	}
	return time.Now()
}
