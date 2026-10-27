// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ListLinkSuggestions answers the review queue of suggested links.
func (h Handlers) ListLinkSuggestions(w http.ResponseWriter, r *http.Request) {
	data, err := h.store.ListLinkSuggestions(r.Context())
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.LinkSuggestionList{Data: data})
}

// AcceptLinkSuggestion files the message under the suggested record.
func (h Handlers) AcceptLinkSuggestion(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	h.answerSuggestion(w, r, h.store.AcceptLinkSuggestion, id)
}

// RejectLinkSuggestion declines the suggestion and attaches nothing.
func (h Handlers) RejectLinkSuggestion(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	h.answerSuggestion(w, r, h.store.RejectLinkSuggestion, id)
}

func (h Handlers) answerSuggestion(
	w http.ResponseWriter, r *http.Request,
	decide func(ctx context.Context, id ids.UUID) (crmcontracts.LinkSuggestion, error), id crmcontracts.Id,
) {
	out, err := decide(r.Context(), ids.UUID(id))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}
