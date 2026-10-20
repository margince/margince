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
	price, _ := httperr.PresentField(r, "unit_price_minor")
	in, err := productCreateInput(req, price != nil)
	if err != nil {
		httperr.Write(w, r, err)
		return
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
	in, err := productUpdateInput(req, httperr.ClearedFields(r), ifVersion)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	product, err := h.store.UpdateProduct(r.Context(), pathID[ids.ProductKind](id), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, product)
}

func (h Handlers) ArchiveProduct(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.ArchiveProductParams) {
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	product, err := h.store.ArchiveProductAt(r.Context(), pathID[ids.ProductKind](id), ifVersion)
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
