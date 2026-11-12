// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package occurredday

import "fmt"

// BookedStartSQL resolves a planned meeting while completed meetings retain their recorded instant.
func BookedStartSQL(alias string) string {
	return fmt.Sprintf(`(CASE WHEN %[1]s.kind = 'meeting' AND
 (%[1]s.meeting_status IS NULL OR %[1]s.meeting_status = 'booked')
 THEN coalesce(public.resolve_intent_time(%[1]s.meeting_wall, %[1]s.meeting_zone), %[1]s.occurred_at)
 ELSE %[1]s.occurred_at END)`, alias)
}
