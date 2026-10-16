// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"errors"
	"io"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WithSettings wires the configuration the legal review switch is read and
// saved through.
func (h Handlers) WithSettings(cfg *config.Config) Handlers {
	h.settings = cfg
	return h
}

// ListDealLegalReviews answers the deal's reviews, newest first.
func (h Handlers) ListDealLegalReviews(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	list, err := h.store.ListDealLegalReviews(r.Context(), pathID[ids.DealKind](id))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	if list == nil {
		list = []crmcontracts.LegalReviewSummary{}
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.DealLegalReviewListResponse{Data: list})
}

// RequestLegalReview asks legal to review the deal. The body is optional.
func (h Handlers) RequestLegalReview(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.RequestLegalReviewRequest
	if err := httperr.DecodeOrRefusal(w, r, &req); err != nil && !errors.Is(err, io.EOF) {
		httperr.Write(w, r, err)
		return
	}
	out, err := h.store.RequestLegalReview(r.Context(), pathID[ids.DealKind](id), RequestLegalReviewInput{
		Note: req.Note, PreviousReviewID: optionalUUID(req.PreviousReviewId),
	})
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, out)
}

// ListLegalReviews answers one page of the reviewer's queue.
func (h Handlers) ListLegalReviews(w http.ResponseWriter, r *http.Request, params crmcontracts.ListLegalReviewsParams) {
	filter := QueueOpen
	if params.Status != nil {
		filter = LegalReviewQueueFilter(*params.Status)
	}
	list, page, err := h.store.ListLegalReviewQueue(r.Context(), filter, params.Cursor, params.Limit)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	if list == nil {
		list = []crmcontracts.LegalReviewQueueItem{}
	}
	wire := crmcontracts.PageInfo{HasMore: page.HasMore}
	if page.NextCursor != "" {
		wire.NextCursor = &page.NextCursor
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.LegalReviewListResponse{Data: list, Page: wire})
}

// GetLegalReview answers one review with its pack.
func (h Handlers) GetLegalReview(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	out, err := h.store.GetLegalReview(r.Context(), ids.UUID(id))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// DecideLegalReview records legal's answer.
func (h Handlers) DecideLegalReview(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.DecideLegalReviewRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	out, err := h.store.DecideLegalReview(r.Context(), ids.UUID(id), DecideLegalReviewInput{
		Outcome: LegalReviewStatus(req.Outcome), Comments: req.Comments,
	})
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// WithdrawLegalReview takes back an open request.
func (h Handlers) WithdrawLegalReview(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	out, err := h.store.WithdrawLegalReview(r.Context(), ids.UUID(id))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// GetLegalReviewSettings answers whether legal review is on.
func (h Handlers) GetLegalReviewSettings(w http.ResponseWriter, r *http.Request) {
	if h.settings == nil {
		httperr.NotImplemented(w, r, "GetLegalReviewSettings")
		return
	}
	on, err := config.StoredVal(r.Context(), h.settings, LegalReviewEnabled)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.LegalReviewSettings{Enabled: on})
}

// UpdateLegalReviewSettings switches legal review on or off.
func (h Handlers) UpdateLegalReviewSettings(w http.ResponseWriter, r *http.Request) {
	if h.settings == nil {
		httperr.NotImplemented(w, r, "UpdateLegalReviewSettings")
		return
	}
	var req crmcontracts.LegalReviewSettings
	if !httperr.Decode(w, r, &req) {
		return
	}
	if err := SetLegalReviewEnabled(r.Context(), h.settings, req.Enabled); err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, req)
}
