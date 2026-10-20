// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"strings"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (r approvalPreviewReader) bookingPreview(ctx context.Context, approval crmcontracts.Approval, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	out.Version, out.State = 2, previewIncompleteState
	in, err := commercialRequest[crmcontracts.BookMeetingJSONRequestBody](approval)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	host, err := bookingPreviewHost(approval, in)
	if err != nil {
		return out, nil
	}
	command := bookingPreviewCommand(in, host)
	call := agents.NewBookMeetingCall(r.records, nil, command)
	if err := call.Guards(ctx); err != nil {
		var invalid *agents.BadArgsError
		if previewUnavailable(err) || errors.As(err, &invalid) {
			return out, nil
		}
		return nil, err
	}
	subject, err := call.Subject(ctx)
	if err != nil {
		return nil, err
	}
	if approval.TargetEntityId == nil || approval.TargetEntityType == nil || subject.TargetID != ids.UUID(*approval.TargetEntityId) || subject.TargetType != *approval.TargetEntityType {
		return out, nil
	}
	hostValue, err := r.referenceValue(ctx, bulkParamOwner, host.String())
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	linked, err := r.bookingPreviewLinks(ctx, command.Links)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	details := []crmcontracts.ApprovalPreviewDetail{
		previewDetail("subject", command.Subject),
		previewDetail("meeting_start", in.Start.Format(time.RFC3339Nano)),
		previewDetail("meeting_end", in.End.Format(time.RFC3339Nano)),
		{Field: "meeting_host", Value: hostValue},
	}
	details = append(details, linked...)
	if in.AttendeeEmails != nil {
		for _, address := range *in.AttendeeEmails {
			details = append(details, previewDetail("intended_attendee", string(address)))
		}
	}
	out.Details, out.Effects = &details, &[]string{"record_meeting_only"}
	if err := r.appendBookingConsent(ctx, in, out); err != nil {
		if previewUnavailable(err) {
			return out, nil
		}
		return nil, err
	}
	out.State = previewReadyState
	if err := r.currentEvidence(ctx, approval, out); err != nil {
		return nil, err
	}
	return out, nil
}

func bookingPreviewHost(approval crmcontracts.Approval, in crmcontracts.BookMeetingJSONRequestBody) (ids.UUID, error) {
	if in.HostUserId != nil {
		return ids.UUID(*in.HostUserId), nil
	}
	if actor, ok := strings.CutPrefix(approval.ProposedBy, "human:"); ok {
		host, err := ids.Parse(actor)
		if err == nil {
			return host, nil
		}
	}
	return ids.Nil, apperrors.ErrInvalidArgument
}

func bookingPreviewCommand(in crmcontracts.BookMeetingJSONRequestBody, host ids.UUID) agents.BookMeetingCommand {
	out := agents.BookMeetingCommand{HostUserID: &host, Start: in.Start, End: in.End, Subject: activities.DefaultMeetingSubject}
	if in.Subject != nil && *in.Subject != "" {
		out.Subject = *in.Subject
	}
	for _, link := range in.Links {
		out.Links = append(out.Links, agents.RecordLink{EntityType: string(link.EntityType), EntityID: ids.UUID(link.EntityId)})
	}
	return out
}

func (r approvalPreviewReader) bookingPreviewLinks(ctx context.Context, requested []agents.RecordLink) ([]crmcontracts.ApprovalPreviewDetail, error) {
	unique, err := agents.UniqueRecordLinks(requested)
	if err != nil {
		return nil, err
	}
	links := make([]activities.ActivityLinkInput, 0, len(unique))
	for _, link := range unique {
		item := activities.ActivityLinkInput{EntityType: link.EntityType, EntityID: link.EntityID}
		if err := activities.CheckActivityLinkKind(activities.KindMeeting, item); err != nil {
			return nil, err
		}
		links = append(links, item)
	}
	return r.previewLinkedRecords(ctx, links)
}
