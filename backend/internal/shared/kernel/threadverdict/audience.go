// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package threadverdict

import "fmt"

// WorkspaceAudienceSQL applies thread holds while preserving explicit internal-mail admission.
func WorkspaceAudienceSQL(alias string) string {
	return fmt.Sprintf(`(%[1]s.audience = 'workspace' AND
 (%[1]s.internal_mail_authorization IS NOT NULL OR NOT EXISTS (
 SELECT 1 FROM capture_import thread_hold_import
 JOIN capture_thread_verdict thread_hold ON thread_hold.user_id=thread_hold_import.user_id
 AND thread_hold.thread_key=%[1]s.thread_key
 WHERE thread_hold_import.activity_id=%[1]s.id AND %[2]s)))`, alias, HoldsSQL("thread_hold.status"))
}

// ReasonForKind maps a holding decision onto the activity’s audience reason.
func ReasonForKind(kind string) string {
	if kind == "explicitly_confidential" {
		return "verdict"
	}
	return kind
}

// ReasonSQL uses the same reason mapping during a set-based history repair.
func ReasonSQL(kind string) string {
	return "CASE WHEN " + kind + "='explicitly_confidential' THEN 'verdict' ELSE coalesce(" + kind + ",'') END"
}
