// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package threadverdict

import (
	"fmt"
	"strings"
)

// WorkspaceAudienceSQL applies thread holds while preserving explicit internal-mail admission.
func WorkspaceAudienceSQL(alias string) string {
	return fmt.Sprintf(`(%[1]s.audience = 'workspace' AND
 (%[1]s.internal_mail_authorization IS NOT NULL OR %[2]s))`, alias, UnheldSQL(alias))
}

// UnheldSQL is true while no importing seat holds the thread of activity
// alias. A hold lands before the audience recompute does, so a reader of the
// audience column asks this too.
func UnheldSQL(alias string) string {
	return noImporterVerdictSQL(alias, HoldsSQL("thread_hold.status"))
}

// SettledUnheldSQL is UnheldSQL that also refuses a thread an importing seat
// still waits on (HoldsMessages). Until that verdict lands, nobody knows
// whether the thread is confidential.
func SettledUnheldSQL(alias string) string {
	return noImporterVerdictSQL(alias, HoldsMessagesSQL("thread_hold.status"))
}

func noImporterVerdictSQL(alias, status string) string {
	return fmt.Sprintf(`NOT EXISTS (
 SELECT 1 FROM capture_import thread_hold_import
 JOIN capture_thread_verdict thread_hold ON thread_hold.user_id=thread_hold_import.user_id
 AND thread_hold.thread_key=%[1]s.thread_key
 WHERE thread_hold_import.activity_id=%[1]s.id AND %[2]s)`, alias, status)
}

// BurstKeyPrefix starts the ledger key of a chat burst. The capture sink refuses
// every record whose thread key starts with it. A mail thread root is picked by
// its sender, and so none can equal a burst key.
const BurstKeyPrefix = "chat-burst:"

// BurstKey is the ledger key of the chat burst with this id.
func BurstKey(burst string) string { return BurstKeyPrefix + burst }

// BurstOfKey returns the burst id a ledger key names, and false for a mail key.
func BurstOfKey(key string) (string, bool) { return strings.CutPrefix(key, BurstKeyPrefix) }

// BurstKeySQL is BurstKey over a SQL uuid expression.
func BurstKeySQL(burst string) string {
	return fmt.Sprintf("('%s' || %s::text)", BurstKeyPrefix, burst)
}

// LedgerKeySQL names the ledger key one import row is judged under: its burst
// for a chat message, its thread key for everything else. Only the audience
// derivation reads it; the reader-side thread-hold checks stay on thread keys.
func LedgerKeySQL(importAlias, activityAlias string) string {
	return fmt.Sprintf("coalesce(%s, %s.thread_key)", BurstKeySQL(importAlias+".burst_id"), activityAlias)
}

// AnySeatHoldsSQL is true while any seat holds the thread of activity alias,
// or still waits on its verdict. One seat's hold protects the whole thread.
func AnySeatHoldsSQL(alias string) string {
	return fmt.Sprintf(`EXISTS (SELECT 1 FROM capture_thread_verdict any_seat
 WHERE any_seat.thread_key = %[1]s.thread_key AND %[1]s.thread_key <> ''
 AND %[2]s)`, alias, HoldsMessagesSQL("any_seat.status"))
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
