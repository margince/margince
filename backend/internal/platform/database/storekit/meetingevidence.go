// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// A source author records history; neither that author nor the importer is
// evidence of attendance. Explicit invitation roles remain evidence.
const importedMeetingEvidenceSQL = `(starts_with(COALESCE(a.source_system,''),'` + provenance.ReservedSourceSystemPrefix + `') OR a.source_author_id IS NOT NULL OR a.source_author_name IS NOT NULL OR starts_with(a.captured_by,'connector:') OR EXISTS (SELECT 1 FROM capture_import origin WHERE origin.activity_id=a.id))`

// MeetingResponsibilityClause admits a seat named by attendance or a direct meeting record.
func MeetingResponsibilityClause(alias, reader string) string {
	return qualifyMeetingSQL(alias, `(EXISTS (SELECT 1 FROM activity_participant attending WHERE attending.activity_id=a.id
 AND attending.user_id=`+reader+` AND attending.role IN ('organizer','attendee'))
 OR EXISTS (SELECT 1 FROM meeting_invitation invitation WHERE invitation.activity_id=a.id AND invitation.host_user_id=`+reader+`)
 OR (NOT `+importedMeetingEvidenceSQL+` AND a.host_user_id=`+reader+`))`)
}

// MeetingHostProjection names the supported organizer, never the capturing account.
func MeetingHostProjection(alias string) string {
	return qualifyMeetingSQL(alias, `CASE WHEN NOT `+importedMeetingEvidenceSQL+` OR EXISTS (SELECT 1 FROM activity_participant attending
 WHERE attending.activity_id=a.id AND attending.user_id=a.host_user_id AND attending.role='organizer')
 OR EXISTS (SELECT 1 FROM meeting_invitation invitation WHERE invitation.activity_id=a.id AND invitation.host_user_id=a.host_user_id)
 THEN a.host_user_id ELSE (SELECT host.user_id FROM activity_participant host
 WHERE host.activity_id=a.id AND host.role='organizer' AND host.user_id IS NOT NULL
 AND NOT EXISTS (SELECT 1 FROM activity_participant other_host WHERE other_host.activity_id=a.id
 AND other_host.role='organizer' AND other_host.user_id IS DISTINCT FROM host.user_id)
 LIMIT 1) END`)
}

// ImportedMeetingClause distinguishes captured or imported history from a direct meeting record.
func ImportedMeetingClause(alias string) string {
	return qualifyMeetingSQL(alias, importedMeetingEvidenceSQL)
}

func qualifyMeetingSQL(alias, clause string) string {
	return strings.ReplaceAll(clause, "a.", pgx.Identifier{alias}.Sanitize()+".")
}
