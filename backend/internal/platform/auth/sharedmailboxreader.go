// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// sharedMailboxReaderColumn is the reader-team member the fragment below
// matches; a caller states which members count with a test on it.
const sharedMailboxReaderColumn = "shared_reader.user_id"

// sharedMailboxReaderSQL is "a shared mailbox imported this activity, and a
// live member of its live reader team passes members". The read rules and their
// existence twin all compose it.
// members is a predicate on sharedMailboxReaderColumn.
func sharedMailboxReaderSQL(alias, members string) string {
	return fmt.Sprintf(`EXISTS (SELECT 1 FROM capture_import shared_import
	  JOIN team reader_team ON reader_team.archived_at IS NULL
	  JOIN capture_connection shared_box ON shared_box.id = shared_import.connection_id AND %[4]s
	  JOIN team_membership shared_reader ON shared_reader.team_id = reader_team.id
	  JOIN app_user shared_seat ON shared_seat.id = shared_reader.user_id AND %[3]s
	 WHERE shared_import.activity_id = %[1]s.id AND shared_import.user_id IS NULL AND %[2]s)`,
		alias, members, workforce.LiveMemberSQL("shared_seat"), SharedMailboxOfTeamSQL("shared_box", "reader_team.id"))
}

// SharedMailboxOfTeamSQL is "connection alias is a shared mailbox the team
// reads", whatever its status: a disconnected mailbox's history stays with its
// readers. Every question about which mailboxes a team reads uses it.
func SharedMailboxOfTeamSQL(alias, team string) string {
	return alias + ".user_id IS NULL AND " + alias + ".reader_team_id = " + team
}

// sharedMailboxReaderInSQL is the arm for one member set.
func sharedMailboxReaderInSQL(alias, members string) string {
	return sharedMailboxReaderSQL(alias, sharedMailboxReaderColumn+" IN ("+members+")")
}

// heldByOnlySharedMailboxesSQL is a row only shared mailboxes imported, one of
// them under its held posture. A mailbox set to shared opened it to everyone.
func heldByOnlySharedMailboxesSQL(alias string) string {
	return fmt.Sprintf(`(EXISTS (SELECT 1 FROM capture_import shared_only WHERE shared_only.activity_id = %[1]s.id
	     AND shared_only.user_id IS NULL AND shared_only.posture_at_import IS DISTINCT FROM 'shared')
	   AND NOT EXISTS (SELECT 1 FROM capture_import seat_import WHERE seat_import.activity_id = %[1]s.id AND seat_import.user_id IS NOT NULL))`, alias)
}
