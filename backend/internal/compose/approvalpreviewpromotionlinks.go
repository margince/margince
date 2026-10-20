// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"strconv"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func (r approvalPreviewReader) promotionConsentLinkDetails(ctx context.Context, links commsauthz.CarriedConsentLinks) ([]crmcontracts.ApprovalPreviewDetail, error) {
	details := []crmcontracts.ApprovalPreviewDetail{}
	for _, link := range links.WithdrawalCredentials {
		address := previewDetail("promotion_withdrawal_address", link.Address)
		address.Value.EntityId = new(openapi_types.UUID(link.ID))
		details = append(details, address)
		purpose := previewDetail("promotion_restriction_purpose", "")
		if link.Scope == "named_purpose" {
			if link.PurposeLabel == nil || *link.PurposeLabel == "" {
				return nil, apperrors.ErrInvalidArgument
			}
			purpose = previewDetail("promotion_restriction_purpose", *link.PurposeLabel)
		} else if link.Scope != "all_marketing" {
			return nil, apperrors.ErrInvalidArgument
		}
		details = append(details, purpose, previewMoment("promotion_link_issued", &link.IssuedAt), previewMoment("promotion_link_expires", &link.ExpiresAt), previewRevoked(link.RevokedAt))
	}
	for _, basis := range links.CommunicationBases {
		kind := previewDetail("promotion_basis_kind", basis.Kind)
		kind.Value.EntityId = new(openapi_types.UUID(basis.ID))
		details = append(details, kind, previewMoment("promotion_basis_from", &basis.ValidFrom), previewMoment("promotion_basis_until", basis.ValidUntil), previewRevoked(basis.RevokedAt))
		if basis.SourceActivityID != nil {
			value, err := r.referenceValue(ctx, "activity_id", basis.SourceActivityID.String())
			if err != nil {
				return nil, err
			}
			details = append(details, crmcontracts.ApprovalPreviewDetail{Field: "promotion_basis_activity", Value: value})
		}
	}
	confirmations, err := r.consentConfirmationDetails(ctx, links)
	if err != nil {
		return nil, err
	}
	return append(details, confirmations...), nil
}

func previewMoment(field string, moment *time.Time) crmcontracts.ApprovalPreviewDetail {
	detail := previewDetail(field, "")
	if moment != nil {
		detail.Value.Kind, detail.Value.Text = previewValueDate, moment.Format(time.RFC3339Nano)
	}
	return detail
}

func previewRevoked(moment *time.Time) crmcontracts.ApprovalPreviewDetail {
	return crmcontracts.ApprovalPreviewDetail{Field: "promotion_record_revoked", Value: crmcontracts.ApprovalPreviewValue{Kind: previewValueBoolean, Text: strconv.FormatBool(moment != nil)}}
}
