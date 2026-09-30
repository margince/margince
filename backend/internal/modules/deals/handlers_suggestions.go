// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"errors"
	"io"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ListDealSuggestions answers the open suggestions the caller may see.
func (h Handlers) ListDealSuggestions(w http.ResponseWriter, r *http.Request, params crmcontracts.ListDealSuggestionsParams) {
	in := SuggestionQuery{
		CompanyID: optionalUUID(params.CompanyId), PipelineID: optionalUUID(params.PipelineId),
		StageID: optionalUUID(params.StageId),
	}
	if params.Cursor != nil {
		in.Cursor = *params.Cursor
	}
	if params.Limit != nil {
		in.Limit = *params.Limit
	}
	list, next, err := h.store.ListSuggestions(r.Context(), in)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	page := crmcontracts.PageInfo{HasMore: next != ""}
	if next != "" {
		page.NextCursor = &next
	}
	data := make([]crmcontracts.DealSuggestion, 0, len(list))
	for _, s := range list {
		data = append(data, SuggestionWire(s))
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.DealSuggestionListResponse{Data: data, Page: page})
}

// AcceptDealSuggestion opens the suggested deal. The body is optional: without
// one the suggestion's own values stand.
func (h Handlers) AcceptDealSuggestion(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.AcceptDealSuggestionParams) {
	var req crmcontracts.AcceptDealSuggestionRequest
	if err := httperr.DecodeOrRefusal(w, r, &req); err != nil && !errors.Is(err, io.EOF) {
		httperr.Write(w, r, err)
		return
	}
	in := AcceptSuggestionInput{
		Name: req.Name, AmountMinor: req.AmountMinor, Currency: req.Currency,
		NoAmount: req.NoAmount != nil && *req.NoAmount,
		StageID:  optionalUUID(req.StageId), OwnerID: optionalUUID(req.OwnerId),
	}
	if req.CloseDate != nil {
		in.CloseDate = &req.CloseDate.Time
	}
	out, err := h.store.AcceptSuggestion(r.Context(), ids.UUID(id), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	unlinked := make([]openapi_types.UUID, 0, len(out.Unlinked))
	for _, activity := range out.Unlinked {
		unlinked = append(unlinked, openapi_types.UUID(activity))
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.DealSuggestionAcceptance{
		Suggestion: SuggestionWire(out.Suggestion), DealId: openapi_types.UUID(out.DealID),
		UnlinkedActivityIds: unlinked, AcknowledgedSignals: out.Acknowledged,
	})
}

// DismissDealSuggestion records, for the workspace, that the suggestion is not a deal.
func (h Handlers) DismissDealSuggestion(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.DismissDealSuggestionParams) {
	out, err := h.store.DismissSuggestion(r.Context(), ids.UUID(id))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, SuggestionWire(out))
}

func optionalUUID(id *openapi_types.UUID) *ids.UUID {
	if id == nil {
		return nil
	}
	converted := ids.UUID(*id)
	return &converted
}

// SuggestionWire is a suggestion as the contract spells it, for every surface
// that answers one.
func SuggestionWire(s Suggestion) crmcontracts.DealSuggestion {
	out := crmcontracts.DealSuggestion{
		Id: openapi_types.UUID(s.ID), Kind: crmcontracts.DealSuggestionKind(s.Kind),
		State: crmcontracts.DealSuggestionState(s.State), CompanyId: openapi_types.UUID(s.CompanyID),
		CompanyName: s.CompanyName, PipelineId: openapi_types.UUID(s.PipelineID),
		StageId: openapi_types.UUID(s.StageID), NameHint: crmcontracts.DealSuggestionNameHint(s.NameHint), AmountMinor: s.AmountMinor,
		Currency: s.Currency, Confidence: float32(s.Confidence), CreatedAt: s.CreatedAt,
		Evidence: make([]crmcontracts.DealSuggestionEvidence, 0, len(s.Evidence)),
	}
	if s.CloseDate != nil {
		out.CloseDate = &openapi_types.Date{Time: *s.CloseDate}
	}
	for _, e := range s.Evidence {
		out.Evidence = append(out.Evidence, crmcontracts.DealSuggestionEvidence{
			Kind: crmcontracts.DealSuggestionEvidenceKind(e.Kind), OccurredAt: e.OccurredAt, Title: e.Title,
			ActivityId: uuidPtr(e.ActivityID), SignalId: uuidPtr(e.SignalID), AttachmentId: uuidPtr(e.AttachmentID),
		})
	}
	return out
}
