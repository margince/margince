// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/platform/mailrole"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// auditFieldParticipantSeats names the seats a repair took off meeting
// participant rows, in the audit images.
const auditFieldParticipantSeats = "participant_seats"

// auditFieldCreditedSeats names the seats a repair took out of a meeting's
// held snapshots, in the audit images.
const auditFieldCreditedSeats = "credited_seats"

// otherAddressSeatSQL holds for participant p of calendar meeting a when the row
// binds a seat to an address that is nobody's login or declared address. Only a
// mailbox the seat connected binds it that way. A row a member
// relinked, and a meeting a member logged, are their choice and stay.
const otherAddressSeatSQL = `a.kind = 'meeting' AND a.restricted_at IS NULL AND starts_with(a.captured_by, 'connector:')
	   AND p.user_id IS NOT NULL AND p.address IS NOT NULL AND p.relinked_at IS NULL
	   AND p.role IN (` + storekit.MeetingAttendingRolesSQL + `)
	   AND NOT EXISTS (SELECT 1 FROM app_user u WHERE u.email = p.address OR %s)
	   AND NOT EXISTS (SELECT 1 FROM capture_owner_identity i WHERE i.kind = 'address' AND i.value = p.address)`

func otherAddressSeat() string {
	return fmt.Sprintf(otherAddressSeatSQL, storekit.SeatLoginIsSQL("u", "p.address", "p.address_domain_folded"))
}

// RoleMailboxActivitiesTx locks the next limit meetings after the cursor, in id
// order, whose participants bind a seat to an address other than its login. A
// caller then asks which of those addresses are role mailboxes, because the
// role vocabulary is code and not SQL.
func RoleMailboxActivitiesTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT a.id FROM activity a WHERE a.id > $1
		   AND EXISTS (SELECT 1 FROM activity_participant p WHERE p.activity_id = a.id AND `+otherAddressSeat()+`)
		 ORDER BY a.id LIMIT $2`, after, limit)
	if err != nil {
		return nil, fmt.Errorf("activities: reading meetings with a seat bound by another address: %w", err)
	}
	found, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("activities: reading meetings with a seat bound by another address: %w", err)
	}
	return found, storekit.LockMeetingAttendeePage(ctx, tx, found)
}

// CountRoleMailboxActivities counts the meetings RoleMailboxActivitiesTx would
// read, at most maximum+1. It over-counts the ones whose other address is a
// personal one, so it is an upper bound for the repair's preview.
func CountRoleMailboxActivities(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM activity a
		   WHERE EXISTS (SELECT 1 FROM activity_participant p WHERE p.activity_id = a.id AND `+otherAddressSeat()+`)
		 LIMIT $1) bound`, maximum+1).Scan(&count); err != nil {
		return 0, fmt.Errorf("activities: counting meetings with a seat bound by another address: %w", err)
	}
	return count, nil
}

// UnbindRoleMailboxSeatsTx takes the seat off the participant rows of one
// meeting whose address is a role mailbox. The row, its address and its answer
// stay, so the team's reply is still on record and credits nobody. The rule is
// read again under the row lock, so a replay changes nothing.
func UnbindRoleMailboxSeatsTx(ctx context.Context, tx pgx.Tx, activityID, runID ids.UUID) (bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return false, err
	}
	if err := storekit.LockMeetingAttendeeWrites(ctx, tx, activityID); err != nil {
		return false, err
	}
	if _, err := storekit.LockRow(ctx, tx, "activity", activityID, storekit.IncludeArchived); err != nil {
		return false, err
	}
	rows, err := tx.Query(ctx, `SELECT p.id, p.user_id, p.address FROM activity_participant p JOIN activity a ON a.id = p.activity_id
		 WHERE a.id = $1 AND `+otherAddressSeat()+` AND `+auth.ParticipantStandsSQL("p", "a"), activityID)
	if err != nil {
		return false, fmt.Errorf("activities: reading the seats bound to %s by another address: %w", activityID, err)
	}
	type bound struct {
		ID, Seat ids.UUID
		Address  string
	}
	candidates, err := pgx.CollectRows(rows, pgx.RowToStructByPos[bound])
	if err != nil {
		return false, fmt.Errorf("activities: reading the seats bound to %s by another address: %w", activityID, err)
	}
	var rowIDs, seats []ids.UUID
	for _, c := range candidates {
		if _, role := mailrole.Match(c.Address); role {
			rowIDs, seats = append(rowIDs, c.ID), append(seats, c.Seat)
		}
	}
	if len(rowIDs) == 0 {
		return false, nil
	}
	if _, err := tx.Exec(ctx, `UPDATE activity_participant SET user_id = NULL WHERE id = ANY($1)`, rowIDs); err != nil {
		return false, fmt.Errorf("activities: unbinding the role mailbox seats of %s: %w", activityID, err)
	}
	dropped, err := dropRoleCreditTx(ctx, tx, activityID, seats)
	if err != nil {
		return false, err
	}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", "activity", activityID,
		map[string]any{auditFieldParticipantSeats: seats, auditFieldCreditedSeats: dropped},
		map[string]any{auditFieldParticipantSeats: []ids.UUID{}, auditFieldCreditedSeats: []ids.UUID{}},
		map[string]any{evidenceDataTaskRun: runID.String()})
	if err != nil {
		return false, err
	}
	return true, storekit.EmitEvent(ctx, tx, auditID, activityID, crmcontracts.PublicEventActivityUpdated{})
}

// dropRoleCreditTx takes the seats the role mailbox binding gave credit out of
// the meeting's held snapshots. A seat the live rule still credits, through its
// own answer or as host, keeps it. It answers the seats it removed.
func dropRoleCreditTx(ctx context.Context, tx pgx.Tx, activityID ids.UUID, seats []ids.UUID) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, `SELECT s.seat FROM activity a CROSS JOIN unnest($2::uuid[]) AS s(seat)
		WHERE a.id = $1 AND NOT (`+storekit.MeetingCreditSQL("a", "s.seat")+`)
		  AND EXISTS (SELECT 1 FROM activity_meeting_history h WHERE h.activity_id = $1 AND s.seat = ANY(h.credited_seats_at_change))`,
		activityID, seats)
	if err != nil {
		return nil, fmt.Errorf("activities: reading the snapshot credit of %s: %w", activityID, err)
	}
	dropped, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("activities: reading the snapshot credit of %s: %w", activityID, err)
	}
	if len(dropped) == 0 {
		return nil, nil
	}
	if _, err := tx.Exec(ctx, `UPDATE activity_meeting_history h SET credited_seats_at_change = ARRAY(
		    SELECT kept FROM unnest(h.credited_seats_at_change) AS kept WHERE kept <> ALL($2::uuid[]))
		 WHERE h.activity_id = $1 AND h.credited_seats_at_change && $2::uuid[]`, activityID, dropped); err != nil {
		return nil, fmt.Errorf("activities: taking role mailbox credit out of the snapshots of %s: %w", activityID, err)
	}
	return dropped, nil
}
