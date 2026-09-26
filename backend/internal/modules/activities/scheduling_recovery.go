// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func (s *Store) scopedInvitation(ctx context.Context, host ids.UserID, id ids.UUID) (invitationRow, error) {
	var row invitationRow
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if _, err := readActivity(ctx, tx, ids.From[ids.ActivityKind](id), storekit.LiveOnly); err != nil {
			return err
		}
		var err error
		row, err = readInvitation(ctx, tx, id, false)
		return err
	})
	if err == nil && row.Host != host {
		err = apperrors.ErrPermissionDenied
	}
	return row, err
}

func (s *Store) invitationPolicy(ctx context.Context, row invitationRow) (crmcontracts.SchedulingProfile, error) {
	profile, err := s.hostSchedulingProfile(ctx, row.Host)
	profile.Provider = crmcontracts.SchedulingProfileProvider(row.Provider)
	profile.CalendarId = row.Calendar
	return profile, err
}

func receiptEvent(row invitationRow) string {
	if row.Receipt != nil {
		return row.Receipt.EventID
	}
	return ""
}

func (s *Store) validateInvitationMove(ctx context.Context, host ids.UserID, id ids.UUID, start, end time.Time) (int, error) {
	row, err := s.scopedInvitation(ctx, host, id)
	if err != nil {
		return 0, err
	}
	profile, err := s.invitationPolicy(ctx, row)
	if err != nil {
		return 0, err
	}
	if err := s.validatePolicyTime(ctx, host, profile, start, end); err != nil {
		return 0, err
	}
	busy, err := s.calendarBusy(ctx, host, profile, start, end, receiptEvent(row))
	if err != nil {
		return 0, err
	}
	if overlapsAny(slot{Start: start, End: end}, busy) {
		return 0, &SlotTakenError{Start: start}
	}
	return profile.BufferMinutes, nil
}

func (s *Store) deliverInvitation(ctx context.Context, row invitationRow, in connector.CalendarAppointment) (*connector.CalendarReceipt, error) {
	// Reconcile before checking occupancy: an uncertain create may itself be busy.
	existing, err := s.calendar.Lookup(ctx, row.Host, row.Provider, in)
	if err != nil || existing != nil {
		return existing, err
	}
	if !in.Start.After(s.now()) {
		return nil, &SlotTakenError{Start: in.Start}
	}
	profile, err := s.invitationPolicy(ctx, row)
	if err != nil {
		return nil, err
	}
	// Notice was checked when the request was accepted; queue latency must not
	// invalidate that acceptance, but current hours and horizon still apply.
	profile.NoticeMinutes = 0
	if err := s.validatePolicyTime(ctx, row.Host, profile, in.Start, in.End); err != nil {
		return nil, err
	}
	busy, err := s.calendarBusy(ctx, row.Host, profile, in.Start, in.End, receiptEvent(row))
	if err != nil {
		return nil, err
	}
	if overlapsAny(slot{Start: in.Start, End: in.End}, busy) {
		return nil, &SlotTakenError{Start: in.Start}
	}
	link, err := s.openMeetingLink(ctx, row.ManagementRef)
	if err != nil {
		return nil, err
	}
	in.Description += "\n\nReschedule or cancel: " + link
	receipt, err := s.calendar.Save(ctx, row.Host, row.Provider, in)
	if err != nil {
		return nil, err
	}
	return &receipt, nil
}

func (s *Store) invitationAvailability(ctx context.Context, host ids.UserID, id ids.UUID, from, to time.Time) ([]slot, bool, error) {
	row, err := s.scopedInvitation(ctx, host, id)
	if err != nil {
		return nil, false, err
	}
	if row.Status != invitationConfirmed {
		return nil, false, apperrors.ErrConflict
	}
	if !to.After(from) || to.Sub(from) > maxAvailabilityWindow {
		return nil, false, errAvailabilityWindowTooWide
	}
	profile, err := s.invitationPolicy(ctx, row)
	if err != nil {
		return nil, false, err
	}
	earliest := s.now().Add(time.Duration(profile.NoticeMinutes) * time.Minute)
	latest := s.now().AddDate(0, 0, profile.HorizonDays)
	if from.Before(earliest) {
		from = earliest
	}
	if to.After(latest) {
		to = latest
	}
	hours, err := s.strictHours(ctx, host)
	if err != nil {
		return nil, false, err
	}
	busy, err := s.reliableBusy(ctx, host, profile, from, to, id, receiptEvent(row))
	if err != nil {
		return nil, false, err
	}
	free, truncated := policySlots(from, to, row.Appointment.End.Sub(row.Appointment.Start), busy, hours)
	return free, truncated, nil
}
