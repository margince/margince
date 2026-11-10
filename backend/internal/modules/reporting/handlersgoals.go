// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// planVersion reads If-Match, where 0 names a plan the caller saw as absent.
func planVersion(w http.ResponseWriter, r *http.Request) (int64, bool) {
	if r.Header.Get("If-Match") == "0" {
		return 0, true
	}
	return requiredVersion(w, r)
}

// ListSalesGoalSellers lists whose goals the caller may read for a year.
func (h Handlers) ListSalesGoalSellers(w http.ResponseWriter, r *http.Request, params crmcontracts.ListSalesGoalSellersParams) {
	out, err := h.service.ListGoalSellers(r.Context(), params.Year)
	respond(w, r, out, err)
}

// GetSalesGoalPlan reads one person's plan, refusing alike whether or not it exists.
func (h Handlers) GetSalesGoalPlan(w http.ResponseWriter, r *http.Request, ownerID openapi_types.UUID, year int) {
	out, err := h.service.GetGoalPlan(r.Context(), ids.UUID(ownerID), year)
	respond(w, r, out, err)
}

// SaveSalesGoalDraft stores the shared draft under the plan version the caller saw.
func (h Handlers) SaveSalesGoalDraft(w http.ResponseWriter, r *http.Request, ownerID openapi_types.UUID, year int, _ crmcontracts.SaveSalesGoalDraftParams) {
	var body crmcontracts.SalesGoalDraftInput
	if !httperr.Decode(w, r, &body) {
		return
	}
	version, ok := planVersion(w, r)
	if !ok {
		return
	}
	out, err := h.service.SaveGoalDraft(r.Context(), ids.UUID(ownerID), year, version, body)
	respond(w, r, out, err)
}

// DiscardSalesGoalDraft drops the draft and keeps the publication.
func (h Handlers) DiscardSalesGoalDraft(w http.ResponseWriter, r *http.Request, ownerID openapi_types.UUID, year int, _ crmcontracts.DiscardSalesGoalDraftParams) {
	version, ok := planVersion(w, r)
	if !ok {
		return
	}
	out, err := h.service.DiscardGoalDraft(r.Context(), ids.UUID(ownerID), year, version)
	respond(w, r, out, err)
}

// PublishSalesGoalPlan publishes the draft's twelve months in one transaction.
func (h Handlers) PublishSalesGoalPlan(w http.ResponseWriter, r *http.Request, ownerID openapi_types.UUID, year int, _ crmcontracts.PublishSalesGoalPlanParams) {
	var body crmcontracts.SalesGoalPublishInput
	if !httperr.Decode(w, r, &body) {
		return
	}
	version, ok := planVersion(w, r)
	if !ok {
		return
	}
	out, err := h.service.PublishGoalPlan(r.Context(), ids.UUID(ownerID), year, version, body)
	respond(w, r, out, err)
}

// WithdrawSalesGoalPlan retires a mistaken publication with its reason.
func (h Handlers) WithdrawSalesGoalPlan(w http.ResponseWriter, r *http.Request, ownerID openapi_types.UUID, year int, _ crmcontracts.WithdrawSalesGoalPlanParams) {
	var body crmcontracts.SalesGoalReasonInput
	if !httperr.Decode(w, r, &body) {
		return
	}
	version, ok := planVersion(w, r)
	if !ok {
		return
	}
	out, err := h.service.WithdrawGoalPlan(r.Context(), ids.UUID(ownerID), year, version, body.Reason)
	respond(w, r, out, err)
}
