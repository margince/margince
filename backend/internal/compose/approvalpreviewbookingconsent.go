// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (r approvalPreviewReader) appendBookingConsent(ctx context.Context, in crmcontracts.BookMeetingJSONRequestBody, out *crmcontracts.ApprovalPreview) error {
	if in.Consent == nil {
		return nil
	}
	links := make([]activities.ActivityLinkInput, 0, len(in.Links))
	for _, link := range in.Links {
		links = append(links, activities.ActivityLinkInput{EntityType: string(link.EntityType), EntityID: ids.UUID(link.EntityId)})
	}
	store := consent.NewStore(r.db)
	contact, request, err := activities.PrepareBookingConsent(ctx, *in.Consent, links, bookingConsentAdapter{store: store})
	if err != nil {
		return err
	}
	subject, err := r.referenceValue(ctx, companionContactField, contact.String())
	if err != nil {
		return err
	}
	purposes, err := store.ListPurposes(ctx)
	if err != nil {
		return err
	}
	purpose, err := bookingPurposeLabel(purposes, request.PurposeID)
	if err != nil {
		return err
	}
	decision, err := store.PreviewInitialGrant(ctx, bookingConsentRecord(contact, request))
	if err != nil {
		return err
	}
	details := append(*out.Details,
		crmcontracts.ApprovalPreviewDetail{Field: "consent_contact", Value: subject},
		previewDetail("consent_purpose", purpose),
		previewDetail("consent_policy_version", request.PolicyVersion),
		previewDetail("consent_wording", request.Wording),
		previewDetail("consent_result", decision.State),
	)
	effects := []string{"record_meeting_with_consent"}
	if !decision.Changed {
		effects = append(effects, "preserve_existing_consent")
	}
	if request.Marketing != nil {
		marketingDetails, effect, err := bookingMarketingPreview(ctx, store, contact, purposes, *request.Marketing)
		if err != nil {
			return err
		}
		details = append(details, marketingDetails...)
		effects = append(effects, effect)
	}
	out.Details, out.Effects = &details, &effects
	return nil
}

func bookingPurposeLabel(purposes []consent.Purpose, id ids.UUID) (string, error) {
	for _, purpose := range purposes {
		if purpose.ID.UUID == id && purpose.Label != "" {
			return purpose.Label, nil
		}
	}
	return "", apperrors.ErrPermissionDenied
}

func bookingMarketingPreview(ctx context.Context, store *consent.Store, contact ids.UUID, purposes []consent.Purpose, request activities.BookingMarketing) ([]crmcontracts.ApprovalPreviewDetail, string, error) {
	purpose, err := bookingPurposeLabel(purposes, request.PurposeID)
	if err != nil {
		return nil, "", err
	}
	details := []crmcontracts.ApprovalPreviewDetail{
		previewDetail("marketing_purpose", purpose),
		previewDetail("marketing_policy_version", request.PolicyVersion),
		previewDetail("marketing_wording", request.Wording),
	}
	address, err := store.PreviewConsentLink(ctx, ids.From[ids.ContactKind](contact), ids.From[ids.PurposeKind](request.PurposeID))
	if err != nil {
		if _, err := bookingMarketingFailure(err); err != nil {
			return nil, "", err
		}
		return details, "marketing_confirmation_unavailable", nil
	}
	details = append(details, previewDetail("confirmation_recipient", address))
	return details, "request_marketing_confirmation", nil
}
