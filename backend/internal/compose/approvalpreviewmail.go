// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"strings"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	previewSendEmail        = "send_email"
	previewSendCompanyEmail = "send_company_email"
)

func (r approvalPreviewReader) mailPreview(ctx context.Context, approval crmcontracts.Approval, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	out.Version, out.State = 2, previewIncompleteState
	message, err := commercialRequest[crmcontracts.SendCompanyEmailRequest](approval)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	input, err := activities.DecodeCompanyEmailRequest(message)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if approval.Kind == previewSendCompanyEmail && len(message.Links) == 0 {
		return out, nil
	}
	origin, links, err := r.mailPreviewOrigin(ctx, approval, message)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if err := r.populateFrozenMessage(ctx, input, links, out); err != nil {
		if previewUnavailable(err) {
			return out, nil
		}
		return nil, err
	}
	appendMailSchedule(message, out)
	out.Effects = &[]string{"authorize_message_send"}
	if err := r.appendMailPresentation(ctx, approval, origin, links, input, out); err != nil {
		if previewUnavailable(err) {
			return out, nil
		}
		return nil, err
	}
	out.State = previewReadyState
	return out, r.currentEvidence(ctx, approval, out)
}

// mailPreviewOrigin is where the held message would be sent from, and the
// records that origin files it under.
func (r approvalPreviewReader) mailPreviewOrigin(ctx context.Context, approval crmcontracts.Approval, message crmcontracts.SendCompanyEmailRequest) (activities.SendOrigin, []activities.ActivityLinkInput, error) {
	var origin activities.SendOrigin
	if approval.Kind == previewSendEmail {
		if approval.TargetEntityType == nil || *approval.TargetEntityType != string(recordTypeActivity) || approval.TargetEntityId == nil {
			return activities.SendOrigin{}, nil, apperrors.ErrInvalidArgument
		}
		origin = activities.FromActivity(ids.From[ids.ActivityKind](ids.UUID(*approval.TargetEntityId)))
	} else {
		links := make([]activities.ActivityLinkInput, 0, len(message.Links))
		for _, link := range message.Links {
			links = append(links, activities.ActivityLinkInput{EntityType: string(link.EntityType), EntityID: ids.UUID(link.EntityId)})
		}
		origin = activities.FromAccount(links)
	}
	links, err := origin.Resolve(ctx, r.activities)
	return origin, links, err
}

func mailPreviewDetails(message crmcontracts.SendCompanyEmailRequest, input activities.SendEmailInput, to []string) []crmcontracts.ApprovalPreviewDetail {
	details := []crmcontracts.ApprovalPreviewDetail{
		previewDetail("to", strings.Join(to, "\n")),
		previewDetail("cc", strings.Join(input.Cc, "\n")),
		previewDetail("bcc", strings.Join(input.Bcc, "\n")),
		previewDetail(activityTargetSubject, input.Subject),
		previewDetail("body", input.Body),
	}
	if message.ScheduledAt != nil {
		details = append(details, previewDetail("scheduled_at", message.ScheduledAt.Format(time.RFC3339)))
	}
	if message.ScheduledTz != nil {
		details = append(details, previewDetail("scheduled_tz", *message.ScheduledTz))
	}
	return details
}

// appendMailFooter previews the footer under the category the send will use,
// which the consent engine may widen to marketing for one recipient.
func (r approvalPreviewReader) appendMailFooter(ctx context.Context, origin activities.SendOrigin, links []activities.ActivityLinkInput, in activities.SendEmailInput, out *crmcontracts.ApprovalPreview) error {
	footer, err := r.activities.PreviewMailFooter(ctx, origin, links, in, consentGateOn(r.db))
	if err != nil {
		return err
	}
	for _, line := range footer.Disclosures {
		*out.Details = append(*out.Details, previewDetail("disclosure_text", line.Text))
	}
	if footer.OffersUnsubscribe {
		*out.Effects = append(*out.Effects, "append_recipient_unsubscribe")
	}
	return nil
}

func appendMailSchedule(message crmcontracts.SendCompanyEmailRequest, out *crmcontracts.ApprovalPreview) {
	if message.ScheduledAt != nil {
		*out.Details = append(*out.Details, previewDetail("scheduled_at", message.ScheduledAt.Format(time.RFC3339)))
	}
	if message.ScheduledTz != nil {
		*out.Details = append(*out.Details, previewDetail("scheduled_tz", *message.ScheduledTz))
	}
}
