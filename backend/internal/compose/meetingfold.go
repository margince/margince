// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// One meeting stored as two live rows is folded onto one of them. The calendar
// sync and the repair data tasks share this rule, so a pair the sync keeps is
// one the tasks keep too.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// foldMeetingDuplicateTx retires duplicate onto survivor when retiring it loses
// nothing. The caller has proved that seat's calendar binds the event to
// survivor. calendar names the account the event came from, or is empty when
// any connected calendar of the duplicate's provider will do. It answers
// whether the duplicate was retired.
func foldMeetingDuplicateTx(
	ctx context.Context, tx pgx.Tx, meetings *activities.Store,
	duplicate, survivor ids.ActivityID, seat ids.UUID, calendar string,
) (bool, error) {
	live, err := lockFoldPair(ctx, tx, duplicate, survivor)
	if err != nil || !live {
		return false, err
	}
	retirable, err := copyRetiresCleanly(ctx, tx, duplicate, survivor, seat, calendar)
	if err != nil || !retirable {
		return false, err
	}
	if err := capture.CarryMeetingImportTx(ctx, tx, duplicate, survivor, seat); err != nil {
		return false, err
	}
	if err := meetings.RetireMeetingCopyTx(ctx, tx, duplicate, survivor); err != nil {
		return false, err
	}
	// The survivor's audience follows the seats now recorded on it.
	return true, activities.RecomputeAudiencesTx(ctx, tx, []ids.ActivityID{survivor})
}

// lockFoldPair locks both rows in id order, so two folds of one pair cannot
// deadlock, and answers whether both are still live and unrestricted. The
// checks after it judge rows that stay as read until the fold commits.
func lockFoldPair(ctx context.Context, tx pgx.Tx, duplicate, survivor ids.ActivityID) (bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT archived_at IS NULL AND restricted_at IS NULL FROM activity
		 WHERE id IN ($1, $2) ORDER BY id FOR UPDATE`, duplicate, survivor)
	if err != nil {
		return false, fmt.Errorf("compose: locking a meeting stored twice: %w", err)
	}
	open, err := pgx.CollectRows(rows, pgx.RowTo[bool])
	if err != nil {
		return false, fmt.Errorf("compose: reading a meeting stored twice: %w", err)
	}
	return len(open) == 2 && open[0] && open[1], nil
}

// sameImportAccess compares the import fields that decide who may read a
// meeting: the posture and the verdict.
const sameImportAccess = `(kept.posture_at_import, kept.verdict_status, kept.verdict_reason, kept.verdict_reasons)
	IS NOT DISTINCT FROM (other.posture_at_import, other.verdict_status, other.verdict_reason, other.verdict_reasons)`

// copyRetiresCleanly answers whether archiving the copy loses nothing.
//   - The seat still has the copy's calendar connected.
//   - Every other importer of the copy imports the survivor with the same
//     posture and verdict, and so does the seat where it has both.
//   - No human changed the copy, and no booking stands behind it.
//   - Every record the copy is filed under is the survivor's too.
func copyRetiresCleanly(ctx context.Context, tx pgx.Tx, duplicate, survivor ids.ActivityID, seat ids.UUID, calendar string) (bool, error) {
	var clean bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM capture_connection c JOIN activity d ON d.id = $1
		                WHERE c.provider = d.source_system AND c.user_id = $3 AND c.status = 'connected'
		                  AND ($4 = '' OR lower(btrim(c.account_label)) = lower(btrim($4))))
		   AND NOT EXISTS (SELECT 1 FROM capture_import other
		                    WHERE other.activity_id = $1 AND other.user_id <> $3
		                      AND NOT EXISTS (SELECT 1 FROM capture_import kept
		                                       WHERE kept.activity_id = $2 AND kept.user_id = other.user_id
		                                         AND `+sameImportAccess+`))
		   AND NOT EXISTS (SELECT 1 FROM capture_import kept
		                    JOIN capture_import other ON other.activity_id = $1 AND other.user_id = kept.user_id
		                   WHERE kept.activity_id = $2 AND kept.user_id = $3
		                     AND NOT `+sameImportAccess+`)
		   AND NOT EXISTS (SELECT 1 FROM audit_log worked
		                    WHERE worked.entity_type = 'activity' AND worked.entity_id = $1 AND worked.actor_type = 'human')
		   AND NOT EXISTS (SELECT 1 FROM meeting_invitation booked WHERE booked.activity_id = $1)
		   AND NOT EXISTS (SELECT 1 FROM activity_link l WHERE l.activity_id = $1 AND NOT EXISTS (
		         SELECT 1 FROM activity_link s WHERE s.activity_id = $2 AND s.entity_type = l.entity_type
		            AND (s.contact_id, s.company_id, s.deal_id, s.lead_id, s.project_id)
		                IS NOT DISTINCT FROM (l.contact_id, l.company_id, l.deal_id, l.lead_id, l.project_id)))`,
		duplicate, survivor, seat, calendar).Scan(&clean)
	if err != nil {
		return false, fmt.Errorf("compose: reading whether a calendar copy retires cleanly: %w", err)
	}
	return clean, nil
}

// foldCapturedMeeting is the sink's capture.MeetingFolder. The seat folded for
// is the one whose calendar stamped the duplicate. An unproven fold also needs
// that seat to be on the survivor already, as an importer or a bound guest.
func foldCapturedMeeting(meetings *activities.Store) capture.MeetingFolder {
	return func(ctx context.Context, tx pgx.Tx, duplicate, survivor ids.ActivityID, proven bool) (bool, error) {
		var seat *ids.UUID
		var onSurvivor bool
		err := tx.QueryRow(ctx, `
			SELECT `+auth.ConnectorSeatSQL("d")+`::uuid,
			       EXISTS (SELECT 1 FROM capture_import ci WHERE ci.activity_id = $2 AND ci.user_id::text = `+auth.ConnectorSeatSQL("d")+`)
			    OR EXISTS (SELECT 1 FROM activity_participant ap JOIN activity s ON s.id = ap.activity_id
			                WHERE ap.activity_id = $2 AND ap.user_id::text = `+auth.ConnectorSeatSQL("d")+`
			                  AND `+auth.ParticipantStandsSQL("ap", "s")+`)
			  FROM activity d WHERE d.id = $1 AND d.archived_at IS NULL`, duplicate, survivor).Scan(&seat, &onSurvivor)
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("compose: reading whose calendar stored a meeting copy: %w", err)
		}
		if seat == nil || (!proven && !onSurvivor) {
			return false, nil
		}
		return foldMeetingDuplicateTx(ctx, tx, meetings, duplicate, survivor, *seat, "")
	}
}
