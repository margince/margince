// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import "fmt"

// MeetingSeatDeclinedSQL tests the seat's own stored answer. Arguments are
// compile-time SQL expressions, never request text.
func MeetingSeatDeclinedSQL(meeting, seat string) string {
	return fmt.Sprintf(`EXISTS (SELECT 1 FROM activity_participant declined
 WHERE declined.activity_id=%s.id AND declined.user_id=%s
 AND declined.response='declined' AND %s)`, meeting, seat, ParticipantStandsSQL("declined", meeting))
}
