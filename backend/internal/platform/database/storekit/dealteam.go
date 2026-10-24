// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import "fmt"

// DealTeamSQL is true when user is on the deal's team. The team is its owner,
// the seats assigned to it, and the members of live teams assigned to it.
//
// deal is the deal table's alias and user a uuid expression, both compile-time
// literals or placeholders. Every reader that asks who works a deal composes
// this predicate. So an extension and the composer name the same seats.
func DealTeamSQL(deal, user string) string {
	return fmt.Sprintf(`(coalesce(%[2]s = %[1]s.owner_id, false)
	 OR EXISTS (SELECT 1 FROM record_assignment team_seat
	             WHERE team_seat.deal_id = %[1]s.id AND team_seat.archived_at IS NULL
	               AND team_seat.user_id = %[2]s)
	 OR EXISTS (SELECT 1 FROM record_assignment team_slot
	              JOIN team assigned ON assigned.id = team_slot.team_id AND assigned.archived_at IS NULL
	              JOIN team_membership member ON member.team_id = assigned.id
	             WHERE team_slot.deal_id = %[1]s.id AND team_slot.archived_at IS NULL
	               AND member.user_id = %[2]s))`, deal, user)
}
