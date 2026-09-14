// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The HTTP half of outcome reviews: decode, call the seam next door, and map
// its typed refusals onto the wire.
//
// Split from outcomereview.go the way the extraction accept's transport is
// split from its engine: that file owns the flow and the invariants, this one
// owns nothing but the wire.

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// outcomeReviewHandlers is the transport; the seam above owns the flow.
type outcomeReviewHandlers struct {
	reviews    *OutcomeReviews
	activities *activities.Store
}

func (h outcomeReviewHandlers) ListDealOutcomeReviews(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	items, err := h.reviews.List(r.Context(), ids.From[ids.DealKind](ids.UUID(id)))
	if err != nil {
		writeOutcomeReviewErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.OutcomeReviewListResponse{Data: items})
}

func (h outcomeReviewHandlers) CreateDealOutcomeReview(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.CreateOutcomeReviewRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	review, err := h.reviews.Write(r.Context(), ids.From[ids.DealKind](ids.UUID(id)), req)
	if err != nil {
		writeOutcomeReviewErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, review)
}

func (h outcomeReviewHandlers) ListActivityReviewTemplates(w http.ResponseWriter, r *http.Request) {
	items, err := h.activities.ListReviewTemplates(r.Context())
	if err != nil {
		writeOutcomeReviewErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK,
		crmcontracts.ActivityReviewTemplateListResponse{Data: items})
}

// writeOutcomeReviewErr hands every refusal to the sentinel registry.
//
// There is deliberately no mapping here. Every typed refusal this flow can
// raise already carries its own verdict — StaleClosingError unwraps to a
// conflict, DealNotClosedError and the answer-validation errors implement
// FieldFault — so a spelling in this file would repeat a rule the error type
// already carries, which is the drift the extraction transport's own comment
// warns about.
func writeOutcomeReviewErr(w http.ResponseWriter, r *http.Request, err error) {
	httperr.Write(w, r, err)
}
