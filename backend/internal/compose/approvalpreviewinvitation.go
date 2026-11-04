// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"strconv"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func (r approvalPreviewReader) invitationPreview(ctx context.Context, approval crmcontracts.Approval, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	out.Version, out.State = 2, previewIncompleteState
	in, err := commercialRequest[crmcontracts.MeetingInvitationRequest](approval)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	host, err := bookingPreviewHost(approval, crmcontracts.BookMeetingJSONRequestBody{})
	if previewUnavailable(err) {
		return out, nil
	}
	call := agents.NewMeetingInvitationCall(r.records, nil, nil, in)
	if err := call.Guards(ctx); err != nil {
		var invalid *agents.BadArgsError
		if previewUnavailable(err) || errors.As(err, &invalid) {
			return out, nil
		}
		return nil, err
	}
	if approval.TargetEntityId == nil || approval.TargetEntityType == nil || *approval.TargetEntityType != string(recordTypeContact) || *approval.TargetEntityId != in.ContactId {
		return out, nil
	}
	appointment, err := r.activities.PreviewInvitation(ctx, ids.From[ids.UserKind](host), in)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	hostValue, contact, err := r.invitationParticipants(ctx, host, in)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	details := []crmcontracts.ApprovalPreviewDetail{
		previewDetail(activityTargetSubject, appointment.Subject),
		previewDetail("invitation_description", appointment.Description),
		previewDetail("meeting_location", appointment.Location),
		previewDetail("meeting_start", appointment.Start.Format(time.RFC3339Nano)),
		previewDetail("meeting_end", appointment.End.Format(time.RFC3339Nano)),
		{Field: "meeting_host", Value: hostValue},
		{Field: "filed_under", Value: contact},
		{Field: "video_call", Value: crmcontracts.ApprovalPreviewValue{Kind: previewValueBoolean, Text: strconv.FormatBool(appointment.VideoCall)}},
		{Field: "email_reminder", Value: crmcontracts.ApprovalPreviewValue{Kind: previewValueBoolean, Text: strconv.FormatBool(appointment.EmailReminder)}},
	}
	details = append(details, invitationRecipients(in, appointment)...)
	out.Details, out.Effects, out.State = &details, &[]string{"authorize_calendar_invitation"}, previewReadyState
	return out, r.currentEvidence(ctx, approval, out)
}

func (r approvalPreviewReader) invitationParticipants(ctx context.Context, host ids.UUID, in crmcontracts.MeetingInvitationRequest) (crmcontracts.ApprovalPreviewValue, crmcontracts.ApprovalPreviewValue, error) {
	hostValue, err := r.referenceValue(ctx, bulkParamOwner, host.String())
	if previewUnavailable(err) {
		return hostValue, crmcontracts.ApprovalPreviewValue{}, err
	}
	if err != nil {
		return hostValue, crmcontracts.ApprovalPreviewValue{}, err
	}
	contact, err := r.referenceValue(ctx, companionContactField, ids.UUID(in.ContactId).String())
	if previewUnavailable(err) {
		return hostValue, crmcontracts.ApprovalPreviewValue{}, err
	}
	if err != nil {
		return hostValue, crmcontracts.ApprovalPreviewValue{}, err
	}
	return hostValue, contact, nil
}

// invitationRecipients names every address the approval sends the invite to.
// An address no record the host can read stands behind gets its own field.
// It is the same whether or not a record hidden from the host holds it.
func invitationRecipients(in crmcontracts.MeetingInvitationRequest, appointment connector.CalendarAppointment) []crmcontracts.ApprovalPreviewDetail {
	if len(appointment.Roster) == 0 {
		return []crmcontracts.ApprovalPreviewDetail{previewDetail("to", string(in.AttendeeEmail))}
	}
	out := make([]crmcontracts.ApprovalPreviewDetail, 0, len(appointment.Roster))
	for _, attendee := range appointment.Roster {
		field := "to"
		if attendee.Basis == connector.AttendeeHostNamed {
			field = previewHostNamedAttendee
		}
		out = append(out, previewDetail(field, attendee.Address))
	}
	return out
}
