// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func offerCreationRequest(approval crmcontracts.Approval) (ids.DealID, deals.CreateOfferInput, error) {
	raw, tool, err := creationRequest[json.RawMessage](approval, "offer")
	if err != nil {
		return ids.DealID{}, deals.CreateOfferInput{}, err
	}
	if tool {
		return deals.OfferCreationFromFields(raw, agents.ToolSource)
	}
	operation, opOK := (*approval.ProposedChange)["operation"].(string)
	path, pathOK := (*approval.ProposedChange)["path"].(string)
	parent, prefix := strings.CutPrefix(path, "/v1/deals/")
	parent, suffix := strings.CutSuffix(parent, "/offers")
	if !opOK || operation != operationCreateOffer || !pathOK || !prefix || !suffix {
		return ids.DealID{}, deals.CreateOfferInput{}, apperrors.ErrInvalidArgument
	}
	id, err := ids.Parse(parent)
	if err != nil || id.IsZero() {
		return ids.DealID{}, deals.CreateOfferInput{}, apperrors.ErrInvalidArgument
	}
	in, err := deals.OfferCreationInput(raw)
	return ids.From[ids.DealKind](id), in, err
}

func (r approvalPreviewReader) offerCreationPreview(ctx context.Context, approval crmcontracts.Approval, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	dealID, in, err := offerCreationRequest(approval)
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
	plan, err := r.deals.PreviewOfferCreation(ctx, dealID, in)
	if previewUnavailable(err) || errors.Is(err, apperrors.ErrConflict) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	details, err := r.offerCreationDetails(ctx, dealID, plan)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	out.Details, out.Effects, out.State = &details, &[]string{"create_offer_draft", "assign_offer_number"}, previewReadyState
	return out, r.currentEvidence(ctx, approval, out)
}

func (r approvalPreviewReader) offerCreationDetails(ctx context.Context, dealID ids.DealID, plan deals.OfferCreationPreview) ([]crmcontracts.ApprovalPreviewDetail, error) {
	in := plan.Input
	fields := map[string]any{
		dealIDField: dealID, previewFieldBuyerCompanyID: in.BuyerCompanyID, previewFieldTemplateID: in.TemplateID,
		dealTargetCurrency: in.Currency, "offer_status": previewOfferDraft, "valid_until": in.ValidUntil,
		"intro_text": in.IntroText, "terms_text": in.TermsText, fieldSource: in.Source,
		previewFieldNetMinor: plan.Totals.NetMinor, "tax_minor": plan.Totals.TaxMinor, previewFieldGrossMinor: plan.Totals.GrossMinor,
		"arr_minor": plan.Recurring.ArrMinor, "net_tcv_minor": plan.Recurring.NetTcvMinor,
	}
	if err := creationFieldsVisible(ctx, "offer", map[string]any{"line_items": in.LineItems}); err != nil {
		return nil, err
	}
	details, err := r.creationFieldDetails(ctx, "offer", fields)
	if err != nil {
		return nil, err
	}
	lines := slices.Clone(plan.Lines)
	slices.SortFunc(lines, func(a, b deals.OfferCreationLine) int { return *a.Input.Position - *b.Input.Position })
	for _, line := range lines {
		items, err := r.offerCreationLineDetails(ctx, in.Currency, line)
		if err != nil {
			return nil, err
		}
		details = append(details, items...)
	}
	return details, nil
}

func (r approvalPreviewReader) offerCreationLineDetails(ctx context.Context, currency string, line deals.OfferCreationLine) ([]crmcontracts.ApprovalPreviewDetail, error) {
	in := line.Input
	fields := map[string]any{
		"product_id": in.ProductID, descriptionField: in.Description, previewFieldUnit: in.Unit,
		"quantity": json.Number(in.Quantity), previewFieldUnitPriceMinor: in.UnitPriceMinor, dealTargetCurrency: currency,
		"discount_pct": json.Number(*in.DiscountPct), "tax_rate": json.Number(*in.TaxRate),
		"billing_model": in.BillingModel, "billing_interval_months": in.BillingIntervalMonths, "interval_count": in.IntervalCount,
		"line_net_minor": line.Figures.NetMinor, "line_tax_minor": line.Figures.TaxMinor, "line_total_minor": line.Figures.TotalMinor,
	}
	if in.BillingModel == nil {
		fields["billing_model"] = "not_specified"
	}
	items, err := r.creationFieldDetails(ctx, "offer", fields)
	if err != nil {
		return nil, err
	}
	position, err := json.Marshal(in.Position)
	if err != nil {
		return nil, err
	}
	value, err := previewScalar(position)
	if err != nil {
		return nil, err
	}
	return append([]crmcontracts.ApprovalPreviewDetail{{Field: "offer_line_position", Value: value}}, items...), nil
}
