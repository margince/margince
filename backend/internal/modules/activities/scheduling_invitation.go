// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const auditInvitationStatus = "invitation_status"

const (
	invitationPending        = "pending"
	invitationConfirmed      = "confirmed"
	invitationNeedsAttention = "needs_attention"
	invitationCanceled       = "canceled"
	invitationCancel         = "cancel"
	invitationCanceling      = "canceling"
	invitationErased         = "erased"
	fieldDurationSeconds     = "duration_seconds"
)

type invitationRow struct {
	ID             ids.UUID
	Host           ids.UserID
	Provider       string
	Calendar       string
	Appointment    connector.CalendarAppointment
	Receipt        *connector.CalendarReceipt
	ReminderStatus string
	Status         string
	Version        int64
	Command        string
	Attempts       int
	ManagementRef  string
	PublicIntent   *publicBookingIntent
}

type schedulingArgs []any

//craft:ignore naked-any SQL values cross this boundary with their driver types intact
func (a *schedulingArgs) add(value any) string {
	*a = append(*a, value)
	return fmt.Sprintf("$%d", len(*a))
}

func invitationToken() (string, string, error) {
	var entropy [32]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(entropy[:])
	hash := sha256.Sum256([]byte(token))
	return token, hex.EncodeToString(hash[:]), nil
}

func invitationView(row invitationRow) crmcontracts.MeetingInvitation {
	out := crmcontracts.MeetingInvitation{
		Id: crmcontracts.Id(row.ID), Status: crmcontracts.MeetingInvitationStatus(row.Status),
		Start: row.Appointment.Start, End: row.Appointment.End, Subject: row.Appointment.Subject, Location: row.Appointment.Location, Version: row.Version,
	}
	reminder := crmcontracts.MeetingInvitationReminderStatus(row.ReminderStatus)
	out.ReminderStatus = &reminder
	if row.Receipt != nil && row.Receipt.URL != "" {
		out.CalendarUrl = &row.Receipt.URL
	}
	return out
}

func validateInvitation(in crmcontracts.MeetingInvitationRequest) error {
	if err := httperr.RequireBodyID("contact_id", ids.UUID(in.ContactId)); err != nil {
		return err
	}
	address, err := mail.ParseAddress(string(in.AttendeeEmail))
	if err != nil || address.Address != string(in.AttendeeEmail) || len(in.Subject) > 200 || strings.TrimSpace(in.Subject) == "" || len(in.Description) > 5000 || len(in.Location) > 1000 {
		return &SchedulingArgumentError{Field: "invitation", Code: faultInvalid, Message: "Enter a valid attendee email and meeting details"}
	}
	return nil
}

// CreateInvitation reserves time before queuing an idempotent provider invitation.
func (s *Store) CreateInvitation(ctx context.Context, in crmcontracts.MeetingInvitationRequest) (crmcontracts.MeetingInvitation, error) {
	host, err := schedulingHost(ctx)
	if err != nil {
		return crmcontracts.MeetingInvitation{}, err
	}
	return s.reserveAndQueueInvitation(ctx, host, in, invitationIntent{})
}

func (s *Store) reserveAndQueueInvitation(ctx context.Context, host ids.UserID, in crmcontracts.MeetingInvitationRequest, intent invitationIntent) (crmcontracts.MeetingInvitation, error) {
	if err := validateInvitation(in); err != nil {
		return crmcontracts.MeetingInvitation{}, err
	}
	profile, err := s.validateInvitationTime(ctx, host, in.Start, in.End)
	if err != nil {
		return crmcontracts.MeetingInvitation{}, err
	}
	if err := s.calendar.CheckRecipient(ctx, host, ids.UUID(in.ContactId), string(in.AttendeeEmail)); err != nil {
		return crmcontracts.MeetingInvitation{}, err
	}
	token, hash, err := invitationToken()
	if err != nil {
		return crmcontracts.MeetingInvitation{}, err
	}
	ref, err := s.sealMeetingLink(ctx, token)
	if err != nil {
		return crmcontracts.MeetingInvitation{}, err
	}
	row := invitationRow{PublicIntent: intent.Public, ManagementRef: ref, Host: host, Provider: string(profile.Provider), Calendar: profile.CalendarId, ReminderStatus: "off", Status: invitationPending, Version: 1, Command: "create"}
	row.Appointment = connector.CalendarAppointment{
		ContactID: ids.UUID(in.ContactId), CalendarID: profile.CalendarId, Subject: in.Subject, Description: in.Description,
		Location: in.Location, Start: in.Start, End: in.End, Attendees: []string{string(in.AttendeeEmail)},
	}
	if profile.EmailReminder != nil && *profile.EmailReminder {
		row.Appointment.EmailReminder = true
		row.ReminderStatus = invitationPending
	}
	actor, _ := principal.Actor(ctx)
	row.Appointment.PassportID = actor.PassportID
	err = s.insertInvitation(ctx, &row, in, intent.ProposalID, profile.BufferMinutes, hash)
	if err != nil {
		err = errors.Join(err, s.deleteMeetingLink(ctx, ref))
	}
	if _, conflict := storekit.ExclusionViolation(err); conflict {
		err = &SlotTakenError{Start: in.Start}
	}
	if err != nil {
		return crmcontracts.MeetingInvitation{}, err
	}
	out := invitationView(row)
	out.ManagementToken = &token
	return out, nil
}

func reserveInvitation(ctx context.Context, tx pgx.Tx, host ids.UserID, start, end time.Time, buffer int, exclude ids.UUID) error {
	padding := time.Duration(buffer) * time.Minute
	busy, err := localMeetingSlots(ctx, tx, host, start.Add(-padding), end.Add(padding), exclude)
	if err != nil {
		return err
	}
	if overlapsAny(slot{Start: start.Add(-padding), End: end.Add(padding)}, busy) {
		return &SlotTakenError{Start: start}
	}
	return nil
}

func readInvitation(ctx context.Context, tx pgx.Tx, id ids.UUID, lock bool) (invitationRow, error) {
	args := schedulingArgs{}
	query := `SELECT activity_id,host_user_id,provider,calendar_id,appointment,receipt,status,version,command,attempts,management_ref,reminder_status,public_intent FROM meeting_invitation WHERE activity_id=` + args.add(id)
	if lock {
		query += " FOR UPDATE"
	}
	var row invitationRow
	err := tx.QueryRow(ctx, query, args...).Scan(&row.ID, &row.Host, &row.Provider, &row.Calendar, &row.Appointment, &row.Receipt, &row.Status, &row.Version, &row.Command, &row.Attempts, &row.ManagementRef, &row.ReminderStatus, &row.PublicIntent)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, apperrors.ErrNotFound
	}
	return row, err
}

func auditInvitation(ctx context.Context, tx pgx.Tx, before *invitationRow, after invitationRow) error {
	var audit ids.UUID
	var err error
	if before == nil {
		audit, err = storekit.AuditEvent(ctx, tx, "create", "activity", after.ID, map[string]any{auditInvitationStatus: after.Status, auditReminderStatus: after.ReminderStatus})
	} else {
		audit, err = storekit.Audit(ctx, tx, "update", "activity", after.ID, map[string]any{auditInvitationStatus: before.Status, auditReminderStatus: before.ReminderStatus}, map[string]any{auditInvitationStatus: after.Status, auditReminderStatus: after.ReminderStatus})
	}
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, audit, after.ID, crmcontracts.InternalEventMeetingInvitationUpdated{Status: after.Status, Version: after.Version})
}

// Invitation returns status only after checking the linked activity’s visibility.
func (s *Store) Invitation(ctx context.Context, id ids.UUID) (crmcontracts.MeetingInvitation, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return crmcontracts.MeetingInvitation{}, err
	}
	var row invitationRow
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if _, err := readActivity(ctx, tx, ids.From[ids.ActivityKind](id), storekit.LiveOnly); err != nil {
			return err
		}
		var err error
		row, err = readInvitation(ctx, tx, id, false)
		return err
	})
	return invitationView(row), err
}

func invitationSource(ctx context.Context) string {
	actor, _ := principal.Actor(ctx)
	if actor.ID == "system:public_booking" {
		return "public_booking"
	}
	return sourceManual
}

func (s *Store) insertInvitation(ctx context.Context, row *invitationRow, in crmcontracts.MeetingInvitationRequest, proposalID ids.UUID, buffer int, hash string) error {
	host := row.Host
	return s.tx(ctx, func(tx pgx.Tx) error {
		if proposalID != ids.Nil {
			if err := s.lockOpenProposal(ctx, tx, proposalID); err != nil {
				return err
			}
		}
		if err := storekit.LockWriteIdentity(ctx, tx, "meeting_host", host.String()); err != nil {
			return err
		}
		if err := admitPublicBookingIntent(ctx, tx, row.PublicIntent); err != nil {
			return err
		}
		if err := reserveInvitation(ctx, tx, host, in.Start, in.End, buffer, ids.Nil); err != nil {
			return err
		}
		seconds := int(in.End.Sub(in.Start) / time.Second)
		activity, _, err := s.LogActivityTx(ctx, tx, LogActivityInput{
			Kind: KindMeeting, Subject: &in.Subject, Body: &in.Description,
			OccurredAt: &in.Start, DurationSeconds: &seconds, HostUserID: &host, ClaimsHostSlot: true,
			Links: []ActivityLinkInput{{EntityType: linkEntityContact, EntityID: ids.UUID(in.ContactId)}}, Source: invitationSource(ctx),
		})
		if err != nil {
			return err
		}
		row.ID = ids.UUID(activity.Id)
		row.Appointment.RequestID = row.ID.String()
		args := schedulingArgs{}
		values := []string{args.add(row.ID), args.add(host), args.add(row.Provider), args.add(row.Calendar), args.add(row.Appointment), args.add(row.Status), args.add(hash), args.add(row.ManagementRef), args.add(s.now()), args.add(row.ReminderStatus), args.add(row.PublicIntent), args.add(s.now())}
		_, err = tx.Exec(ctx, `INSERT INTO meeting_invitation(activity_id,host_user_id,provider,calendar_id,appointment,status,management_hash,management_ref,next_attempt_at,reminder_status,public_intent,created_at) VALUES (`+strings.Join(values, ",")+`)`, args...)
		if err != nil {
			return err
		}
		if proposalID != ids.Nil {
			if err := consumeProposal(ctx, tx, proposalID, row.ID); err != nil {
				return err
			}
		}
		return auditInvitation(ctx, tx, nil, *row)
	})
}
