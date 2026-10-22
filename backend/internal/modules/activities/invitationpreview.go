// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func invitationAppointment(profile crmcontracts.SchedulingProfile, in crmcontracts.MeetingInvitationRequest) connector.CalendarAppointment {
	return connector.CalendarAppointment{
		ContactID: ids.UUID(in.ContactId), CalendarID: profile.CalendarId, Subject: in.Subject, Description: in.Description,
		Location: in.Location, Start: in.Start, End: in.End, Attendees: []string{string(in.AttendeeEmail)},
		VideoCall: videoCallFor(profile, in.VideoCall), EmailReminder: profile.EmailReminder != nil && *profile.EmailReminder,
	}
}

// PreviewInvitation reads the proposed delivery without reserving time or contacting a provider.
func (s *Store) PreviewInvitation(ctx context.Context, host ids.UserID, in crmcontracts.MeetingInvitationRequest) (connector.CalendarAppointment, error) {
	actor, err := schedulingHost(ctx)
	if err != nil {
		return connector.CalendarAppointment{}, err
	}
	if actor != host {
		if err := auth.RequireAdmin(ctx); err != nil {
			return connector.CalendarAppointment{}, err
		}
	}
	if err := validateInvitation(in); err != nil {
		return connector.CalendarAppointment{}, err
	}
	if !in.End.After(in.Start) {
		return connector.CalendarAppointment{}, errBookingEndNotAfterStart
	}
	profile, err := s.hostSchedulingProfile(ctx, host)
	if err != nil {
		return connector.CalendarAppointment{}, err
	}
	if profile.Provider == "" || profile.CalendarId == "" {
		return connector.CalendarAppointment{}, apperrors.ErrInvalidArgument
	}
	return invitationAppointment(profile, in), nil
}
