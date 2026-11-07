// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import "fmt"

// MeetingSeatDeclinedSQL tests the seat's own stored answer. Arguments are
// compile-time SQL expressions, never request text.
func MeetingSeatDeclinedSQL(meeting, seat string) string {
	return fmt.Sprintf(`EXISTS (SELECT 1 FROM activity_participant declined
 WHERE declined.activity_id=%s.id AND declined.user_id=%s
 AND declined.response='declined')`, meeting, seat)
}

// MeetingHasCalendarRosterSQL distinguishes invitation evidence from legacy
// calendar-owner fallback when deciding whose time a meeting blocks.
func MeetingHasCalendarRosterSQL(meeting string) string {
	return fmt.Sprintf(`EXISTS (SELECT 1 FROM activity_participant roster
 WHERE roster.activity_id=%s.id AND roster.roster_seat IS NOT NULL
 AND roster.role IN ('organizer','attendee'))`, meeting)
}
