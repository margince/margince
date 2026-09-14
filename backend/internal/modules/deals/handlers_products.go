// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (h Handlers) ListProducts(w http.ResponseWriter, r *http.Request, params crmcontracts.ListProductsParams) {
	in := ListProductsInput{
		Cursor:          params.Cursor,
		Limit:           params.Limit,
		Query:           params.Q,
		Active:          params.Active,
		IncludeArchived: params.IncludeArchived != nil && *params.IncludeArchived,
		Sort:            params.Sort,
	}
	products, page, err := h.store.ListProducts(r.Context(), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.ProductListResponse{Data: products, Page: pageInfo(page)})
}

func (h Handlers) CreateProduct(w http.ResponseWriter, r *http.Request, _ crmcontracts.CreateProductParams) {
	var req crmcontracts.CreateProductRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeStoreErr(w, r, &RequiredFieldError{Field: "name"})
		return
	}
	billingModel, billingMonths := billingClassificationOf(req.BillingModel, req.BillingIntervalMonths)
	in := CreateProductInput{
		Name:           req.Name,
		SKU:            req.Sku,
		Description:    req.Description,
		Unit:           req.Unit,
		UnitPriceMinor: req.UnitPriceMinor,
		Currency:       req.Currency,
		DefaultTaxRate: req.DefaultTaxRate,
		Active:         req.Active,
		Source:         req.Source,

		BillingModel:          billingModel,
		BillingIntervalMonths: billingMonths,
	}
	product, err := h.store.CreateProduct(r.Context(), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.Header().Set("Location", "/v1/products/"+product.Id.String())
	httperr.WriteJSON(w, http.StatusCreated, product)
}

func (h Handlers) GetProduct(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	product, err := h.store.GetProduct(r.Context(), pathID[ids.ProductKind](id), storekit.IncludeArchived)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, product)
}

func (h Handlers) UpdateProduct(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.UpdateProductParams) {
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	var req crmcontracts.UpdateProductRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	billingModel, billingMonths := billingClassificationOf(req.BillingModel, req.BillingIntervalMonths)
	// `not_specified` is how a caller says "nobody has classified this",
	// because a JSON null cannot: the generated field is a pointer, so a null
	// and an omitted field both arrive as nil and an omitted field has to keep
	// meaning "leave it alone". Translated to the pair of nulls the column
	// holds, here at the edge, so the store speaks in stored values only.
	if billingModel != nil && *billingModel == string(crmcontracts.UpdateProductRequestBillingModelNotSpecified) {
		billingModel, billingMonths = nil, nil
	}
	in := UpdateProductInput{
		Name:           req.Name,
		SKU:            req.Sku,
		Description:    req.Description,
		Unit:           req.Unit,
		UnitPriceMinor: req.UnitPriceMinor,
		Currency:       req.Currency,
		DefaultTaxRate: req.DefaultTaxRate,
		Active:         req.Active,
		IfVersion:      ifVersion,

		// A PATCH addresses the classification when it names either half.
		Classified:            req.BillingModel != nil || req.BillingIntervalMonths != nil,
		BillingModel:          billingModel,
		BillingIntervalMonths: billingMonths,
	}
	product, err := h.store.UpdateProduct(r.Context(), pathID[ids.ProductKind](id), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, product)
}

func (h Handlers) ArchiveProduct(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	product, err := h.store.ArchiveProduct(r.Context(), pathID[ids.ProductKind](id))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, product)
}

// billingClassificationOf turns the contract's generated enum types back into
// the plain pair the store validates.
//
// Generic over both because the generator gives EVERY schema its own named
// type for an enumerated field — CreateProductRequestBillingModel is a
// different type from UpdateProductRequestBillingModel and from
// ProductBillingModel, though all three hold the same four words. The store
// speaks in strings and ints so one shape rule serves products and offer lines
// alike, and this is the one place that conversion happens.
func billingClassificationOf[M ~string, I ~int](model *M, months *I) (*string, *int) {
	var outModel *string
	if model != nil {
		m := string(*model)
		outModel = &m
	}
	var outMonths *int
	if months != nil {
		v := int(*months)
		outMonths = &v
	}
	return outModel, outMonths
}
