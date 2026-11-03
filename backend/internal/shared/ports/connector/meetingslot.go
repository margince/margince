// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

import "time"

// MeetingSlot is where a calendar now puts a meeting, as MeetingMover takes it.
type MeetingSlot struct {
	Start    time.Time
	Duration *int
	// AllDay says Start only carries a date (meetingmap.AllDayStart), not a
	// moment, so a meeting moved to or from a whole day changes precision too.
	AllDay bool
	// Agenda is the fingerprint of the invite's description as the calendar
	// now sends it. Empty when the caller did not read it, which keeps the stored one.
	Agenda string
}
