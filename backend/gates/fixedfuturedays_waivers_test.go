// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import "github.com/margince/margince/backend/internal/shared/gatekit"

// The reasons a fixed day after fixedDateHorizon is safe in an integration
// test. Each waiver below names one of them.
const (
	dayUnderPinnedClock = "the test pins the product's clock near this day, so the writer compares the day with that " +
		"instant and never with today"
	dayIsCalendarFact = "the day is a calendar fact under test, such as a zone shift or a quarter boundary, and no " +
		"writer compares it with today"
	dayIsOpaquePayload = "the day is payload the test hashes, echoes, counts or stores raw; no writer compares it " +
		"with today"
	dayStoredEitherSide = "the writer takes this day in the past as readily as in the future, so the test reads " +
		"the same after the day passes"
	dayBoundsARead = "the day bounds a read or filter over rows seeded at fixed days, and nothing compares it with " +
		"today"
)

// fixedFutureDayWaivers ratifies each fixed day an integration test may spell
// after fixedDateHorizon, keyed by file and day.
var fixedFutureDayWaivers = gatekit.Waive(map[string]string{})
