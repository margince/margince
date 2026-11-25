// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"strings"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
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
	if err := appendMailSchedule(message, out); err != nil {
		return nil, err
	}
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
	return mailApprovalOrigin(ctx, r.activities, approval, message)
}

func mailApprovalOrigin(ctx context.Context, store *activities.Store, approval crmcontracts.Approval, message crmcontracts.SendCompanyEmailRequest) (activities.SendOrigin, []activities.ActivityLinkInput, error) {
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
	links, err := origin.Resolve(ctx, store)
	return origin, links, err
}

func mailPreviewDetails(input activities.SendEmailInput, to []string) []crmcontracts.ApprovalPreviewDetail {
	details := []crmcontracts.ApprovalPreviewDetail{
		previewDetail("to", strings.Join(to, "\n")),
		previewDetail("cc", strings.Join(input.Cc, "\n")),
		previewDetail("bcc", strings.Join(input.Bcc, "\n")),
		previewDetail(activityTargetSubject, input.Subject),
		previewDetail("body", input.Body),
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
		if !line.Missing() {
			*out.Details = append(*out.Details, previewDetail("disclosure_text", line.Text))
		}
	}
	for _, finding := range footer.Findings {
		*out.Details = append(*out.Details, previewDetail("footer_warning", mailFooterFinding(finding)))
	}
	if footer.OffersUnsubscribe {
		*out.Effects = append(*out.Effects, "append_recipient_unsubscribe")
	}
	return nil
}

func appendMailSchedule(message crmcontracts.SendCompanyEmailRequest, out *crmcontracts.ApprovalPreview) error {
	schedule, err := activities.ParseSendSchedule(message.ScheduledAt, message.ScheduledTz, message.ScheduledIntent)
	if err != nil {
		return err
	}
	if schedule == nil {
		return nil
	}
	*out.Details = append(*out.Details, previewDetail("scheduled_at", schedule.At.Format(time.RFC3339)), previewDetail("scheduled_tz", schedule.TZ))
	return nil
}

func mailFooterFinding(finding commsauthz.FooterFinding) string {
	label := "some company footer text"
	switch finding.Kind {
	case "objection_route":
		label = "opt-out instructions"
	case "privacy_contact":
		label = "privacy contact information"
	case "controller_identity":
		label = "the company name and address"
	case "advertiser_contact":
		label = "advertiser contact information"
	case "advertisement":
		label = "the line that identifies it as an advertisement"
	case "recipient_private_link":
		label = "a private opt-out link, to protect each recipient's private information"
	case "public_opt_out_link":
		label = "an opt-out link because the company has not configured a working public address"
	case "unsubscribe_link":
		label = "an opt-out link because the send could be given no stop link"
	}
	if finding.Language != "" {
		label += " in " + finding.Language
	}
	return fmt.Sprintf("The email will omit %s. Sending requires a recorded reason.", label)
}
