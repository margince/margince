// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import "fmt"

// MeetingResponsibilitySQL renders "this seat answers for this meeting", over
// the activity row `meeting` and the seat expression `seat`. The Worklist
// lanes, the follow-up reminder, the weekly figures and plan capacity read it.
//
// Three kinds of evidence, each a record of taking part: the host the meeting
// names (whose calendar it is on, or who logged it), a provider-recorded
// organizer or attendee row, and an invitation this seat sent through the
// booking doors. Importing a copy of it, being copied on mail about it, or
// filing it is none of these, so none of them makes the meeting anybody's work.
//
// Both arguments are SQL the caller renders: a table alias and a placeholder
// or column. Neither is ever request text.
func MeetingResponsibilitySQL(meeting, seat string) string {
	return fmt.Sprintf(`(%[1]s.host_user_id = %[2]s
	   OR EXISTS (SELECT 1 FROM activity_participant attending
	               WHERE attending.activity_id = %[1]s.id AND attending.user_id = %[2]s
	                 AND attending.role IN ('organizer', 'attendee'))
	   OR EXISTS (SELECT 1 FROM meeting_invitation booked
	               WHERE booked.activity_id = %[1]s.id AND booked.host_user_id = %[2]s))`, meeting, seat)
}

// UnclaimedMeetingSQL is a meeting no seat answers for by that same evidence.
func UnclaimedMeetingSQL(meeting string) string {
	return fmt.Sprintf(`(%[1]s.host_user_id IS NULL
	   AND NOT EXISTS (SELECT 1 FROM activity_participant attending
	               WHERE attending.activity_id = %[1]s.id AND attending.user_id IS NOT NULL
	                 AND attending.role IN ('organizer', 'attendee'))
	   AND NOT EXISTS (SELECT 1 FROM meeting_invitation booked WHERE booked.activity_id = %[1]s.id))`, meeting)
}
