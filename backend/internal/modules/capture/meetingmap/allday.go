// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingmap

import (
	"strings"
	"time"
)

// AllDayStart anchors an all-day date (YYYY-MM-DD) at noon UTC, where every
// door stores a day entry (occurredday.Anchor).
//
// An all-day date is calendar-local with no timezone; reading it as midnight
// UTC lands it on the PREVIOUS day for any zone west of UTC. Noon keeps the
// intended calendar date across the whole ±12h range of real offsets, absent a
// per-event timezone. Exported because every calendar provider has all-day
// events and the off-by-one is the same one in each.
func AllDayStart(date string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(date))
	if err != nil {
		return time.Time{}, false
	}
	return t.Add(12 * time.Hour).UTC(), true
}
