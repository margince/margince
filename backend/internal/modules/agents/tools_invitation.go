// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/baselanguage"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// MeetingInviter queues real invitations through the shared scheduling engine.
type MeetingInviter interface {
	InviteMeeting(context.Context, crmcontracts.MeetingInvitationRequest) (crmcontracts.MeetingInvitation, error)
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

var inviteMeetingCopy = toolCopy{Purpose: "Create a real calendar invitation for one contact and notify their chosen address.", Limits: "Requires a connected writable calendar and a contact email the acting host may use. It checks actual busy time and returns pending until the provider confirms. Does not prove the guest agreed or received the notification.", Instead: "Use book_meeting only to record a meeting without inviting anyone. Use check_availability with reliable=true to propose calendar-checked times.", Retain: "Keep the invitation id and inspect its status before claiming it is booked. Retrying an uncertain delivery must use the existing invitation."}

func (t inviteMeetingTool) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "invite_meeting", Title: "Send a calendar invitation", Version: toolVersionV1, RequiredScope: principal.ScopeSend, Tier: mcp.TierConfirmationRequired, Egress: true, OpenAPIOp: "createMeetingInvitation",
		Description: inviteMeetingCopy.render(),
		Instead:     inviteMeetingCopy.Instead,
		InputSchema: schemaFor[crmcontracts.MeetingInvitationRequest](), OutputSchema: schemaFor[crmcontracts.MeetingInvitation](),
	}
}

func (t inviteMeetingTool) StageInfo(ctx context.Context, raw json.RawMessage) (StageInfo, error) {
	var in crmcontracts.MeetingInvitationRequest
	if err := decodeArgs(raw, &in); err != nil {
		return StageInfo{}, err
	}
	return StageSubject(ctx, NewMeetingInvitationCall(t.records, t.language, in))
}

func (t inviteMeetingTool) Handle(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	var in crmcontracts.MeetingInvitationRequest
	if err := decodeArgs(raw, &in); err != nil {
		return nil, err
	}
	if err := NewMeetingInvitationCall(t.records, t.language, in).Guards(ctx); err != nil {
		return nil, err
	}
	noteEvidence(ctx, datasource.EntityContact, ids.UUID(in.ContactId))
	out, err := t.inviter.InviteMeeting(ctx, in)
	out.ManagementToken = nil
	return marshalResult(out, err)
}

type invitationResolver struct{ booking *bookMeetingResolver }

// NewMeetingInvitationCall shares staging and guards between web and tool transports.
//
//nolint:ireturn // both transports bind the same governance resolver.
func NewMeetingInvitationCall(records datasource.SystemOfRecordProvider, language baselanguage.Resolver, in crmcontracts.MeetingInvitationRequest) GovernedCall {
	return bind[crmcontracts.MeetingInvitationRequest](invitationResolver{&bookMeetingResolver{links: namedLinks{records: records}, language: language}}, in)
}

func invitationBooking(in crmcontracts.MeetingInvitationRequest) BookMeetingCommand {
	return BookMeetingCommand{Start: in.Start, End: in.End, Subject: in.Subject, Links: []RecordLink{{EntityType: importObjectContact, EntityID: ids.UUID(in.ContactId)}}}
}

func (r invitationResolver) Guards(ctx context.Context, in crmcontracts.MeetingInvitationRequest) error {
	if err := httperr.RequireBodyID("contact_id", ids.UUID(in.ContactId)); err != nil {
		return err
	}
	return r.booking.Guards(ctx, invitationBooking(in))
}

func (r invitationResolver) Subject(ctx context.Context, in crmcontracts.MeetingInvitationRequest) (StageInfo, error) {
	out, err := r.booking.Subject(ctx, invitationBooking(in))
	out.Summary += " → " + string(in.AttendeeEmail) + " · " + in.Location + "\n\n" + in.Description
	return out, err
}
