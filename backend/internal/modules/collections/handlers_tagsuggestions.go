// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"context"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// GetTagSuggestion serves one open suggestion with its evidence.
func (h Handlers) GetTagSuggestion(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	h.serveTagSuggestion(w, r, id, h.store.GetTagSuggestion)
}

// AcceptTagSuggestion applies the suggested tag as the caller.
func (h Handlers) AcceptTagSuggestion(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	h.serveTagSuggestion(w, r, id, h.store.AcceptTagSuggestion)
}

// DismissTagSuggestion records, for the workspace, that the evidence does not earn the tag.
func (h Handlers) DismissTagSuggestion(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	h.serveTagSuggestion(w, r, id, h.store.DismissTagSuggestion)
}

func (h Handlers) serveTagSuggestion(w http.ResponseWriter, r *http.Request, id crmcontracts.Id,
	act func(context.Context, ids.UUID) (TagSuggestion, error),
) {
	suggestion, err := act(r.Context(), ids.UUID(id))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, WireTagSuggestion(suggestion))
}

// WireTagSuggestion renders a suggestion. Evidence is always an array.
func WireTagSuggestion(s TagSuggestion) crmcontracts.TagSuggestion {
	tag := crmcontracts.RowTag{TagId: openapi_types.UUID(s.TagID.UUID), Name: s.TagName}
	if s.TagColor != nil {
		color := crmcontracts.RowTagColor(*s.TagColor)
		tag.Color = &color
	}
	out := crmcontracts.TagSuggestion{
		Id:         openapi_types.UUID(s.ID),
		State:      crmcontracts.TagSuggestionState(s.State),
		Tag:        tag,
		EntityType: crmcontracts.TagSuggestionEntityType(s.EntityType),
		EntityId:   openapi_types.UUID(s.EntityID),
		EntityName: s.EntityName,
		CreatedAt:  s.CreatedAt,
		Evidence:   make([]crmcontracts.TagSuggestionEvidence, 0, len(s.Evidence)),
	}
	for _, e := range s.Evidence {
		out.Evidence = append(out.Evidence, crmcontracts.TagSuggestionEvidence{
			ActivityId: openapi_types.UUID(e.ActivityID),
			Kind:       e.Kind,
			Subject:    e.Subject,
			OccurredAt: e.OccurredAt,
		})
	}
	return out
}
