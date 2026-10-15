// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"encoding/json"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// The offer's request rules, read by both doors. Money totals are derived and
// never accepted, a create names its source, and a line item maps one way. A
// rule written only in a handler is one the MCP door does not hold.

// totalsDerived is the refusal of a smuggled money total.
func totalsDerived(key string) error {
	return httperr.Validation(key, "totals_derived",
		"money totals are computed server-side from the line items and cannot be supplied")
}

// refuseClientTotals refuses an extension-carrying body that names a total.
func refuseClientTotals(extra map[string]any) error {
	for _, key := range clientTotalKeys {
		if _, found := extra[key]; found {
			return totalsDerived(key)
		}
	}
	return nil
}

func (c clientSuppliedTotals) refuse() error {
	for key, v := range map[string]*int64{
		"line_total": c.LineTotal, "line_total_minor": c.LineTotalMinor,
		"line_net": c.LineNet, "line_net_minor": c.LineNetMinor,
		"line_tax": c.LineTax, "line_tax_minor": c.LineTaxMinor,
		"net_minor": c.NetMinor, "tax_minor": c.TaxMinor,
		"gross_minor": c.GrossMinor, "total_minor": c.TotalMinor,
	} {
		if v != nil {
			return totalsDerived(key)
		}
	}
	return nil
}

// offerCreateInput maps a create body. It decodes twice. The generated shape's
// UnmarshalJSON owns the extension map. A probe then reads the nested lines,
// whose decode target has none and would drop a smuggled total.
func offerCreateInput(raw []byte) (CreateOfferInput, error) {
	var req crmcontracts.CreateOfferRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return CreateOfferInput{}, httperr.Validation("body", "malformed_json", err.Error())
	}
	if err := refuseClientTotals(req.AdditionalProperties); err != nil {
		return CreateOfferInput{}, err
	}
	var probe struct {
		LineItems []clientSuppliedTotals `json:"line_items"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return CreateOfferInput{}, httperr.Validation("body", "malformed_json", err.Error())
	}
	for _, li := range probe.LineItems {
		if err := li.refuse(); err != nil {
			return CreateOfferInput{}, err
		}
	}
	source, err := httperr.RequireNonBlank("source", req.Source)
	if err != nil {
		return CreateOfferInput{}, err
	}
	in := CreateOfferInput{
		Currency:       req.Currency,
		BuyerCompanyID: idArg[ids.CompanyKind](req.BuyerCompanyId),
		IntroText:      req.IntroText,
		TermsText:      req.TermsText,
		TemplateID:     idArg[ids.OfferTemplateKind](req.TemplateId),
		Source:         source,
	}
	if req.ValidUntil != nil {
		v := req.ValidUntil.Format("2006-01-02")
		in.ValidUntil = &v
	}
	if req.LineItems != nil {
		for _, li := range *req.LineItems {
			in.LineItems = append(in.LineItems, lineInputRow(li))
		}
	}
	return in, nil
}

// offerUpdateInput maps a patch; cleared names the fields sent as null.
func offerUpdateInput(req crmcontracts.UpdateOfferRequest, cleared []string, ifVersion *int64) (UpdateOfferInput, error) {
	if err := refuseClientTotals(req.AdditionalProperties); err != nil {
		return UpdateOfferInput{}, err
	}
	in := UpdateOfferInput{
		Currency:       req.Currency,
		BuyerCompanyID: idArg[ids.CompanyKind](req.BuyerCompanyId),
		IntroText:      req.IntroText,
		TermsText:      req.TermsText,
		TemplateID:     idArg[ids.OfferTemplateKind](req.TemplateId),
		IfVersion:      ifVersion,
		Clear:          cleared,
	}
	if req.ValidUntil != nil {
		v := req.ValidUntil.Format("2006-01-02")
		in.ValidUntil = &v
	}
	return in, nil
}

// addLineInput maps one line item to add.
func addLineInput(raw []byte) (OfferLineInputRow, error) {
	var req struct {
		crmcontracts.OfferLineItemInput
		clientSuppliedTotals
	}
	if err := datasource.StrictDecode(raw, &req); err != nil {
		return OfferLineInputRow{}, err
	}
	if err := req.refuse(); err != nil {
		return OfferLineInputRow{}, err
	}
	return lineInputRow(req.OfferLineItemInput), nil
}

// updateLineInput maps a line item patch. The billing classification is
// written whenever either half is named, so one cannot be left half-stated.
func updateLineInput(raw []byte) (UpdateOfferLineInput, error) {
	var req struct {
		crmcontracts.UpdateOfferLineItemRequest
		clientSuppliedTotals
	}
	if err := datasource.StrictDecode(raw, &req); err != nil {
		return UpdateOfferLineInput{}, err
	}
	if err := req.refuse(); err != nil {
		return UpdateOfferLineInput{}, err
	}
	in := UpdateOfferLineInput{
		Position:       req.Position,
		Description:    req.Description,
		Unit:           req.Unit,
		UnitPriceMinor: req.UnitPriceMinor,
	}
	if req.Quantity != nil {
		v := wireDecimal(*req.Quantity)
		in.Quantity = &v
	}
	if req.DiscountPct != nil {
		v := wireDecimal(*req.DiscountPct)
		in.DiscountPct = &v
	}
	if req.TaxRate != nil {
		v := wireDecimal(*req.TaxRate)
		in.TaxRate = &v
	}
	if req.BillingModel != nil || req.BillingIntervalMonths != nil {
		in.Classified = true
		in.BillingModel, in.BillingIntervalMonths = billingClassificationOf(req.BillingModel, req.BillingIntervalMonths)
	}
	if req.IntervalCount != nil {
		in.IntervalCountSet = true
		in.IntervalCount = req.IntervalCount
	}
	return in, nil
}
