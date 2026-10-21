// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The offer transport surface (B-E03.20). One wire rule enforced HERE,
// before any store code runs: money totals are derived, never accepted —
// a body carrying any total spelling answers 422 totals_derived
// (B-E03.18), whatever else it says.

package deals

import (
	"io"
	"log/slog"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// clientTotalKeys are every accepted-nowhere spelling of a money total.
// The x-extension request shapes surface unknown keys in
// AdditionalProperties; the plain line-item shapes decode alongside
// clientSuppliedTotals below. Both paths check against this one list.
// maxOfferBody bounds the hand-read create body; far above any real
// offer, far below abuse.
const maxOfferBody = 1 << 20

var clientTotalKeys = []string{
	"line_total", "line_total_minor", "line_net", "line_net_minor",
	"line_tax", "line_tax_minor", "net_minor", "tax_minor", "gross_minor", "total_minor",
}

// clientSuppliedTotals rides embedded in the plain (non-extension)
// line-item decode targets so a smuggled total is SEEN instead of
// silently dropped by encoding/json.
type clientSuppliedTotals struct {
	LineTotal      *int64 `json:"line_total"`
	LineTotalMinor *int64 `json:"line_total_minor"`
	LineNet        *int64 `json:"line_net"`
	LineNetMinor   *int64 `json:"line_net_minor"`
	LineTax        *int64 `json:"line_tax"`
	LineTaxMinor   *int64 `json:"line_tax_minor"`
	NetMinor       *int64 `json:"net_minor"`
	TaxMinor       *int64 `json:"tax_minor"`
	GrossMinor     *int64 `json:"gross_minor"`
	TotalMinor     *int64 `json:"total_minor"`
}

func lineInputRow(in crmcontracts.OfferLineItemInput) OfferLineInputRow {
	row := OfferLineInputRow{
		Position:       in.Position,
		ProductID:      idArg[ids.ProductKind](in.ProductId),
		Description:    in.Description,
		Unit:           in.Unit,
		Quantity:       wireDecimal(in.Quantity),
		UnitPriceMinor: in.UnitPriceMinor,
	}
	if in.DiscountPct != nil {
		v := wireDecimal(*in.DiscountPct)
		row.DiscountPct = &v
	}
	if in.TaxRate != nil {
		v := wireDecimal(*in.TaxRate)
		row.TaxRate = &v
	}
	model, months := billingClassificationOf(in.BillingModel, in.BillingIntervalMonths)
	row.BillingModel, row.BillingIntervalMonths = model, months
	row.IntervalCount = in.IntervalCount
	return row
}

func (h Handlers) ListDealOffers(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.ListDealOffersParams) {
	in := ListDealOffersInput{Cursor: params.Cursor, Limit: params.Limit}
	if params.Status != nil {
		s := string(*params.Status)
		in.Status = &s
	}
	offers, page, err := h.store.ListDealOffers(r.Context(), pathID[ids.DealKind](id), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.OfferListResponse{Data: offers, Page: pageInfo(page)})
}

func (h Handlers) CreateOffer(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.CreateOfferParams) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxOfferBody))
	if err != nil {
		httperr.Write(w, r, httperr.Validation("body", "unreadable", "request body could not be read"))
		return
	}
	in, err := OfferCreationInput(raw)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}

	offer, err := h.store.CreateOffer(r.Context(), pathID[ids.DealKind](id), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.Header().Set("Location", "/v1/offers/"+offer.Id.String())
	httperr.WriteJSON(w, http.StatusCreated, offer)
}

func (h Handlers) GetOffer(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	offer, err := h.store.GetOffer(r.Context(), pathID[ids.OfferKind](id), storekit.IncludeArchived)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, offer)
}

func (h Handlers) UpdateOffer(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.UpdateOfferParams) {
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	var req crmcontracts.UpdateOfferRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	in, err := offerUpdateInput(req, httperr.ClearedFields(r), ifVersion)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}

	offerID := pathID[ids.OfferKind](id)
	offer, retired, err := h.store.UpdateOffer(r.Context(), offerID, in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	// The row is already committed without the retired rendering, so a failed
	// delete leaves an inert orphan, never a dangling reference — logged, as the
	// render's reclamation of a superseded PDF is, rather than failing the edit.
	if retired != nil && h.blob != nil {
		if delErr := h.blob.Delete(r.Context(), *retired); delErr != nil {
			slog.WarnContext(r.Context(), "reclaiming the rendering a buyer change retired",
				"offer", offerID.String(), "ref", *retired, "err", delErr)
		}
	}
	httperr.WriteJSON(w, http.StatusOK, offer)
}

// ArchiveOffer preserves the request version through the native archive writer.
func (h Handlers) ArchiveOffer(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.ArchiveOfferParams) {
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	offer, err := h.store.ArchiveOfferAt(r.Context(), pathID[ids.OfferKind](id), ifVersion)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, offer)
}

func (h Handlers) AddOfferLineItem(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxOfferBody))
	if err != nil {
		httperr.Write(w, r, httperr.Validation("body", "unreadable", "request body could not be read"))
		return
	}
	line, err := addLineInput(raw)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	offer, err := h.store.AddOfferLineItem(r.Context(), pathID[ids.OfferKind](id), line)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, offer)
}

func (h Handlers) UpdateOfferLineItem(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, lineItemId openapi_types.UUID) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxOfferBody))
	if err != nil {
		httperr.Write(w, r, httperr.Validation("body", "unreadable", "request body could not be read"))
		return
	}
	in, err := updateLineInput(raw)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	offer, err := h.store.UpdateOfferLineItem(r.Context(), pathID[ids.OfferKind](id), ids.UUID(lineItemId), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, offer)
}

func (h Handlers) RemoveOfferLineItem(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, lineItemId openapi_types.UUID) {
	offer, err := h.store.RemoveOfferLineItem(r.Context(), pathID[ids.OfferKind](id), ids.UUID(lineItemId))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, offer)
}

func (h Handlers) SendOffer(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.SendOfferParams) {
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	offer, err := h.store.SendOffer(r.Context(), pathID[ids.OfferKind](id), ifVersion)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, offer)
}

func (h Handlers) AcceptOffer(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.AcceptOfferParams) {
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	offer, err := h.store.AcceptOffer(r.Context(), pathID[ids.OfferKind](id), ifVersion)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, offer)
}

func (h Handlers) RejectOffer(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.RejectOfferParams) {
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	var req crmcontracts.RejectOfferRequest
	if r.ContentLength > 0 && !httperr.Decode(w, r, &req) {
		return
	}
	offer, err := h.store.RejectOffer(r.Context(), pathID[ids.OfferKind](id), req.Reason, ifVersion)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, offer)
}

func (h Handlers) RegenerateOffer(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.RegenerateOfferParams) {
	offer, err := h.store.RegenerateOffer(r.Context(), pathID[ids.OfferKind](id))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.Header().Set("Location", "/v1/offers/"+offer.Id.String())
	httperr.WriteJSON(w, http.StatusCreated, offer)
}
