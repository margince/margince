// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"fmt"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// MeetingResponsibilitySQL binds work to a supported host, participant or booking.
// Importing a record or owning its calendar does not establish attendance.
// Arguments are compile-time SQL expressions, never request text.
func MeetingResponsibilitySQL(meeting, seat string) string {
	return fmt.Sprintf(`((%[4]s = %[2]s AND NOT starts_with(%[1]s.captured_by, 'connector:'))
	   OR EXISTS (SELECT 1 FROM activity_participant attending
	               WHERE attending.activity_id = %[1]s.id AND attending.user_id = %[2]s
	                 AND attending.role IN ('organizer', 'attendee')
	                 AND %[3]s)
	   OR EXISTS (SELECT 1 FROM meeting_invitation booked
	               WHERE booked.activity_id = %[1]s.id AND booked.host_user_id = %[2]s))`,
		meeting, seat, auth.ParticipantStandsSQL("attending", meeting), KnownMeetingHostSQL(meeting))
}

// UnclaimedMeetingSQL is a meeting no seat answers for by that same evidence.
func UnclaimedMeetingSQL(meeting string) string {
	return fmt.Sprintf(`((%[3]s IS NULL OR starts_with(%[1]s.captured_by, 'connector:'))
	   AND NOT EXISTS (SELECT 1 FROM activity_participant attending
	               WHERE attending.activity_id = %[1]s.id AND attending.user_id IS NOT NULL
	                 AND attending.role IN ('organizer', 'attendee')
	                 AND %[2]s)
	   AND NOT EXISTS (SELECT 1 FROM meeting_invitation booked WHERE booked.activity_id = %[1]s.id))`,
		meeting, auth.ParticipantStandsSQL("attending", meeting), KnownMeetingHostSQL(meeting))
}

// MeetingCreditSQL renders "this seat is credited with this meeting" for the
// figures that count meetings per colleague. It is the attendance rule above,
// narrowed to evidence the seat took part (creditedAttendanceSQL). Each
// credited seat counts the meeting; a team counts it once, as one row.
func MeetingCreditSQL(meeting, seat string) string {
	return meetingCreditSQL(meeting, meeting+".host_user_id", seat)
}

// MeetingCreditedSeatsSQL renders a row source of the seats MeetingCreditSQL
// credits, as `credit(id)`, for a caller that narrows them to a population.
func MeetingCreditedSeatsSQL(meeting string) string {
	return meetingCreditedSeatsSQL(meeting, meeting+".host_user_id")
}

// MeetingCreditedSeatsAtChangeSQL is MeetingCreditedSeatsSQL for a status
// change. It reads the host the change recorded. A later edit of the host
// then does not move credit for a meeting already held. Where legacy holds, the
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
	     UNION SELECT seen.user_id FROM activity_participant seen WHERE seen.activity_id = %[1]s.id AND %[4]s
	     UNION SELECT hosted.host_user_id FROM meeting_invitation hosted WHERE hosted.activity_id = %[1]s.id) seat(id)
	    WHERE seat.id IS NOT NULL AND %[3]s) credit(id)`, meeting, host, meetingCreditSQL(meeting, host, "seat.id"),
		auth.ParticipantStandsSQL("seen", meeting))
}

// MeetingCreditedSeatSQL renders the one seat a meeting is listed under where
// a figure groups by colleague. In order: the human-logged host, the organizer,
// the seat that booked it, and the first guest who accepted. Each of them is
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
	       AND %[2]s AND %[4]s ORDER BY attending.user_id LIMIT 1),
	   (SELECT booked.host_user_id FROM meeting_invitation booked
	     WHERE booked.activity_id = %[1]s.id ORDER BY booked.host_user_id LIMIT 1),
	   (SELECT attending.user_id FROM activity_participant attending
	     WHERE attending.activity_id = %[1]s.id AND attending.user_id IS NOT NULL
	       AND %[2]s AND %[4]s ORDER BY attending.user_id LIMIT 1))`,
		meeting, creditedAttendanceSQL("attending", meeting), knownHostExpressionSQL(meeting, host), auth.ParticipantStandsSQL("attending", meeting))
}

func meetingCreditSQL(meeting, host, seat string) string {
	return fmt.Sprintf(`((%[4]s = %[2]s AND NOT starts_with(%[1]s.captured_by, 'connector:'))
	   OR EXISTS (SELECT 1 FROM activity_participant attending
	               WHERE attending.activity_id = %[1]s.id AND attending.user_id = %[2]s
	                 AND %[3]s AND %[5]s)
	   OR EXISTS (SELECT 1 FROM meeting_invitation booked
	               WHERE booked.activity_id = %[1]s.id AND booked.host_user_id = %[2]s))`,
		meeting, seat, creditedAttendanceSQL("attending", meeting), knownHostExpressionSQL(meeting, host), auth.ParticipantStandsSQL("attending", meeting))
}

// creditedAttendanceSQL is one participant row that proves its seat took part.
// On a meeting a human logged, any organizer or attendee row counts: it is the
// logger's own statement. On a calendar's meeting, an attendee counts once they
// accepted, and the organizer unless they answered otherwise. A calendar that
// lists the organizer apart from the guests states no answer for them. Each
// caller also asks auth.ParticipantStandsSQL of the row.
func creditedAttendanceSQL(participant, meeting string) string {
	return fmt.Sprintf(`%[1]s.role IN ('organizer', 'attendee')
	   AND (NOT starts_with(%[2]s.captured_by, 'connector:')
	     OR %[1]s.response = 'accepted'
	     OR (%[1]s.role = 'organizer' AND %[1]s.response IS NULL))`, participant, meeting)
}

// ExplicitMeetingHostEvidence records a supplied host in the append-only creation audit.
const ExplicitMeetingHostEvidence = "explicit_meeting_host"

// KnownMeetingHostSQL withholds an importer's default unless source attribution or an explicit host write supports it.
func KnownMeetingHostSQL(meeting string) string {
	return knownHostExpressionSQL(meeting, meeting+".host_user_id")
}

func knownHostExpressionSQL(meeting, host string) string {
	return fmt.Sprintf(`(CASE WHEN (%[2]s) IS NULL THEN NULL
 WHEN starts_with(coalesce(%[1]s.source_system,''),'%[3]s')
 AND %[1]s.captured_by = 'human:' || (%[2]s)::text
 AND %[1]s.source_author_id IS DISTINCT FROM (%[2]s)
 AND NOT EXISTS (SELECT 1 FROM audit_log host_statement
     WHERE host_statement.entity_type='activity' AND host_statement.entity_id=%[1]s.id
       AND host_statement.action='create'
       AND host_statement.evidence->>'%[4]s'=(%[2]s)::text)
 THEN NULL ELSE %[2]s END)`, meeting, host, provenance.ReservedSourceSystemPrefix, ExplicitMeetingHostEvidence)
}
