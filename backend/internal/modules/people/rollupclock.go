// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

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
// So the date is bound by the caller, from ONE clock. A package var rather than
// a field, matching leadSLAClock beside it: both are read by free functions on
// a transaction rather than by methods, and threading a clock through them would
// change every signature to say what one line says here.

import "time"

// rollupClock is the clock the as-of date comes from. Overridden in tests, and
// nowhere else — a second production writer would be the second clock again.
var rollupClock = time.Now

// rollupAsOf is the date the conversion is asked for.
//
// UTC, deliberately. The rate table is keyed by date and the alternative is the
// server's local zone, which would make the same installation convert
// differently depending on where its process happens to run.
func rollupAsOf() time.Time {
	return rollupClock().UTC().Truncate(24 * time.Hour)
}
