// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// What a meeting on a lead proves: lead evidence, lead scoring and the lead
// status ladder all read it here. Capture writes a calendar meeting with no
// status, so an unrecorded meeting is a booking until someone records more.
// A lead who declined the invitation stays filed on it and proves nothing.

// meetingProof is the engagement trigger a meeting with this status proves.
// status is "" when the meeting has none; declined is leadDeclinedSQL.
func meetingProof(status string, declined bool) (PromoteTrigger, bool) {
	switch {
	case declined:
		return "", false
	case status == "held":
		return TriggerMeetingHeld, true
	case relstrength.MeetingStatusCounts(status):
		return TriggerMeetingBooked, true
	}
	return "", false
}

// meetingProves is whether a meeting with this status proves any engagement.
func meetingProves(status string, declined bool) bool {
	_, proves := meetingProof(status, declined)
	return proves
}

// meetingProofSQL is meetingProof for the meeting row alias and the lead
// leadRef names, NULL when it proves nothing. Both are compile-time literals.
func meetingProofSQL(alias, leadRef string) string {
	return `(CASE WHEN ` + leadDeclinedSQL(leadRef, alias) + ` THEN NULL
	              WHEN ` + alias + `.meeting_status = 'held' THEN '` + string(TriggerMeetingHeld) + `'
	              WHEN ` + relstrength.MeetingCountsSQL(alias) + ` THEN '` + string(TriggerMeetingBooked) + `' END)`
}

// leadDeclinedSQL is whether the lead leadRef names answered the calendar
// invitation of the activity alias with a no.
func leadDeclinedSQL(leadRef, alias string) string {
	return `EXISTS (SELECT 1 FROM lead decliner JOIN activity_participant answer ON answer.activity_id = ` + alias + `.id
	         WHERE decliner.id = ` + leadRef + ` AND answer.response = 'declined'
	           AND ` + storekit.LeadEmailMatchClause("decliner", "answer.address") + `)`
}
