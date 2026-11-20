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
// A package var rather than a field, matching leadSLAClock beside it. Both are
// read by free functions on a transaction, not by methods. Threading a clock
// through them would change every signature to say what one line says here.

import "time"

// rollupClock is the clock the as-of instant comes from. Overridden in tests,
// and nowhere else; a second production writer would be a second clock.
var rollupClock = time.Now

// rollupAsOf is the instant the conversion is asked for, sampled once per read.
func rollupAsOf() time.Time {
	return rollupClock()
}
