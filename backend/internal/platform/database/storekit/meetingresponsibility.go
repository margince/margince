// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"fmt"

	"github.com/margince/margince/backend/internal/platform/auth"
)

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
	                 AND attending.role IN ('organizer', 'attendee')
	                 AND %[3]s)
	   OR EXISTS (SELECT 1 FROM meeting_invitation booked
	               WHERE booked.activity_id = %[1]s.id AND booked.host_user_id = %[2]s))`,
		meeting, seat, auth.ParticipantStandsSQL("attending", meeting))
}

// UnclaimedMeetingSQL is a meeting no seat answers for by that same evidence.
func UnclaimedMeetingSQL(meeting string) string {
	return fmt.Sprintf(`((%[1]s.host_user_id IS NULL OR starts_with(%[1]s.captured_by, 'connector:'))
	   AND NOT EXISTS (SELECT 1 FROM activity_participant attending
	               WHERE attending.activity_id = %[1]s.id AND attending.user_id IS NOT NULL
	                 AND attending.role IN ('organizer', 'attendee')
	                 AND %[2]s)
	   AND NOT EXISTS (SELECT 1 FROM meeting_invitation booked WHERE booked.activity_id = %[1]s.id))`,
		meeting, auth.ParticipantStandsSQL("attending", meeting))
}

// MeetingCreditSQL renders "this seat is credited with this meeting" for the
// figures that count meetings per person: meetings held in analytics, the
// meeting-conversion report and the weekly scorecard. It is the attendance
// rule above, narrowed to evidence that the seat actually took part. A calendar
// guest counts only once they accepted; an invitation left unanswered,
// declined or tentative is not attendance. The organizer, a host a human
// logged, a guest a human logged and the seat that booked it all count.
//
// Every credited seat counts the meeting in their own figures. A team or a
// workspace counts it once, because the meeting is one row in those figures.
func MeetingCreditSQL(meeting, seat string) string {
	return meetingCreditSQL(meeting, meeting+".host_user_id", seat)
}

// MeetingCreditedSeatsSQL renders a row source of the seats MeetingCreditSQL
// credits, as `credit(id)`, for a caller that narrows them to a population.
func MeetingCreditedSeatsSQL(meeting string) string {
	return meetingCreditedSeatsSQL(meeting, meeting+".host_user_id")
}

// MeetingCreditedSeatsAtChangeSQL is MeetingCreditedSeatsSQL for a status
// change: it reads the host the change recorded, so a later edit of the host
// does not move credit for a meeting already held. Where legacy holds, the
// change recorded nothing and the meeting's own host is read. Only a host a
// human logged counts either way.
func MeetingCreditedSeatsAtChangeSQL(meeting, legacy, recorded string) string {
	return meetingCreditedSeatsSQL(meeting, recordedHostSQL(meeting, legacy, recorded))
}

// MeetingCreditedSeatAtChangeSQL is MeetingCreditedSeatSQL over the host a
// status change recorded, as MeetingCreditedSeatsAtChangeSQL reads it.
func MeetingCreditedSeatAtChangeSQL(meeting, legacy, recorded string) string {
	return meetingCreditedSeatSQL(meeting, recordedHostSQL(meeting, legacy, recorded))
}

func recordedHostSQL(meeting, legacy, recorded string) string {
	return fmt.Sprintf("(CASE WHEN %s THEN %s.host_user_id ELSE %s END)", legacy, meeting, recorded)
}

func meetingCreditedSeatsSQL(meeting, host string) string {
	return fmt.Sprintf(`(SELECT seat.id FROM (
	     SELECT %[2]s
	     UNION SELECT seen.user_id FROM activity_participant seen WHERE seen.activity_id = %[1]s.id
	     UNION SELECT hosted.host_user_id FROM meeting_invitation hosted WHERE hosted.activity_id = %[1]s.id) seat(id)
	    WHERE seat.id IS NOT NULL AND %[3]s) credit(id)`, meeting, host, meetingCreditSQL(meeting, host, "seat.id"))
}

// MeetingCreditedSeatSQL renders the one seat a meeting is listed under where
// a figure groups by person: the human-logged host, then the organizer, then
// the seat that booked it, then the first guest who accepted. Each of them is
// credited by MeetingCreditSQL; this only picks the label for a one-row grain.
// NULL when nobody is credited.
func MeetingCreditedSeatSQL(meeting string) string {
	return meetingCreditedSeatSQL(meeting, meeting+".host_user_id")
}

func meetingCreditedSeatSQL(meeting, host string) string {
	return fmt.Sprintf(`COALESCE(
	   CASE WHEN NOT starts_with(%[1]s.captured_by, 'connector:') THEN %[3]s END,
	   (SELECT attending.user_id FROM activity_participant attending
	     WHERE attending.activity_id = %[1]s.id AND attending.user_id IS NOT NULL AND attending.role = 'organizer'
	       AND %[2]s ORDER BY attending.user_id LIMIT 1),
	   (SELECT booked.host_user_id FROM meeting_invitation booked
	     WHERE booked.activity_id = %[1]s.id ORDER BY booked.host_user_id LIMIT 1),
	   (SELECT attending.user_id FROM activity_participant attending
	     WHERE attending.activity_id = %[1]s.id AND attending.user_id IS NOT NULL
	       AND %[2]s ORDER BY attending.user_id LIMIT 1))`,
		meeting, creditedAttendanceSQL("attending", meeting), host)
}

func meetingCreditSQL(meeting, host, seat string) string {
	return fmt.Sprintf(`((%[4]s = %[2]s AND NOT starts_with(%[1]s.captured_by, 'connector:'))
	   OR EXISTS (SELECT 1 FROM activity_participant attending
	               WHERE attending.activity_id = %[1]s.id AND attending.user_id = %[2]s
	                 AND %[3]s)
	   OR EXISTS (SELECT 1 FROM meeting_invitation booked
	               WHERE booked.activity_id = %[1]s.id AND booked.host_user_id = %[2]s))`,
		meeting, seat, creditedAttendanceSQL("attending", meeting), host)
}

// creditedAttendanceSQL is one participant row that proves its seat took part.
// On a meeting a human logged, any organizer or attendee row counts: it is the
// logger's own statement. On a calendar's meeting, an attendee counts once they
// accepted, and the organizer unless they answered otherwise; a calendar that
// lists the organizer apart from the guests states no answer for them.
func creditedAttendanceSQL(participant, meeting string) string {
	return fmt.Sprintf(`%[1]s.role IN ('organizer', 'attendee')
	   AND (NOT starts_with(%[2]s.captured_by, 'connector:')
	     OR %[1]s.response = 'accepted'
	     OR (%[1]s.role = 'organizer' AND %[1]s.response IS NULL))
	   AND %[3]s`, participant, meeting, auth.ParticipantStandsSQL(participant, meeting))
}
