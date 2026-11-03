// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Who a calendar invitation goes to, beyond the one invitee it is filed on.
//
// An invitation carries one primary invitee (a contact or a lead) and up to
// 24 further addresses. Every address lands on the one provider event, and the
// provider sends each one the invite. The further addresses are resolved
// through the calendar seam's resolver (compose schedulingCalendar), which is
// the same resolver the provider save asks again before it sends.

import (
	"context"
	"net/mail"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// maxInvitationAttendees caps the addresses on one invitation, the primary included.
const maxInvitationAttendees = 25

const fieldAttendeeEmails = "attendee_emails"

// normalizedAttendees lower-cases, trims and de-duplicates addresses, and
// refuses a malformed one. skip names addresses already on the invitation.
func normalizedAttendees(addresses []string, skip ...string) ([]string, error) {
	seen := make(map[string]bool, len(addresses)+len(skip))
	for _, address := range skip {
		seen[strings.ToLower(strings.TrimSpace(address))] = true
	}
	out := make([]string, 0, len(addresses))
	for _, raw := range addresses {
		address := strings.ToLower(strings.TrimSpace(raw))
		parsed, err := mail.ParseAddress(address)
		if err != nil || parsed.Address != address {
			return nil, &SchedulingArgumentError{Field: fieldAttendeeEmails, Code: faultInvalid, Message: "Enter a valid email address for every attendee"}
		}
		if seen[address] {
			continue
		}
		seen[address] = true
		out = append(out, address)
	}
	if len(out)+len(skip) > maxInvitationAttendees {
		return nil, &SchedulingArgumentError{Field: fieldAttendeeEmails, Code: "too_many", Message: "Invite at most 25 attendees to one meeting"}
	}
	return out, nil
}

// furtherAttendees is the request's extra addresses, normalised against the primary.
func furtherAttendees(in crmcontracts.MeetingInvitationRequest) ([]string, error) {
	if in.AttendeeEmails == nil {
		return nil, nil
	}
	raw := make([]string, 0, len(*in.AttendeeEmails))
	for _, address := range *in.AttendeeEmails {
		raw = append(raw, string(address))
	}
	return normalizedAttendees(raw, string(in.AttendeeEmail))
}

// invitationRoster resolves the invitation's addresses: what each one is to the host.
// The primary is the invitee the request names; it was checked against its
// own record already, so only the further addresses are resolved here.
func (s *Store) invitationRoster(ctx context.Context, host ids.UserID, in crmcontracts.MeetingInvitationRequest, intent invitationIntent) ([]connector.CalendarAttendee, error) {
	primary := connector.CalendarAttendee{Address: string(in.AttendeeEmail), Basis: connector.AttendeeContact, ID: ids.UUID(in.ContactId), AskConsent: true}
	if !intent.Lead.IsZero() {
		primary.Basis, primary.ID = connector.AttendeeLead, intent.Lead.UUID
	}
	further, err := furtherAttendees(in)
	if err != nil {
		return nil, err
	}
	roster := []connector.CalendarAttendee{primary}
	if len(further) == 0 {
		return roster, nil
	}
	resolved, err := s.calendar.ResolveAttendees(ctx, host, further)
	if err != nil {
		return nil, err
	}
	return append(roster, resolved...), nil
}

// rosterAudit is the roster as the History tab shows it: each address with
// what it resolved to, so a host-named address reads as one.
func rosterAudit(roster []connector.CalendarAttendee) []string {
	out := make([]string, 0, len(roster))
	for _, attendee := range roster {
		out = append(out, attendee.Address+" ("+attendee.Basis+")")
	}
	return out
}

// rosterAddresses is the provider event's attendee list, in roster order.
func rosterAddresses(roster []connector.CalendarAttendee) []string {
	out := make([]string, 0, len(roster))
	for _, attendee := range roster {
		out = append(out, attendee.Address)
	}
	return out
}

// invitationLinks is the invitee's link, then the request's further links,
// each record once.
func invitationLinks(in crmcontracts.MeetingInvitationRequest, intent invitationIntent) []ActivityLinkInput {
	first := invitee(in, intent)
	out := []ActivityLinkInput{first}
	seen := map[ActivityLinkInput]bool{first: true}
	if in.Links == nil {
		return out
	}
	for _, link := range *in.Links {
		next := ActivityLinkInput{EntityType: string(link.EntityType), EntityID: ids.UUID(link.EntityId)}
		if seen[next] {
			continue
		}
		seen[next] = true
		out = append(out, next)
	}
	return out
}

// stampInvitationAttendees records who is in the room: the host as organizer
// and every invited address as attendee. A contact or colleague is written by
// id; a lead or a host-named address by the bare address, which is how
// capture records a party who is no contact here yet. A contact the host
// cannot see resolved as host-named, so its id never lands on this row.
func stampInvitationAttendees(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, host ids.UserID, roster []connector.CalendarAttendee) error {
	hostID := host.UUID
	if err := insertParticipantRow(ctx, tx, activityID, "organizer", &hostID, nil, ""); err != nil {
		return err
	}
	for _, attendee := range roster {
		id := attendee.ID
		var err error
		switch attendee.Basis {
		case connector.AttendeeContact:
			err = insertParticipantRow(ctx, tx, activityID, "attendee", nil, &id, "")
		case connector.AttendeeColleague:
			err = insertParticipantRow(ctx, tx, activityID, "attendee", &id, nil, "")
		default:
			err = insertParticipantRow(ctx, tx, activityID, "attendee", nil, nil, attendee.Address)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
