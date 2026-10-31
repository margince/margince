// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// What a meeting on a lead proves: lead evidence, lead scoring and the lead
// status ladder all read it here. Capture writes a calendar meeting with no
// status, so an unrecorded meeting is a booking until someone records more.

// meetingProof is the engagement trigger a meeting with this status proves.
// status is "" when the meeting has none.
func meetingProof(status string) (PromoteTrigger, bool) {
	switch {
	case status == "held":
		return TriggerMeetingHeld, true
	case relstrength.MeetingStatusCounts(status):
		return TriggerMeetingBooked, true
	}
	return "", false
}

// meetingProofSQL is meetingProof for the meeting row alias, NULL when it
// proves nothing. alias is a compile-time literal.
func meetingProofSQL(alias string) string {
	return `(CASE WHEN ` + alias + `.meeting_status = 'held' THEN '` + string(TriggerMeetingHeld) + `'
	              WHEN ` + relstrength.MeetingCountsSQL(alias) + ` THEN '` + string(TriggerMeetingBooked) + `' END)`
}

// meetingProves is whether a meeting with this status proves any engagement.
func meetingProves(status string) bool {
	_, proves := meetingProof(status)
	return proves
}
