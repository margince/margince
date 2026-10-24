// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import "fmt"

// MeetingResponsibilitySQL renders "this seat answers for this meeting", over
// the activity row `meeting` and the seat expression `seat`. The Worklist
// lanes, the follow-up reminder, the weekly figures and plan capacity read it.
//
// Three kinds of evidence each record taking part. The first is the host a
// human logged the meeting with. The second is a provider-recorded organizer
// or attendee row. The third is an invitation this seat sent through the
// booking doors. A captured meeting's host column says only whose calendar it
// came from, so on a connector row it is no evidence. Importing a copy, being
// copied on mail about it, or filing it is none of these.
//
// Both arguments are SQL the caller renders: a table alias and a placeholder
// or column. Neither is ever request text.
func MeetingResponsibilitySQL(meeting, seat string) string {
	return fmt.Sprintf(`((%[1]s.host_user_id = %[2]s AND NOT starts_with(%[1]s.captured_by, 'connector:'))
	   OR EXISTS (SELECT 1 FROM activity_participant attending
	               WHERE attending.activity_id = %[1]s.id AND attending.user_id = %[2]s
	                 AND attending.role IN ('organizer', 'attendee'))
	   OR EXISTS (SELECT 1 FROM meeting_invitation booked
	               WHERE booked.activity_id = %[1]s.id AND booked.host_user_id = %[2]s))`, meeting, seat)
}

// UnclaimedMeetingSQL is a meeting no seat answers for by that same evidence.
func UnclaimedMeetingSQL(meeting string) string {
	return fmt.Sprintf(`((%[1]s.host_user_id IS NULL OR starts_with(%[1]s.captured_by, 'connector:'))
	   AND NOT EXISTS (SELECT 1 FROM activity_participant attending
	               WHERE attending.activity_id = %[1]s.id AND attending.user_id IS NOT NULL
	                 AND attending.role IN ('organizer', 'attendee'))
	   AND NOT EXISTS (SELECT 1 FROM meeting_invitation booked WHERE booked.activity_id = %[1]s.id))`, meeting)
}
