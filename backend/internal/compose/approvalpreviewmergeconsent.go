// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

func (r approvalPreviewReader) mergeConsentDetails(ctx context.Context, scope contacts.MergeSurvivorship) ([]crmcontracts.ApprovalPreviewDetail, error) {
	details, err := r.promotionCarryDetails(ctx, contacts.LeadPromotionCarry{Consent: scope.Consent.States, Stops: scope.Stops, ConsentLinks: scope.ConsentLinks}, false)
	if err != nil {
		return nil, err
	}
	for _, override := range scope.Overrides {
		details = append(details, mergeIdentityDetail("merge_override", override.Category, override.ID), previewDetail("merge_override_reason", override.Reason), previewDetail("merge_override_authority", override.Authority))
	}
	for _, link := range scope.PreferenceLinks {
		details = append(details, mergeIdentityDetail("merge_preference_link", "", link.ID), previewMoment("promotion_link_issued", &link.CreatedAt), previewMoment("promotion_link_expires", &link.ExpiresAt), previewRevoked(link.RevokedAt), mergeBooleanDetail("merge_preference_retired", link.WillRetire))
		if link.RevokedReason != nil {
			details = append(details, previewDetail("merge_preference_reason", *link.RevokedReason))
		}
	}
	return details, nil
}

func (r approvalPreviewReader) consentConfirmationDetails(ctx context.Context, links commsauthz.CarriedConsentLinks) ([]crmcontracts.ApprovalPreviewDetail, error) {
	details := []crmcontracts.ApprovalPreviewDetail{}
	for _, confirmation := range links.Confirmations {
		details = append(details, mergeIdentityDetail("merge_confirmation", confirmation.Address, confirmation.ID), previewMoment("promotion_link_issued", &confirmation.IssuedAt), previewMoment("promotion_link_expires", &confirmation.ExpiresAt), previewMoment("merge_link_opened", confirmation.OpenedAt), previewMoment("merge_link_consumed", confirmation.ConsumedAt))
	}
	for _, optin := range links.DoubleOptIns {
		details = append(details, mergeIdentityDetail("merge_optin", optin.PurposeLabel, optin.ID), previewMoment("promotion_link_issued", &optin.IssuedAt), previewMoment("promotion_link_expires", &optin.ExpiresAt), previewMoment("merge_link_consumed", optin.ConsumedAt))
	}
	for _, event := range links.QualifyingEvents {
		details = append(details, mergeIdentityDetail("merge_consent_event", event.Kind, event.ID), previewMoment("merge_consent_event_at", &event.OccurredAt), mergeBooleanDetail("merge_consent_event_collapsed", event.Collapsed))
		if event.Note != nil {
			details = append(details, previewDetail("merge_consent_event_note", *event.Note))
		}
		if event.SourceID != nil && event.SourceType != nil {
			source, err := r.referenceValue(ctx, *event.SourceType+"_id", event.SourceID.String())
			if err != nil {
				return nil, err
			}
			details = append(details, crmcontracts.ApprovalPreviewDetail{Field: "merge_consent_event_source", Value: source})
		}
	}
	return details, nil
}
