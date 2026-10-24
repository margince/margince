// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "fmt"

// firstMoveSQL selects the first audit row that moved a record off its
// colleague: `holder` is who it named before. No row means the current column
// is still the first holder.
//
// The commitment-task history and the automatic-work report share it, so they
// agree on who a record named first.
func firstMoveSQL(entityType, rowAlias, field, unscrubbed string) string {
	return fmt.Sprintf(`SELECT (h.before->>'%[3]s')::uuid AS holder, h.actor_type, h.passport_id
  FROM audit_log h
 WHERE h.entity_type = '%[1]s' AND h.entity_id = %[2]s.id AND h.before ? '%[3]s' AND (%[4]s)
 ORDER BY h.occurred_at, h.id LIMIT 1`, entityType, rowAlias, field, unscrubbed)
}
