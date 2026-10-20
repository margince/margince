// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type channelPreviewRequest struct {
	crmcontracts.SendMessageRequest
	ActivityID *ids.UUID `json:"activity_id"`
}

func (r approvalPreviewReader) channelPreview(ctx context.Context, approval crmcontracts.Approval, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	out.Version, out.State = 2, previewIncompleteState
	request, err := commercialRequest[channelPreviewRequest](approval)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	in, err := activities.DecodeSendMessageRequest(request.SendMessageRequest)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Body) == "" || approval.TargetEntityType == nil || *approval.TargetEntityType != "activity" || approval.TargetEntityId == nil {
		return out, nil
	}
	if request.ActivityID != nil && *request.ActivityID != ids.UUID(*approval.TargetEntityId) {
		return out, nil
	}
	anchorID := ids.From[ids.ActivityKind](ids.UUID(*approval.TargetEntityId))
	reply, err := r.activities.PreviewChannelReply(ctx, anchorID)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	linked, err := r.previewLinkedRecords(ctx, reply.Links)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	files, err := r.previewFiles(ctx, request.AttachmentIds)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	contextDetails, err := r.communicationDetails(ctx, communicationPreviewClaim{category: in.Context, purpose: in.ConsentPurpose, marketing: in.MarketingPurpose, reason: in.OperatorReason, evidence: in.Evidence})
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	details := []crmcontracts.ApprovalPreviewDetail{
		previewDetail("channel_provider", reply.Provider),
		previewDetail("channel_account", reply.Recipient.ChannelUserID),
		previewDetail("body", in.Body),
	}
	if reply.Recipient.Username != "" {
		details = append(details, previewDetail("channel_username", reply.Recipient.Username))
	}
	details = append(details, linked...)
	details = append(details, contextDetails...)
	out.Details, out.Files, out.Effects = &details, &files, &[]string{"authorize_channel_reply"}
	out.State = previewReadyState
	if err := r.currentEvidence(ctx, approval, out); err != nil {
		return nil, err
	}
	return out, nil
}
