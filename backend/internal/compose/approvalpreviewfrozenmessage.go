// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func (r approvalPreviewReader) populateFrozenMessage(ctx context.Context, input activities.SendEmailInput, links []activities.ActivityLinkInput, out *crmcontracts.ApprovalPreview) error {
	to := activities.ToRecipients(input.Recipients, input.Cc, input.Bcc)
	if len(to) == 0 || strings.TrimSpace(input.Body) == "" {
		return apperrors.ErrInvalidArgument
	}
	contextDetails, err := r.communicationDetails(ctx, communicationPreviewClaim{category: input.Context, purpose: input.ConsentPurpose, marketing: input.MarketingPurpose, reason: input.OperatorReason, evidence: input.Evidence})
	if err != nil {
		return err
	}
	details := mailPreviewDetails(crmcontracts.SendCompanyEmailRequest{}, input, to)
	filing, err := r.previewLinkedRecords(ctx, links)
	if err != nil {
		return err
	}
	details = append(details, filing...)
	details = append(details, contextDetails...)
	attachments := make([]openapi_types.UUID, 0, len(input.AttachmentIDs))
	for _, id := range input.AttachmentIDs {
		attachments = append(attachments, openapi_types.UUID(id))
	}
	files, err := r.previewFiles(ctx, &attachments)
	if err != nil {
		return err
	}
	if input.HTMLBody != "" {
		html, err := activities.SanitizeOutboundHTML(input.HTMLBody)
		if err != nil {
			return apperrors.ErrInvalidArgument
		}
		details = append(details, previewDetail("html_body", html))
	}
	out.Details, out.Files = &details, &files
	return nil
}
