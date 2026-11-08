// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
		if err == nil && !found {
			id, found, err = s.meetingOfOwnEventID(ctx, tx, key)
		}
		if err != nil || !found {
			return err
		}
		organizer, err := SeatOrganizesMeetingTx(ctx, tx, id, actorUserID(ctx))
		if err != nil {
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

// SeatOrganizesMeetingTx reports whether the seat is the meeting's recorded
// organizer. Only the organizer's calendar speaks for the whole meeting: its
// removal cancels it and its complete roster retires the guests it dropped.
func SeatOrganizesMeetingTx(ctx context.Context, tx pgx.Tx, meeting ids.ActivityID, seat ids.UUID) (bool, error) {
	if seat == ids.Nil {
		return false, nil
	}
	var organizer bool
	if err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM activity_participant organizer_row JOIN activity a ON a.id=organizer_row.activity_id
 WHERE a.archived_at IS NULL AND a.restricted_at IS NULL AND organizer_row.activity_id=@activity AND organizer_row.user_id=@seat AND organizer_row.role=@organizer AND %s)`, auth.ParticipantStandsSQL("organizer_row", "a")), pgx.StrictNamedArgs{
		capturedActivityObject: meeting, rosterSeatArg: seat, "organizer": connector.ParticipantRoleOrganizer,
	}).Scan(&organizer); err != nil {
		return false, fmt.Errorf("capture: reading whether a seat organizes a meeting: %w", err)
	}
	return organizer, nil
}
