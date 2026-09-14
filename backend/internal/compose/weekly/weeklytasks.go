// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"fmt"

	"github.com/margince/margince/backend/internal/modules/privacy"
)

// A reopening clears done_at. The earliest later patch retains the stamp at
// the reporting boundary; JSON null must remain null rather than falling back
// to today's completion. Callers also enforce the task's live content scope.
func taskCompletionAtSQL(alias, cutoff, verbs string) string {
	return fmt.Sprintf(`(coalesce((SELECT change.before->'done_at' FROM audit_log change
  WHERE change.entity_type='activity' AND change.entity_id=%s.id
    AND change.occurred_at >= %s AND change.before ? 'done_at'
    AND %s
  ORDER BY change.occurred_at,change.id LIMIT 1),to_jsonb(%s.done_at)) #>> '{}')::timestamptz`,
		alias, cutoff, privacy.UnscrubbedImageSQL("change", verbs), alias)
}
