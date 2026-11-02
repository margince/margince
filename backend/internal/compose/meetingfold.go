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
	live, err := lockLiveSurvivor(ctx, tx, survivor)
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

// lockLiveSurvivor locks the meeting a copy binds to and answers whether it is
// still live and unrestricted. Nothing is carried onto a row erased or held
// since the binding rule answered.
func lockLiveSurvivor(ctx context.Context, tx pgx.Tx, survivor ids.ActivityID) (bool, error) {
	var open bool
	err := tx.QueryRow(ctx, `SELECT restricted_at IS NULL FROM activity WHERE id = $1 AND archived_at IS NULL FOR UPDATE`, survivor).Scan(&open)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("compose: locking the meeting a calendar copy binds to: %w", err)
	}
	return open, nil
}

// copyRetiresCleanly answers whether archiving the copy loses nothing.
//   - The seat still has the copy's calendar connected.
//   - Every other importer of the copy imports the survivor with the same
//     posture, and the seat's own posture matches where it has both.
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
		                                         AND kept.posture_at_import IS NOT DISTINCT FROM other.posture_at_import))
		   AND NOT EXISTS (SELECT 1 FROM capture_import kept
		                    JOIN capture_import copied ON copied.activity_id = $1 AND copied.user_id = kept.user_id
		                   WHERE kept.activity_id = $2 AND kept.user_id = $3
		                     AND kept.posture_at_import IS DISTINCT FROM copied.posture_at_import)
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
