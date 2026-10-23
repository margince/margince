// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

var importedMeetingSQL = storekit.ImportedMeetingClause("a")

func meetingResponsibilitySQL(reader string) string {
	return storekit.MeetingResponsibilityClause("a", reader)
}
func meetingHostProjectionSQL() string { return storekit.MeetingHostProjection("a") }

func importedMeeting(in LogActivityInput) bool {
	return in.Kind == KindMeeting && (in.Author.AuthorID != nil || in.Author.AuthorName != nil || (in.SourceSystem != nil && provenance.ImporterNamespace(*in.SourceSystem)))
}

func currentMeetingSQL() string {
	return `(NOT ` + importedMeetingSQL + ` OR EXISTS (SELECT 1 FROM activity_participant attendance
 WHERE attendance.activity_id=a.id AND attendance.user_id IS NOT NULL AND attendance.role IN ('organizer','attendee'))
 OR EXISTS (SELECT 1 FROM meeting_invitation invitation WHERE invitation.activity_id=a.id))`
}
