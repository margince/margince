// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"encoding/json"
	"slices"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// The catalog's request rules, read once by both doors: the REST handlers and
// the record seam a passport reaches over MCP. A rule written only in a handler
// is one the other door does not hold.

// productCreateInput maps a create body. priceStated reports whether the body
// named unit_price_minor with a value: a missing or null price decodes to 0,
// which would sell the product free.
func productCreateInput(req crmcontracts.CreateProductRequest, priceStated bool) (CreateProductInput, error) {
	source, err := httperr.RequireNonBlank("source", req.Source)
	if err != nil {
		return CreateProductInput{}, err
	}
	if !priceStated {
		return CreateProductInput{}, httperr.Validation("unit_price_minor", "required",
			"unit_price_minor is required: send the price in minor units, or 0 for a free item")
	}
	billingModel, billingMonths := billingClassificationOf(req.BillingModel, req.BillingIntervalMonths)
	return CreateProductInput{
		Name:           req.Name,
		SKU:            req.Sku,
		Description:    req.Description,
		Unit:           req.Unit,
		UnitPriceMinor: req.UnitPriceMinor,
		Currency:       req.Currency,
		DefaultTaxRate: req.DefaultTaxRate,
		Active:         req.Active,
		Source:         source,

		BillingModel:          billingModel,
		BillingIntervalMonths: billingMonths,
	}, nil
}

// productUpdateInput maps a patch. cleared names the fields the body sent as
// an explicit null.
func productUpdateInput(req crmcontracts.UpdateProductRequest, cleared []string, ifVersion *int64) (UpdateProductInput, error) {
	if slices.Contains(cleared, "name") {
		return UpdateProductInput{}, httperr.Validation("name", "required", "name is required")
	}
	billingModel, billingMonths := billingClassificationOf(req.BillingModel, req.BillingIntervalMonths)
	// `not_specified` is how a caller says "nobody has classified this".
	// product_billing_shape ties billing_model to billing_interval_months, so
	// one sentinel moves the pair. It becomes the two nulls the columns hold
	// here, and the store speaks in stored values only.
	if billingModel != nil && *billingModel == string(crmcontracts.UpdateProductRequestBillingModelNotSpecified) {
		billingModel, billingMonths = nil, nil
	}
	// Beside a named model, a null interval is the pair's other half. The
	// classification writes it, so it is no clear of one column.
	clears := slices.Clone(cleared)
	if req.BillingModel != nil {
		clears = slices.DeleteFunc(clears, func(field string) bool { return field == billingIntervalColumn })
	}
	return UpdateProductInput{
		Name:           req.Name,
		SKU:            req.Sku,
		Description:    req.Description,
		Unit:           req.Unit,
		UnitPriceMinor: req.UnitPriceMinor,
		Currency:       req.Currency,
		DefaultTaxRate: req.DefaultTaxRate,
		Active:         req.Active,
		Clear:          clears,
		IfVersion:      ifVersion,

		// A patch addresses the classification when it names either half.
		Classified:            req.BillingModel != nil || req.BillingIntervalMonths != nil,
		BillingModel:          billingModel,
		BillingIntervalMonths: billingMonths,
	}, nil
}

// OfferTemplateCreationInput keeps layout presence checks shared by creation and review.
func OfferTemplateCreationInput(req crmcontracts.CreateOfferTemplateRequest) (CreateOfferTemplateInput, error) {
	name, err := httperr.RequireNonBlank(offerTemplateNameField, req.Name)
	if err != nil {
		return CreateOfferTemplateInput{}, err
	}
	if req.Layout == nil {
		return CreateOfferTemplateInput{}, &RequiredFieldError{Field: "layout"}
	}
	in := CreateOfferTemplateInput{Name: name, Layout: req.Layout}
	if req.Locale != nil {
		in.Locale = *req.Locale
	}
	if req.IsDefault != nil {
		in.IsDefault = *req.IsDefault
	}
	return in, nil
}

// offerTemplateUpdateInput maps the full-replace body: every writable field is
// required, matching the store's full-replace semantics.
func offerTemplateUpdateInput(req crmcontracts.UpdateOfferTemplateRequest, ifVersion *int64) (UpdateOfferTemplateInput, error) {
	name, err := httperr.RequireNonBlank(offerTemplateNameField, req.Name)
	if err != nil {
		return UpdateOfferTemplateInput{}, err
	}
	if req.Layout == nil {
		return UpdateOfferTemplateInput{}, &RequiredFieldError{Field: "layout"}
	}
	return UpdateOfferTemplateInput{
		Name: name, Locale: req.Locale, IsDefault: req.IsDefault, Layout: req.Layout, IfVersion: ifVersion,
	}, nil
}

// ProductCreationFromJSON preserves price presence so omission cannot become a free product.
func ProductCreationFromJSON(raw json.RawMessage, sourceOverride *string) (CreateProductInput, error) {
	var req crmcontracts.CreateProductRequest
	if err := datasource.StrictDecode(raw, &req); err != nil {
		return CreateProductInput{}, err
	}
	if sourceOverride != nil {
		req.Source = *sourceOverride
	}
	return productCreateInput(req, statesAValue(raw, "unit_price_minor"))
}
