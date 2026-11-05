// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"io"
	"net/http"
	"slices"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func (h Handlers) GetDealLines(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	out, err := h.store.GetDealLines(r.Context(), pathID[ids.DealKind](id))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func (h Handlers) AddDealLine(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxOfferBody))
	if err != nil {
		httperr.Write(w, r, httperr.Validation("body", "unreadable", "request body could not be read"))
		return
	}
	in, err := addDealLineInput(raw)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out, err := h.store.AddDealLine(r.Context(), pathID[ids.DealKind](id), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func (h Handlers) UpdateDealLine(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, lineID openapi_types.UUID) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxOfferBody))
	if err != nil {
		httperr.Write(w, r, httperr.Validation("body", "unreadable", "request body could not be read"))
		return
	}
	in, err := updateDealLineInput(raw)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out, err := h.store.UpdateDealLine(r.Context(), pathID[ids.DealKind](id), ids.UUID(lineID), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func (h Handlers) RemoveDealLine(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, lineID openapi_types.UUID, params crmcontracts.RemoveDealLineParams) {
	out, err := h.store.RemoveDealLine(r.Context(), pathID[ids.DealKind](id), ids.UUID(lineID), params.ExpectedRevision)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func (h Handlers) UseDealEstimate(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.DealRevisionRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	out, err := h.store.UseDealEstimate(r.Context(), pathID[ids.DealKind](id), req.ExpectedRevision)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func (h Handlers) RefreshOfferFromDeal(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.RefreshOfferFromDealParams) {
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	var req crmcontracts.DealRevisionRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	offer, err := h.store.RefreshOfferFromDeal(r.Context(), pathID[ids.OfferKind](id), req.ExpectedRevision, ifVersion)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, offer)
}

// addDealLineInput maps a line to add. Decimals become exact strings once,
// here, as an offer line's do.
func addDealLineInput(raw []byte) (AddDealLineInput, error) {
	var req struct {
		crmcontracts.AddDealLineRequest
		clientSuppliedTotals
	}
	if err := datasource.StrictDecode(raw, &req); err != nil {
		return AddDealLineInput{}, err
	}
	if err := req.refuse(); err != nil {
		return AddDealLineInput{}, err
	}
	in := AddDealLineInput{
		ExpectedRevision: req.ExpectedRevision,
		ReplaceEstimate:  req.ReplaceEstimate != nil && *req.ReplaceEstimate,
		Currency:         req.Currency,
		ProductID:        idArg[ids.ProductKind](req.ProductId),
		Description:      req.Description,
		Unit:             req.Unit,
		Quantity:         wireDecimalPtr(req.Quantity),
		UnitPriceMinor:   req.UnitPriceMinor,
		DiscountPct:      wireDecimalPtr(req.DiscountPct),
		TaxRate:          wireDecimalPtr(req.TaxRate),
		TaxCategory:      componentKindOf(req.TaxCategory),
		IntervalCount:    req.IntervalCount,
	}
	in.BillingModel, in.IntervalMonths = billingClassificationOf(req.BillingModel, req.BillingIntervalMonths)
	return in, nil
}

// updateDealLineInput maps a line edit. A null names a field as much as a
// value does: a quantity or price sent as null becomes unknown again.
func updateDealLineInput(raw []byte) (UpdateDealLineInput, error) {
	var req struct {
		crmcontracts.UpdateDealLineRequest
		clientSuppliedTotals
	}
	if err := datasource.StrictDecode(raw, &req); err != nil {
		return UpdateDealLineInput{}, err
	}
	if err := req.refuse(); err != nil {
		return UpdateDealLineInput{}, err
	}
	nulls, err := httperr.NullKeysOf(raw)
	if err != nil {
		return UpdateDealLineInput{}, err
	}
	named := func(field string, sent bool) bool { return sent || slices.Contains(nulls, field) }
	in := UpdateDealLineInput{
		ExpectedRevision: req.ExpectedRevision,
		Description:      req.Description,
		Unit:             req.Unit,
		QuantitySet:      named("quantity", req.Quantity != nil),
		Quantity:         wireDecimalPtr(req.Quantity),
		PriceSet:         named("unit_price_minor", req.UnitPriceMinor != nil),
		UnitPriceMinor:   req.UnitPriceMinor,
		DiscountPct:      wireDecimalPtr(req.DiscountPct),
		TaxRate:          wireDecimalPtr(req.TaxRate),
		TaxCategory:      componentKindOf(req.TaxCategory),
		Classified: named("billing_model", req.BillingModel != nil) ||
			named("billing_interval_months", req.BillingIntervalMonths != nil),
		IntervalCountSet: named("interval_count", req.IntervalCount != nil),
		IntervalCount:    req.IntervalCount,
	}
	in.BillingModel, in.IntervalMonths = billingClassificationOf(req.BillingModel, req.BillingIntervalMonths)
	return in, nil
}

func wireDecimalPtr(v *float64) *string {
	if v == nil {
		return nil
	}
	s := wireDecimal(*v)
	return &s
}
