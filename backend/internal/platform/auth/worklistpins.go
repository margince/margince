// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import "fmt"

const effectivePinLimit = 50

// EffectiveWorklistPinsSQL gives each reader's bounded current pins, shared by work and claim admission.
func EffectiveWorklistPinsSQL(arg func(any) int) string {
	return effectiveWorklistPinsSQL(fmt.Sprintf("$%d", arg(effectivePinLimit)))
}

// AssigneePinnedTaskClause admits only a task's assignee's effective pin.
// alias names a compile-time activity alias, never request input.
func AssigneePinnedTaskClause(alias string) string {
	return `EXISTS (SELECT 1 FROM ` + effectiveWorklistPinsSQL(fmt.Sprint(effectivePinLimit)) + ` pinned
 WHERE pinned.reader_id=` + alias + `.assignee_id AND pinned.source='task' AND pinned.row_id=` + alias + `.id::text)`
}

func effectiveWorklistPinsSQL(limit string) string {
	return `(SELECT reader_id, source, row_id FROM (
 SELECT reader_id, source, row_id, row_number() OVER (
 PARTITION BY reader_id ORDER BY pinned_at DESC, source, row_id) AS newness
 FROM worklist_pin) ranked WHERE newness <= ` + limit + `)`
}
