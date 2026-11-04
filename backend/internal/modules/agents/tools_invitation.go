// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/baselanguage"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// MeetingInviter queues real invitations through the shared scheduling engine.
type MeetingInviter interface {
	InviteMeeting(context.Context, crmcontracts.MeetingInvitationRequest) (crmcontracts.MeetingInvitation, error)
	InvitationRoster
}

// InvitationRoster resolves a request's addresses as delivery will, so the
// staged summary can mark the ones no consent check clears.
type InvitationRoster interface {
	InvitationRoster(context.Context, crmcontracts.MeetingInvitationRequest) ([]connector.CalendarAttendee, error)
}
type inviteMeetingTool struct {
	inviter  MeetingInviter
	records  datasource.SystemOfRecordProvider
	language baselanguage.Resolver
}

// RegisterMeetingInvitationTool adds the explicitly approved calendar-send boundary.
func RegisterMeetingInvitationTool(r *Registry, inviter MeetingInviter, records datasource.SystemOfRecordProvider) {
	r.Register(inviteMeetingTool{inviter, records, r.language})
}

var inviteMeetingCopy = toolCopy{Purpose: "Create a real calendar invitation for one contact, plus any further attendees, and have the provider notify every one.", Limits: "Requires a connected writable calendar and a contact email the acting host may use. It checks actual busy time and returns pending until the provider confirms. Does not prove the guest agreed or received the notification.", Instead: "Use log_activity to record a meeting without inviting anyone. Use check_availability with reliable=true to propose calendar-checked times.", Retain: "Keep the invitation id and inspect its status before claiming it is booked. Retrying an uncertain delivery must use the existing invitation."}

func (t inviteMeetingTool) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "invite_meeting", Title: "Send a calendar invitation", Version: toolVersionV1, RequiredScope: principal.ScopeSend, Tier: mcp.TierConfirmationRequired, Egress: true, AskUserFirst: true, OpenAPIOp: "createMeetingInvitation",
		Unlisted:    true,
		Description: inviteMeetingCopy.render(),
		Instead:     inviteMeetingCopy.Instead,
		InputSchema: argumentSchemaFor[crmcontracts.MeetingInvitationRequest](windowRule), OutputSchema: schemaFor[crmcontracts.MeetingInvitation](),
	}
}

func (t inviteMeetingTool) StageInfo(ctx context.Context, raw json.RawMessage) (StageInfo, error) {
	var in crmcontracts.MeetingInvitationRequest
	if err := decodeArgs(raw, &in); err != nil {
		return StageInfo{}, err
	}
	return StageSubject(ctx, NewMeetingInvitationCall(t.records, t.language, t.inviter, in))
}

func (t inviteMeetingTool) Handle(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	var in crmcontracts.MeetingInvitationRequest
	if err := decodeArgs(raw, &in); err != nil {
		return nil, err
	}
	if err := NewMeetingInvitationCall(t.records, t.language, t.inviter, in).Guards(ctx); err != nil {
		return nil, err
	}
	noteEvidence(ctx, datasource.EntityContact, ids.UUID(in.ContactId))
	out, err := t.inviter.InviteMeeting(ctx, in)
	out.ManagementToken = nil
	return marshalResult(out, err)
}

type invitationResolver struct {
	booking *bookMeetingResolver
	roster  InvitationRoster
}

// NewMeetingInvitationCall shares staging and guards between web and tool transports.
//
// A nil roster stages the plain address list.
//
//nolint:ireturn // both transports bind the same governance resolver.
func NewMeetingInvitationCall(records datasource.SystemOfRecordProvider, language baselanguage.Resolver, roster InvitationRoster, in crmcontracts.MeetingInvitationRequest) GovernedCall {
	return bind[crmcontracts.MeetingInvitationRequest](invitationResolver{booking: &bookMeetingResolver{links: namedLinks{records: records}, language: language}, roster: roster}, in)
}

// invitationBooking reads every record the invitation files on, so the guards
// probe the further links under the caller's row scope too.
func invitationBooking(in crmcontracts.MeetingInvitationRequest) BookMeetingCommand {
	links := []RecordLink{{EntityType: importObjectContact, EntityID: ids.UUID(in.ContactId)}}
	if in.Links != nil {
		for _, link := range *in.Links {
			links = append(links, RecordLink{EntityType: string(link.EntityType), EntityID: ids.UUID(link.EntityId)})
		}
	}
	return BookMeetingCommand{Start: in.Start, End: in.End, Subject: in.Subject, Links: links}
}

// invitationRecipients lists the invitation's addresses, primary first. With
// the resolved roster, an address no visible record stands behind is marked as
// entered. A hidden record holding it changes nothing.
func (r invitationResolver) invitationRecipients(ctx context.Context, in crmcontracts.MeetingInvitationRequest) (string, error) {
	recipients := []string{string(in.AttendeeEmail)}
	if in.AttendeeEmails != nil {
		for _, address := range *in.AttendeeEmails {
			recipients = append(recipients, string(address))
		}
	}
	if r.roster == nil {
		return strings.Join(recipients, ", "), nil
	}
	roster, err := r.roster.InvitationRoster(ctx, in)
	if err != nil {
		return "", err
	}
	said := summaryIn(ctx, r.booking.language)
	marked := make([]string, 0, len(roster))
	for _, attendee := range roster {
		if attendee.Basis == connector.AttendeeHostNamed {
			marked = append(marked, fmt.Sprintf(said.hostNamed, attendee.Address))
			continue
		}
		marked = append(marked, attendee.Address)
	}
	if len(marked) == 0 {
		marked = recipients
	}
	return strings.Join(marked, ", "), nil
}

func (r invitationResolver) Guards(ctx context.Context, in crmcontracts.MeetingInvitationRequest) error {
	if err := httperr.RequireBodyID("contact_id", ids.UUID(in.ContactId)); err != nil {
		return err
	}
	return r.booking.Guards(ctx, invitationBooking(in))
}

func (r invitationResolver) Subject(ctx context.Context, in crmcontracts.MeetingInvitationRequest) (StageInfo, error) {
	out, err := r.booking.Subject(ctx, invitationBooking(in))
	if err != nil {
		return out, err
	}
	recipients, err := r.invitationRecipients(ctx, in)
	out.Summary += " → " + recipients + " · " + in.Location + "\n\n" + in.Description
	return out, err
}
