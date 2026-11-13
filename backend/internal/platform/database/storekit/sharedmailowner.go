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
	return fmt.Sprintf(`(SELECT CASE WHEN contact_owner.seats = 1 THEN contact_owner.seat
	   WHEN contact_owner.seats = 0 AND company_owner.seats = 1 THEN company_owner.seat
	   WHEN backup_owner.seats = 1 THEN backup_owner.seat END
	 FROM %[2]s contact_owner, %[3]s company_owner, %[4]s backup_owner WHERE %[1]s)`,
		only, soleOf(alias, `SELECT party_contact.owner_id AS seat FROM `+parties+`
	   JOIN contact party_contact ON party_contact.id = `+partyContactSQL+` AND party_contact.archived_at IS NULL`),
		soleOf(alias, `SELECT employer.owner_id AS seat FROM `+parties+`
	   JOIN relationship job ON job.contact_id = `+partyContactSQL+` AND `+employment.LiveCurrentPrimarySQL("job")+`
	   JOIN company employer ON employer.id = job.company_id AND employer.archived_at IS NULL`),
		soleOf(alias, `SELECT shared_box.backup_owner_id AS seat FROM capture_import shared_import
	   JOIN capture_connection shared_box ON shared_box.id = shared_import.connection_id AND shared_box.user_id IS NULL
	   JOIN team_membership backup_member ON backup_member.team_id = shared_box.reader_team_id
	    AND backup_member.user_id = shared_box.backup_owner_id
	  WHERE shared_import.activity_id = `+alias+`.id AND shared_import.user_id IS NULL`))
}

// soleOf counts the distinct seats candidates name that may hold work from
// message alias and read it, beside one of them.
func soleOf(alias, candidates string) string {
	return fmt.Sprintf(`(SELECT count(DISTINCT shared_owner_seat.id) AS seats, min(shared_owner_seat.id::text)::uuid AS seat
	  FROM (%s) shared_owner JOIN app_user shared_owner_seat ON shared_owner_seat.id = shared_owner.seat AND %s AND %s)`,
		candidates, auth.AssigneeEligibleSQL("shared_owner_seat"), auth.SeatMayReadMailSQL(alias, "shared_owner_seat.id"))
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
