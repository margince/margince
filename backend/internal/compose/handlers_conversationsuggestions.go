// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

type conversationSuggestionHandlers struct {
	read func(context.Context, datasource.EntityRef) (capture.ConversationSuggestions, error)
}

func (h conversationSuggestionHandlers) ListContactConversationSuggestions(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	h.listConversationSuggestions(w, r, datasource.EntityRef{Type: datasource.EntityContact, ID: ids.UUID(id)})
}

func (h conversationSuggestionHandlers) ListLeadConversationSuggestions(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	h.listConversationSuggestions(w, r, datasource.EntityRef{Type: datasource.EntityLead, ID: ids.UUID(id)})
}

func (h conversationSuggestionHandlers) listConversationSuggestions(w http.ResponseWriter, r *http.Request, anchor datasource.EntityRef) {
	suggestions, err := h.read(r.Context(), anchor)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out := crmcontracts.ConversationSuggestionListResponse{Data: []crmcontracts.ConversationSuggestion{}, ScanLimited: suggestions.ScanLimited}
	for _, suggestion := range suggestions.Data {
		out.Data = append(out.Data, crmcontracts.ConversationSuggestion{Address: suggestion.Address, DisplayName: suggestion.DisplayName, ActivityId: openapi_types.UUID(suggestion.Activity.UUID)})
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}
