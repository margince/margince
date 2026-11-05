// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func productCreationRequest(approval crmcontracts.Approval) (deals.CreateProductInput, error) {
	raw, tool, err := creationRequest[json.RawMessage](approval, "product")
	if err != nil {
		return deals.CreateProductInput{}, err
	}
	var source *string
	if tool {
		source = new(agents.ToolSource)
	}
	return deals.ProductCreationFromJSON(raw, source)
}

func (r approvalPreviewReader) productCreationPreview(ctx context.Context, approval crmcontracts.Approval, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	in, err := productCreationRequest(approval)
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
	plan, err := r.deals.PreviewProductCreation(ctx, in)
	if previewUnavailable(err) || errors.Is(err, apperrors.ErrConflict) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	details, err := r.productCreationDetails(ctx, plan)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	out.Details, out.Effects, out.State = &details, &[]string{"create_product_record"}, previewReadyState
	return out, r.currentEvidence(ctx, approval, out)
}

func (r approvalPreviewReader) productCreationDetails(ctx context.Context, plan deals.ProductCreationPreview) ([]crmcontracts.ApprovalPreviewDetail, error) {
	in := plan.Input
	fields := map[string]any{
		previewFieldName: in.Name, "sku": in.SKU, descriptionField: in.Description, previewFieldUnit: in.Unit,
		previewFieldUnitPriceMinor: in.UnitPriceMinor, dealTargetCurrency: in.Currency,
		"default_tax_rate": json.Number(plan.TaxRate), previewProductActive: in.Active,
		"billing_model": in.BillingModel, "billing_interval_months": in.BillingIntervalMonths, fieldSource: in.Source,
	}
	if in.BillingModel == nil {
		fields["billing_model"] = "not_specified"
	}
	return r.creationFieldDetails(ctx, "product", fields)
}
