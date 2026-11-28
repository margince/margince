// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingoutcome

import "strings"

const userWorkActorSQL = `actor_type IN ('human','agent')`

// BodyAuthoredSQL recognizes an earlier body edit by a signed user or agent.
// Calendar rendering and capture writes are not evidence of rep notes.
const BodyAuthoredSQL = `(edit.` + userWorkActorSQL + ` AND (edit.after ? 'body' OR edit.before ? 'body'))`

// UserWorkSQL keeps calendar repair from replacing work a member or agent recorded.
func UserWorkSQL(alias string) string { return alias + "." + userWorkActorSQL }

// LegacyManualNotes says whether body is the older manual meeting notes field.
func LegacyManualNotes(sourceSystem *string, hasInvitation bool, capturedBy *string) bool {
	return sourceSystem == nil && !hasInvitation && (capturedBy == nil || !strings.HasPrefix(*capturedBy, "connector:"))
}

// ManualNotesSQL is LegacyManualNotes for a trusted activity alias.
func ManualNotesSQL(a string) string {
	return "(" + a + ".source_system IS NULL AND coalesce(" + a + ".captured_by,'') NOT LIKE 'connector:%' AND NOT EXISTS (SELECT 1 FROM meeting_invitation notes_invitation WHERE notes_invitation.activity_id=" + a + ".id AND notes_invitation.status<>'erased'))"
}

// ReadNotesSQL resolves stored notes and preserves an earlier rep-authored
// calendar body. Provider descriptions alone never become internal notes.
func ReadNotesSQL(a string) string {
	return "CASE WHEN " + a + ".kind='meeting' THEN CASE WHEN " + a + ".meeting_notes IS NOT NULL THEN " + a + ".meeting_notes WHEN NOT " + ManualNotesSQL(a) + " AND EXISTS (SELECT 1 FROM audit_log edit WHERE edit.entity_type='activity' AND edit.entity_id=" + a + ".id AND " + BodyAuthoredSQL + ") THEN " + a + ".body END END"
}

// RecordBodySQL selects a meeting's rep notes, or another activity's body.
func RecordBodySQL(a string) string {
	return "CASE WHEN " + a + ".kind='meeting' THEN coalesce((" + ReadNotesSQL(a) + "), CASE WHEN " + ManualNotesSQL(a) + " THEN " + a + ".body END) ELSE " + a + ".body END"
}
