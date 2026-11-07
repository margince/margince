// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

var errBookingInviteeRequired = &SchedulingArgumentError{
	Field: "links", Code: "invitee_required",
	Message: "Name who the meeting is with: link a contact or a lead with an email address, or add the email of a contact you can see. " +
		"To block time without inviting anyone, log a meeting instead",
}

// bookingInvitation maps a booking onto the invitation engine's request. The
// invitee is the first contact link, else the first lead link, else a contact
// an attendee address resolves to.
func (s *Store) bookingInvitation(ctx context.Context, in BookMeetingInput) (crmcontracts.MeetingInvitationRequest, invitationIntent, error) {
	subject := in.Subject
	if subject == "" {
		subject = DefaultMeetingSubject
	}
	addresses, err := normalizedAttendees(in.AttendeeEmails)
	if err != nil {
		return crmcontracts.MeetingInvitationRequest{}, invitationIntent{}, err
	}
	req := crmcontracts.MeetingInvitationRequest{Start: in.Start, End: in.End, Subject: subject, Description: subject}
	links := make([]crmcontracts.MeetingRecordLink, 0, len(in.Links))
	for _, link := range in.Links {
		links = append(links, crmcontracts.MeetingRecordLink{EntityType: crmcontracts.MeetingRecordLinkEntityType(link.EntityType), EntityId: openapi_types.UUID(link.EntityID)})
	}
	req.Links = &links
	inviteeID, primary, lead, err := s.bookingInvitee(ctx, in, addresses)
	if err != nil {
		return req, invitationIntent{}, err
	}
	req.ContactId, req.AttendeeEmail = crmcontracts.Id(inviteeID), openapi_types.Email(primary)
	further := make([]openapi_types.Email, 0, len(addresses))
	for _, address := range addresses {
		if address != primary {
			further = append(further, openapi_types.Email(address))
		}
	}
	req.AttendeeEmails = &further
	return req, invitationIntent{Lead: lead}, nil
}

func (s *Store) bookingInvitee(ctx context.Context, in BookMeetingInput, addresses []string) (ids.UUID, string, ids.LeadID, error) {
	if s.calendar == nil {
		return ids.Nil, "", ids.LeadID{}, apperrors.ErrPermissionDenied
	}
	for _, kind := range []string{linkEntityContact, linkEntityLead} {
		for _, link := range in.Links {
			if link.EntityType != kind {
				continue
			}
			address, contact, err := s.calendar.InviteeAddress(ctx, in.Host, kind, link.EntityID, addresses)
			if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) || (err == nil && address == "") {
				return ids.Nil, "", ids.LeadID{}, errBookingInviteeRequired
			}
			if err != nil {
				return ids.Nil, "", ids.LeadID{}, err
			}
			if !contact.IsZero() {
				return contact, address, ids.LeadID{}, nil
			}
			return link.EntityID, address, ids.From[ids.LeadKind](link.EntityID), nil
		}
	}
	if len(addresses) > 0 {
		resolved, err := s.calendar.ResolveAttendees(ctx, in.Host, addresses)
		if err != nil {
			return ids.Nil, "", ids.LeadID{}, err
		}
		for _, attendee := range resolved {
			if attendee.Basis == connector.AttendeeContact {
				return attendee.ID, attendee.Address, ids.LeadID{}, nil
			}
		}
	}
	return ids.Nil, "", ids.LeadID{}, errBookingInviteeRequired
}
