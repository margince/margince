// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/emailview"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

const auditReminderStatus = "reminder_status"

// MeetingReminder names the host whose live authority a due reminder needs.
type MeetingReminder struct {
	ID   ids.UUID
	Host ids.UserID
}

// DueMeetingReminders exposes no guest content to the scheduling dispatcher.
func (s *Store) DueMeetingReminders(ctx context.Context) ([]MeetingReminder, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	var due []MeetingReminder
	err := s.tx(ctx, func(tx pgx.Tx) error {
		args := schedulingArgs{}
		rows, err := tx.Query(ctx, `SELECT activity_id,host_user_id FROM meeting_invitation WHERE status='confirmed' AND reminder_status='pending' AND (appointment->>'Start')::timestamptz<=`+args.add(s.now().Add(time.Hour))+` ORDER BY (appointment->>'Start')::timestamptz LIMIT 20`, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var reminder MeetingReminder
			if err := rows.Scan(&reminder.ID, &reminder.Host); err != nil {
				return err
			}
			due = append(due, reminder)
		}
		return rows.Err()
	})
	return due, err
}

// SendMeetingReminder shares the governed email writer and commits its marker
// with the delivery, so overlapping workers cannot enqueue the reminder twice.
func (s *Store) SendMeetingReminder(ctx context.Context, id ids.UUID, gate ConsentGate, stager DeliveryStager) error {
	host, err := schedulingHost(ctx)
	if err != nil {
		return err
	}
	row, err := s.scopedInvitation(ctx, host, id)
	if err != nil {
		return err
	}
	ready, err := s.meetingReminderReady(ctx, row)
	if err != nil || !ready {
		return err
	}
	attendee := ActivityLinkInput{EntityType: linkEntityContact, EntityID: row.Appointment.ContactID}
	if !row.Appointment.LeadID.IsZero() {
		attendee = ActivityLinkInput{EntityType: linkEntityLead, EntityID: row.Appointment.LeadID}
	}
	origin := FromAccount([]ActivityLinkInput{attendee})
	input, link, err := s.meetingReminderInput(ctx, row)
	if err != nil {
		return err
	}
	prepared, err := s.PrepareSend(ctx, origin, input, gate, stager)
	if err != nil {
		return s.meetingReminderNotSent(ctx, row, err, stager)
	}
	prepared = prepared.withholdingFromRecord(link)
	err = s.txRetryingLockCycles(ctx, pgx.ReadCommitted, func(tx pgx.Tx) error {
		current, err := readInvitation(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if current.Version != row.Version || current.Status != invitationConfirmed || current.ReminderStatus != invitationPending {
			return nil
		}
		if _, err := s.SendPreparedTx(ctx, tx, origin, prepared, stager); err != nil {
			return err
		}
		return writeMeetingReminder(ctx, tx, current, "queued")
	})
	if err != nil {
		return s.meetingReminderNotSent(ctx, row, err, stager)
	}
	return nil
}

// meetingReminderNotSent marks the reminder unavailable. A refusal by the send
// rules is handled once the host is told; any other failure is the job's fault.
func (s *Store) meetingReminderNotSent(ctx context.Context, row invitationRow, err error, stager DeliveryStager) error {
	if errors.Is(err, apperrors.ErrConsentNotGranted) {
		return s.markMeetingReminderRefused(ctx, row, err, stager)
	}
	return errors.Join(err, s.markMeetingReminderUnavailable(ctx, row))
}

// meetingReminderInput is the reminder and the guest's management link it
// carries. The link is a bearer capability over the host's meeting, so the
// caller keeps it off the recorded copy.
func (s *Store) meetingReminderInput(ctx context.Context, row invitationRow) (SendEmailInput, string, error) {
	link, err := s.openMeetingLink(ctx, row.ManagementRef)
	if err != nil {
		return SendEmailInput{}, "", err
	}
	hours, err := s.strictHours(ctx, row.Host)
	if err != nil {
		return SendEmailInput{}, "", err
	}
	zone := hours.Location
	words := mailcopy.For(string(s.footerLanguage(ctx, row.Appointment.Description, row.Appointment.Subject)))
	// No letterhead: the reminder leaves through the host's own mailbox, not
	// the system relay, so it does not speak as the installation.
	message := words.MeetingMessage(row.Appointment.Subject, row.Appointment.Description, row.Appointment.Location, link, row.Appointment.Start.In(zone), row.Appointment.End.In(zone))
	return SendEmailInput{Recipients: row.Appointment.Attendees, Subject: message.Title, Body: message.Text, HTMLBody: emailview.Render(message, emailview.Letterhead{}), ConsentPurpose: "transactional", Context: commsauthz.CategoryRequestedFollowup, Evidence: commsauthz.Evidence{ActivityID: row.ID}}, link, nil
}

func (s *Store) markMeetingReminderUnavailable(ctx context.Context, observed invitationRow) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		row, err := readInvitation(ctx, tx, observed.ID, true)
		if err != nil {
			return err
		}
		if row.Version != observed.Version || row.ReminderStatus != invitationPending {
			return nil
		}
		return writeMeetingReminder(ctx, tx, row, "unavailable")
	})
}

// markMeetingReminderRefused tells the host, in the transaction that marks the
// reminder unavailable, that the send rules stopped it. The reminder is not held
// for later, because a held copy could go out after the meeting moved. The
// stager records why in the same transaction.
func (s *Store) markMeetingReminderRefused(ctx context.Context, observed invitationRow, refusal error, stager DeliveryStager) error {
	if s.invitationAttention == nil {
		return errors.New("meeting reminder: nothing is wired to tell the host of a refused reminder")
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		row, err := readInvitation(ctx, tx, observed.ID, true)
		if err != nil || row.Version != observed.Version || row.ReminderStatus != invitationPending {
			return err
		}
		if err := writeMeetingReminder(ctx, tx, row, "unavailable"); err != nil {
			return err
		}
		if err := recordUnheldRefusalTx(ctx, tx, stager, refusal); err != nil {
			return err
		}
		return s.invitationAttention.InvitationAttentionTx(ctx, tx, row.Host, row.ID, ids.NewV7(), AttentionReminderRefused)
	})
}

func writeMeetingReminder(ctx context.Context, tx pgx.Tx, row invitationRow, status string) error {
	before := row
	row.ReminderStatus = status
	row.Version++
	args := schedulingArgs{}
	if _, err := tx.Exec(ctx, `UPDATE meeting_invitation SET reminder_status=`+args.add(status)+`,version=`+args.add(row.Version)+`,updated_at=now() WHERE activity_id=`+args.add(row.ID), args...); err != nil {
		return err
	}
	return auditInvitation(ctx, tx, &before, row)
}

func (s *Store) meetingReminderReady(ctx context.Context, row invitationRow) (bool, error) {
	if row.Status != invitationConfirmed || row.ReminderStatus != invitationPending {
		return false, nil
	}
	until := row.Appointment.Start.Sub(s.now())
	if until > time.Hour {
		return false, nil
	}
	if until < 45*time.Minute {
		return false, s.markMeetingReminderUnavailable(ctx, row)
	}
	if row.Receipt == nil || s.calendar == nil {
		return false, errors.New("meeting reminder: confirmed invitation has no receipt")
	}
	state, err := s.calendar.Inspect(ctx, row.Host, row.Provider, row.Calendar, row.Receipt.EventID)
	if err != nil {
		return false, err
	}
	if state.Canceled || !state.Start.Equal(row.Appointment.Start) || !state.End.Equal(row.Appointment.End) {
		return false, s.reconcileInvitation(ctx, row, state)
	}
	return true, nil
}

// RefuseMeetingReminder makes withdrawn sender authority visible to the host.
func (s *Store) RefuseMeetingReminder(ctx context.Context, reminder MeetingReminder) error {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return err
	}
	row, err := s.scopedInvitation(ctx, reminder.Host, reminder.ID)
	if err != nil {
		return err
	}
	return s.markMeetingReminderUnavailable(ctx, row)
}
