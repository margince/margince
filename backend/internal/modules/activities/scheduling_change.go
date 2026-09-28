// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// ChangeInvitation requires host authority and a current invitation version.
func (s *Store) ChangeInvitation(ctx context.Context, id ids.UUID, in crmcontracts.MeetingInvitationChange) (crmcontracts.MeetingInvitation, error) {
	host, err := schedulingHost(ctx)
	if err != nil {
		return crmcontracts.MeetingInvitation{}, err
	}
	return s.applyInvitationChange(ctx, host, id, in)
}

func (s *Store) applyInvitationChange(ctx context.Context, host ids.UserID, id ids.UUID, in crmcontracts.MeetingInvitationChange) (crmcontracts.MeetingInvitation, error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return crmcontracts.MeetingInvitation{}, err
	}
	var buffer int
	if in.Action == "reschedule" {
		if in.Start == nil || in.End == nil {
			return crmcontracts.MeetingInvitation{}, errBookingEndNotAfterStart
		}
		var err error
		buffer, err = s.validateInvitationMove(ctx, host, id, *in.Start, *in.End)
		if err != nil {
			return crmcontracts.MeetingInvitation{}, err
		}
	}

	var result invitationRow
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := storekit.LockWriteIdentity(ctx, tx, "meeting_host", host.String()); err != nil {
			return err
		}
		if _, err := readActivity(ctx, tx, ids.From[ids.ActivityKind](id), storekit.LiveOnly); err != nil {
			return err
		}
		row, err := readInvitation(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if row.Host != host {
			return apperrors.ErrPermissionDenied
		}
		if row.Version != in.Version {
			return apperrors.ErrConflict
		}
		before := row
		if err := transitionInvitation(ctx, tx, &row, in, buffer); err != nil {
			return err
		}
		actor, _ := principal.Actor(ctx)
		row.Appointment.PassportID = actor.PassportID
		row.Version++
		args := schedulingArgs{}
		// Keep an in-flight save fenced until its receipt or lease expires.
		query := `UPDATE meeting_invitation SET appointment=` + args.add(row.Appointment) + `,status=` + args.add(row.Status) + `,command=` + args.add(row.Command) + `,version=` + args.add(row.Version) + `,next_attempt_at=` + args.add(s.now()) + `,attempts=0,updated_at=now() WHERE activity_id=` + args.add(id)
		if _, err := tx.Exec(ctx, query, args...); err != nil {
			return err
		}
		if err := auditInvitation(ctx, tx, &before, row); err != nil {
			return err
		}
		result = row
		return nil
	})
	return invitationView(result), err
}

// DeliverInvitations uses a fenced lease. A timeout retains the reservation and stable
// provider request id; it never turns an uncertain send into a fresh event.
func (s *Store) DeliverInvitations(ctx context.Context) error {
	if s.calendar == nil {
		return apperrors.ErrPermissionDenied
	}
	var faults []error
	for n := 0; n < 10; n++ {
		row, found, err := s.claimInvitation(ctx)
		if err != nil {
			return err
		}
		if !found {
			break
		}
		deliveryCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		receipt, err := s.executeInvitation(deliveryCtx, row)
		cancel()
		if finishErr := s.finishInvitation(ctx, row, receipt, err); finishErr != nil {
			faults = append(faults, finishErr, s.finishInvitation(ctx, row, receipt, finishErr))
		}
	}
	return errors.Join(faults...)
}

func (s *Store) claimInvitation(ctx context.Context) (invitationRow, bool, error) {
	var row invitationRow
	found := false
	err := s.tx(ctx, func(tx pgx.Tx) error {
		args := schedulingArgs{}
		now := args.add(s.now())
		rows, err := tx.Query(ctx, `SELECT activity_id FROM meeting_invitation WHERE (status IN ('pending','rescheduling','canceling') OR (status='erased' AND NOT cleanup_complete)) AND next_attempt_at<=`+now+` AND (lease_until IS NULL OR lease_until<`+now+`) ORDER BY next_attempt_at LIMIT 1 FOR UPDATE SKIP LOCKED`, args...)
		if err != nil {
			return err
		}
		var id ids.UUID
		if rows.Next() {
			err = rows.Scan(&id)
			found = true
		}
		rows.Close()
		if err != nil {
			return err
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if !found {
			return nil
		}
		row, err = readInvitation(ctx, tx, id, false)
		if err != nil {
			return err
		}
		before := row
		row.Attempts++
		row.Version++
		args = schedulingArgs{}
		query := `UPDATE meeting_invitation SET attempts=` + args.add(row.Attempts) + `,version=` + args.add(row.Version) + `,lease_until=` + args.add(s.now().Add(3*time.Minute)) + ` WHERE activity_id=` + args.add(id)
		if _, err := tx.Exec(ctx, query, args...); err != nil {
			return err
		}
		return auditInvitation(ctx, tx, &before, row)
	})
	return row, found, err
}

func (s *Store) finishInvitation(ctx context.Context, claimed invitationRow, receipt *connector.CalendarReceipt, deliveryErr error) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		if err := storekit.LockWriteIdentity(ctx, tx, "meeting_host", claimed.Host.String()); err != nil {
			return err
		}
		row, err := readInvitation(ctx, tx, claimed.ID, true)
		if err != nil {
			return err
		}
		if row.Status == invitationErased {
			return s.finishErasedInvitation(ctx, tx, row, claimed, receipt, deliveryErr)
		}
		if row.Version != claimed.Version {
			return s.finishSupersededInvitation(ctx, tx, row, claimed, receipt)
		}
		before := row
		if deliveryErr != nil {
			if row.Attempts >= 5 {
				row.Status = invitationNeedsAttention
			}
		} else {
			row.Receipt = receipt
			row.Status = invitationConfirmed
			if row.Command == invitationCancel {
				row.Status = invitationCanceled
			}
			if err := s.settleInvitation(ctx, tx, row); err != nil {
				return err
			}
		}
		row.Version++
		args := schedulingArgs{}
		query := `UPDATE meeting_invitation SET status=` + args.add(row.Status) + `,receipt=` + args.add(row.Receipt) + `,version=` + args.add(row.Version) + `,next_attempt_at=` + args.add(s.now().Add(time.Duration(row.Attempts)*time.Minute)) + `,lease_until=NULL,updated_at=now() WHERE activity_id=` + args.add(row.ID)
		if _, err := tx.Exec(ctx, query, args...); err != nil {
			return err
		}
		return auditInvitation(ctx, tx, &before, row)
	})
}

func (s *Store) settleInvitation(ctx context.Context, tx pgx.Tx, row invitationRow) error {
	current, err := readActivity(ctx, tx, ids.From[ids.ActivityKind](row.ID), storekit.LiveOnly)
	if err != nil {
		return err
	}
	status := "booked"
	if current.MeetingStatus != nil && (*current.MeetingStatus == ScheduledStatusHeld || *current.MeetingStatus == "no_show") {
		status = string(*current.MeetingStatus)
	}
	if row.Status == invitationCanceled {
		status = invitationCanceled
	}
	id := ids.From[ids.ActivityKind](row.ID)
	// Provider receipts identify the organizer's intent without taking the natural
	// key of a colleague's independently captured attendee copy.
	seconds := int(row.Appointment.End.Sub(row.Appointment.Start) / time.Second)
	if _, err := updateActivityInTx(ctx, tx, id, UpdateActivityInput{calendarSettlement: true, OccurredAt: &row.Appointment.Start, MeetingStatus: &status, DurationSeconds: &seconds}); err != nil {
		return err
	}

	if row.Receipt != nil && row.Receipt.UID != "" {
		_, err := ClaimIdentity(ctx, tx, id, IdentityKindMeeting, MeetingIdentityKey(row.Receipt.UID, row.Appointment.Start.Format(time.RFC3339Nano)), "calendar_booking")
		if err != nil {
			return err
		}
	}
	return nil
}

func refuseActiveInvitationPatch(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	args := schedulingArgs{}
	var active bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM meeting_invitation WHERE activity_id=`+args.add(id)+` AND status<>'canceled')`, args...).Scan(&active)
	if err != nil {
		return err
	}
	if active {
		return &SchedulingArgumentError{Field: KindMeeting, Code: "calendar_managed", Message: "Change or cancel this meeting through its calendar invitation"}
	}
	return nil
}

func transitionInvitation(ctx context.Context, tx pgx.Tx, row *invitationRow, in crmcontracts.MeetingInvitationChange, buffer int) error {
	switch in.Action {
	case "reschedule":
		if row.Status != invitationConfirmed {
			return apperrors.ErrConflict
		}
		if err := reserveInvitation(ctx, tx, row.Host, *in.Start, *in.End, buffer, row.ID); err != nil {
			return err
		}
		row.Appointment.Start, row.Appointment.End = *in.Start, *in.End
		row.Status, row.Command = "rescheduling", "update"
	case invitationCancel:
		if row.Status == invitationCanceled || row.Status == invitationCanceling || row.Status == invitationErased {
			return apperrors.ErrConflict
		}
		row.Status, row.Command = invitationCanceling, invitationCancel
	case "retry":
		if row.Status != invitationNeedsAttention {
			return apperrors.ErrConflict
		}
		row.Status = invitationPending
		if row.Command == "update" {
			row.Status = "rescheduling"
		}
		if row.Command == invitationCancel {
			row.Status = invitationCanceling
		}
	default:
		return apperrors.ErrConflict
	}
	return nil
}

//nolint:nilnil // Authoritative provider absence completes a cancellation without a receipt.
func (s *Store) executeInvitation(ctx context.Context, row invitationRow) (*connector.CalendarReceipt, error) {
	if err := s.calendar.Check(ctx, row.Host, row.Provider); err != nil {
		return row.Receipt, err
	}
	if row.Command != invitationCancel {
		appointment := row.Appointment
		if row.Receipt != nil {
			appointment.EventID = row.Receipt.EventID
		}
		return s.deliverInvitation(ctx, row, appointment)
	}
	receipt := row.Receipt
	if receipt == nil {
		appointment := row.Appointment
		appointment.Cancel = true
		found, err := s.calendar.Lookup(ctx, row.Host, row.Provider, appointment)
		if err != nil {
			return nil, err
		}
		receipt = found
	}
	if receipt == nil {
		return nil, nil
	}
	return receipt, s.calendar.Cancel(ctx, row.Host, row.Provider, row.Calendar, receipt.EventID)
}

func changesInvitation(in UpdateActivityInput) bool {
	return in.OccurredAt != nil || in.DurationSeconds != nil || in.Subject != nil || in.Body != nil ||
		(in.MeetingStatus != nil && *in.MeetingStatus != ScheduledStatusHeld && *in.MeetingStatus != "no_show")
}
