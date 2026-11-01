// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ListNoticeCaseReviews keeps source work separate from the published duty-only contract.
func (h Handlers) ListNoticeCaseReviews(w http.ResponseWriter, r *http.Request, params crmcontracts.ListNoticeCaseReviewsParams) {
	var states []NoticeState
	if params.State != nil {
		for _, state := range *params.State {
			states = append(states, NoticeState(state))
		}
	}
	limit, cursor := 0, ""
	if params.Limit != nil {
		limit = int(*params.Limit)
	}
	if params.Cursor != nil {
		cursor = *params.Cursor
	}
	cases, page, err := h.store.ListNoticeCaseReviews(r.Context(), states, limit, cursor)
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	h.writeNoticeCasePage(w, r, cases, page)
}

// GetNoticeCaseReview exposes the evidence without asserting a duty for an unknown source.
func (h Handlers) GetNoticeCaseReview(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	found, err := h.store.GetNoticeCaseReview(r.Context(), ids.UUID(id))
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	h.writeNoticeCase(w, r, found)
}

// AssignNoticeCaseReview lets a review be claimed without inventing a legal deadline.
func (h Handlers) AssignNoticeCaseReview(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.AssignNoticeCase
	if !httperr.Decode(w, r, &req) {
		return
	}
	assigned, err := h.store.AssignNoticeCaseReview(r.Context(), ids.UUID(id), ids.UUID(req.OwnerUserId))
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	h.writeNoticeCase(w, r, assigned)
}
