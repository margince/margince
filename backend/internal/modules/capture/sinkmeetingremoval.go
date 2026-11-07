// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// RemoveCalendarMeeting treats a calendar-copy deletion as local unless the
// syncing seat is the meeting's recorded organizer.
func (s *Sink) RemoveCalendarMeeting(ctx context.Context, key connector.NaturalKey, identity connector.CrossDoorIdentity, at time.Time) error {
	if err := admitCalendarVerb(ctx, key, "remove"); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		ctx, err := s.mailboxRunAllowedTx(ctx, tx)
		if err != nil {
			return err
		}
		id, found, err := s.meetingForSeat(ctx, tx, connector.NormalizedRecord{NaturalKey: key, CrossDoorIdentity: identity})
		if err != nil || !found {
			return err
		}
		var organizer bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM activity_participant
   WHERE activity_id=@activity AND user_id=@seat AND role=@organizer)`, pgx.StrictNamedArgs{
			capturedActivityObject: id, "seat": actorUserID(ctx), "organizer": connector.ParticipantRoleOrganizer,
		}).Scan(&organizer); err != nil {
			return err
		}
		if organizer {
			if s.cancelMeetingByID != nil {
				_, err = s.cancelMeetingByID(ctx, tx, id, key, at)
				return err
			}
			if s.cancelMeeting != nil {
				_, _, err = s.cancelMeeting(ctx, tx, key, at)
				return err
			}
			return nil
		}
		_, err = SettleMeetingRosterTx(ctx, ctx, tx, id, MeetingRoster{Seat: actorUserID(ctx), Current: true, Left: true}, nil)
		return err
	})
}
