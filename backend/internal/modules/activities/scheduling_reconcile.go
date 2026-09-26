// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// ReconcileInvitations imports provider-side moves and cancellations into the same activity.
func (s *Store) ReconcileInvitations(ctx context.Context) error {
	var rows []invitationRow
	err := s.tx(ctx, func(tx pgx.Tx) error {
		args := schedulingArgs{}
		cursor, err := tx.Query(ctx, `SELECT activity_id FROM meeting_invitation WHERE status='confirmed' AND (appointment->>'End')::timestamptz>`+args.add(s.now().Add(-7*24*time.Hour))+` AND next_attempt_at<`+args.add(s.now())+` ORDER BY next_attempt_at LIMIT 20 FOR UPDATE SKIP LOCKED`, args...)
		if err != nil {
			return err
		}
		var keys []ids.UUID
		for cursor.Next() {
			var id ids.UUID
			if err := cursor.Scan(&id); err != nil {
				cursor.Close()
				return err
			}
			keys = append(keys, id)
		}
		cursor.Close()
		if err := cursor.Err(); err != nil {
			return err
		}
		for _, id := range keys {
			row, err := readInvitation(ctx, tx, id, false)
			if err != nil {
				return err
			}
			args := schedulingArgs{}
			if _, err := tx.Exec(ctx, `UPDATE meeting_invitation SET next_attempt_at=`+args.add(s.now().Add(15*time.Minute))+`,updated_at=now() WHERE activity_id=`+args.add(row.ID), args...); err != nil {
				return err
			}
			rows = append(rows, row)
		}
		return nil
	})
	if err != nil {
		return err
	}
	var faults []error
	for _, row := range rows {
		if row.Receipt == nil {
			faults = append(faults, fmt.Errorf("calendar: confirmed invitation has no receipt"))
			continue
		}
		state, err := s.calendar.Inspect(ctx, row.Host, row.Provider, row.Calendar, row.Receipt.EventID)
		if err == nil {
			err = s.reconcileInvitation(ctx, row, state)
		} else if errors.Is(err, connector.ErrAuthRejected) || errors.Is(err, apperrors.ErrPermissionDenied) {
			err = errors.Join(err, s.markInvitationAttention(ctx, row))
		}
		if err != nil {
			faults = append(faults, err)
		}
	}
	return errors.Join(faults...)
}

func (s *Store) reconcileInvitation(ctx context.Context, observed invitationRow, state connector.CalendarState) error {
	if !state.Canceled && !state.End.After(state.Start) {
		return fmt.Errorf("calendar: invalid observed interval")
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		if err := storekit.LockWriteIdentity(ctx, tx, "meeting_host", observed.Host.String()); err != nil {
			return err
		}
		row, err := readInvitation(ctx, tx, observed.ID, true)
		if err != nil {
			return err
		}
		if row.Version != observed.Version || row.Status != invitationConfirmed {
			return nil
		}
		before := row
		changed := state.Canceled || !state.Start.Equal(row.Appointment.Start) || !state.End.Equal(row.Appointment.End)
		if !changed {
			return nil
		}
		if state.Canceled {
			row.Status = invitationCanceled
		} else {
			row.Appointment.Start = state.Start
			row.Appointment.End = state.End
		}
		// Provider changes can overlap reservations; localMeetingSlots still counts
		// this invitation after its exclusivity claim is released.
		args := schedulingArgs{}
		if _, err := tx.Exec(ctx, `UPDATE activity SET claims_host_slot=false WHERE id=`+args.add(row.ID), args...); err != nil {
			return err
		}
		if err := s.settleInvitation(ctx, tx, row); err != nil {
			return err
		}
		row.Version++
		args = schedulingArgs{}
		if _, err := tx.Exec(ctx, `UPDATE meeting_invitation SET appointment=`+args.add(row.Appointment)+`,status=`+args.add(row.Status)+`,version=`+args.add(row.Version)+`,next_attempt_at=`+args.add(s.now().Add(15*time.Minute))+`,updated_at=now() WHERE activity_id=`+args.add(row.ID), args...); err != nil {
			return err
		}
		return auditInvitation(ctx, tx, &before, row)
	})
}
