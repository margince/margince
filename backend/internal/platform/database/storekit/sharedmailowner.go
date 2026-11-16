// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SharedMailWorkOwnerSQL is who holds the work in message alias when only
// shared mailboxes captured it, else NULL. Every producer of such work reads
// it. The order: the owner of the other parties' contacts, then of their
// current companies, then the mailboxes' backup owner.
//
// A seat counts only when it can hold work and already reads the mail, so
// routing never widens who reads it. A step naming no seat falls to the next;
// one naming two falls to the backup step. Backup owners that differ name
// nobody. A shared mailbox's own address is never another party.
func SharedMailWorkOwnerSQL(alias string) string {
	only := fmt.Sprintf(`EXISTS (SELECT 1 FROM capture_import shared_import
	   WHERE shared_import.activity_id = %[1]s.id AND shared_import.user_id IS NULL)
	 AND NOT EXISTS (SELECT 1 FROM capture_import seat_import
	   WHERE seat_import.activity_id = %[1]s.id AND seat_import.user_id IS NOT NULL)`, alias)
	parties := fmt.Sprintf(`(SELECT other.contact_id, other.address FROM activity_participant other
	 WHERE other.activity_id = %[1]s.id AND other.role = %[2]s
	   AND NOT EXISTS (SELECT 1 FROM capture_connection own_box
	     WHERE own_box.user_id IS NULL AND lower(own_box.account_label) = lower(other.address))) party`, alias, partyRoleSQL(alias))
	candidates := `SELECT 1 AS step, party_contact.owner_id AS seat FROM ` + parties + `
	   JOIN contact party_contact ON party_contact.id = ` + partyContactSQL + ` AND party_contact.archived_at IS NULL
	 UNION ALL SELECT 2, employer.owner_id FROM ` + parties + `
	   JOIN relationship job ON job.contact_id = ` + partyContactSQL + ` AND ` + employment.LiveCurrentPrimarySQL("job") + `
	   JOIN company employer ON employer.id = job.company_id AND employer.archived_at IS NULL
	 UNION ALL SELECT 3, shared_box.backup_owner_id FROM capture_import shared_import
	   JOIN capture_connection shared_box ON shared_box.id = shared_import.connection_id AND shared_box.user_id IS NULL
	   JOIN team_membership backup_member ON backup_member.team_id = shared_box.reader_team_id
	    AND backup_member.user_id = shared_box.backup_owner_id
	  WHERE shared_import.activity_id = ` + alias + `.id AND shared_import.user_id IS NULL`
	// One join judges every step's seats, so the read check is planned once.
	return fmt.Sprintf(`(SELECT CASE WHEN %[2]s = 1 THEN %[3]s
	   WHEN %[2]s = 0 AND %[4]s = 1 THEN %[5]s
	   WHEN %[6]s = 1 THEN %[7]s END
	 FROM (%[8]s) shared_owner JOIN app_user shared_owner_seat ON shared_owner_seat.id = shared_owner.seat AND %[9]s AND %[10]s
	 WHERE %[1]s)`,
		only, stepSeats(1), stepSeat(1), stepSeats(2), stepSeat(2), stepSeats(3), stepSeat(3), candidates,
		auth.AssigneeEligibleSQL("shared_owner_seat"), auth.SeatMayReadMailSQL(alias, "shared_owner_seat.id"))
}

// stepSeats counts the distinct seats one routing step names; stepSeat is one
// of them, the one when the count is one.
func stepSeats(step int) string {
	return fmt.Sprintf("count(DISTINCT shared_owner_seat.id) FILTER (WHERE shared_owner.step = %d)", step)
}

func stepSeat(step int) string {
	return fmt.Sprintf("min(shared_owner_seat.id::text) FILTER (WHERE shared_owner.step = %d)::uuid", step)
}

// partyContactSQL is the contact of participant `party`: the one capture
// stamped, else the live contact holding its address.
const partyContactSQL = `coalesce(party.contact_id, (SELECT party_email.contact_id FROM contact_email party_email
	  WHERE party_email.email = lower(party.address) AND party_email.archived_at IS NULL
	  ORDER BY party_email.contact_id LIMIT 1))`

// partyRoleSQL is the participant role of the other party on message alias.
func partyRoleSQL(alias string) string {
	return fmt.Sprintf(`(CASE WHEN %s.direction = 'outbound' THEN 'to' ELSE 'from' END)`, alias)
}

// SharedMailWorkOwnerTx is SharedMailWorkOwnerSQL for one live message, or
// zero. sentOnly answers zero unless the shared mailbox sent the message.
func SharedMailWorkOwnerTx(ctx context.Context, tx pgx.Tx, message ids.UUID, sentOnly bool) (ids.UUID, error) {
	var owner *ids.UUID
	err := tx.QueryRow(ctx, `SELECT `+SharedMailWorkOwnerSQL("a")+` FROM activity a
	  WHERE a.id = $1 AND a.archived_at IS NULL AND (NOT $2 OR a.direction = 'outbound')`, message, sentOnly).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && owner == nil) {
		return ids.Nil, nil
	}
	if err != nil {
		return ids.Nil, fmt.Errorf("reading who holds the work of shared mailbox mail: %w", err)
	}
	return *owner, nil
}
