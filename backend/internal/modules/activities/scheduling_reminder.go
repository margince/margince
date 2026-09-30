// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
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
	origin := FromAccount([]ActivityLinkInput{{EntityType: linkEntityContact, EntityID: row.Appointment.ContactID}})
	input, err := s.meetingReminderInput(ctx, row)
	if err != nil {
		return err
	}
	prepared, err := s.PrepareSend(ctx, origin, input, gate, stager)
	if err != nil {
		return errors.Join(err, s.markMeetingReminderUnavailable(ctx, row))
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
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
		return errors.Join(err, s.markMeetingReminderUnavailable(ctx, row))
	}
	return nil
}

func (s *Store) meetingReminderInput(ctx context.Context, row invitationRow) (SendEmailInput, error) {
	link, err := s.openMeetingLink(ctx, row.ManagementRef)
	if err != nil {
		return SendEmailInput{}, err
	}
	hours, err := s.strictHours(ctx, row.Host)
	if err != nil {
		return SendEmailInput{}, err
	}
	zone := hours.Location
	subject, template := "Meeting reminder: ", "Your meeting starts at %s (%s).\n\n%s\n\nReschedule or cancel: %s"
	switch s.footerLanguage(ctx, row.Appointment.Description, row.Appointment.Subject) {
	case textlang.German:
		subject, template = "Terminerinnerung: ", "Dein Termin beginnt am %s (%s).\n\n%s\n\nVerschieben oder absagen: %s"
	case textlang.Vietnamese:
		subject, template = "Nhắc cuộc hẹn: ", "Cuộc hẹn của bạn bắt đầu lúc %s (%s).\n\n%s\n\nĐổi lịch hoặc hủy: %s"
	}
	body := fmt.Sprintf(template, row.Appointment.Start.In(zone).Format("2006-01-02 15:04 MST"), zone.String(), row.Appointment.Location, link)
	return SendEmailInput{Recipients: row.Appointment.Attendees, Subject: subject + row.Appointment.Subject, Body: body, ConsentPurpose: "transactional", Context: commsauthz.CategoryRequestedFollowup, Evidence: commsauthz.Evidence{ActivityID: row.ID}}, nil
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
		return false, fmt.Errorf("meeting reminder: confirmed invitation has no receipt")
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
