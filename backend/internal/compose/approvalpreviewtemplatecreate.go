// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"slices"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func templateCreationRequest(approval crmcontracts.Approval) (deals.CreateOfferTemplateInput, error) {
	req, _, err := creationRequest[crmcontracts.CreateOfferTemplateRequest](approval, "offer_template")
	if err != nil {
		return deals.CreateOfferTemplateInput{}, err
	}
	return deals.OfferTemplateCreationInput(req)
}

func (r approvalPreviewReader) templateCreationPreview(ctx context.Context, approval crmcontracts.Approval, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	in, err := templateCreationRequest(approval)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if _, err := r.creationAuthority(ctx, approval); err != nil {
		if previewUnavailable(err) {
			return out, nil
		}
		return nil, err
	}
	plan, err := r.deals.PreviewTemplateCreation(ctx, in)
	if previewUnavailable(err) || errors.Is(err, apperrors.ErrConflict) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	details, err := r.templateCreationDetails(ctx, plan)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	effects := []string{"create_offer_template"}
	if plan.IsDefault {
		effects = append(effects, "set_template_locale_default")
	}
	out.Details, out.Effects, out.State = &details, &effects, previewReadyState
	return out, r.currentEvidence(ctx, approval, out)
}

func (r approvalPreviewReader) templateCreationDetails(ctx context.Context, in deals.CreateOfferTemplateInput) ([]crmcontracts.ApprovalPreviewDetail, error) {
	if err := creationFieldsVisible(ctx, "offer_template", map[string]any{"layout": in.Layout}); err != nil {
		return nil, err
	}
	fields := map[string]any{"name": in.Name, "locale": in.Locale, "is_default": in.IsDefault}
	keys := deals.OfferTemplateLayoutTextKeys()
	for key, value := range in.Layout {
		// Unknown layout content needs review through a surface that understands it.
		if !slices.Contains(keys, key) {
			return nil, apperrors.ErrInvalidArgument
		}
		text, ok := value.(string)
		if !ok {
			return nil, apperrors.ErrInvalidArgument
		}
		fields["template_"+key] = text
	}
	for _, key := range keys {
		field := "template_" + key
		if _, present := fields[field]; !present {
			fields[field] = ""
		}
	}
	return r.creationFieldDetails(ctx, "offer_template", fields)
}
