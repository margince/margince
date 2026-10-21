// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import "fmt"

// EffectiveWorklistPinsSQL gives each reader's bounded current pins, shared by work and claim admission.
func EffectiveWorklistPinsSQL(arg func(any) int) string {
	return fmt.Sprintf(`(SELECT reader_id, source, row_id FROM (
 SELECT reader_id, source, row_id, row_number() OVER (
 PARTITION BY reader_id ORDER BY pinned_at DESC, source, row_id) AS newness
 FROM worklist_pin) ranked WHERE newness <= $%d)`, arg(50))
}
