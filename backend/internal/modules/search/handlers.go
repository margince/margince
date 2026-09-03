// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// Handlers is the module's transport slice; compose embeds it so the
// generated Search stub is shadowed by real code.
type Handlers struct {
	store     *Store
	retriever *Retriever
}

// NewHandlers builds the module's HTTP surface over a workspace-bound handle.
// NewHandlers binds the HTTP search surface. `tagReach` counts what a tag hit
// is on for the asking caller; compose supplies the collections store's own
// counter, and a nil one leaves every hit's count absent rather than zero.
// `emailRows` answers the canonical email row behind an activity hit; compose
// supplies the activities store's own reader, and a nil one leaves every email
// hit rendering the generic way.
func NewHandlers(db *database.DB, tagReach TagReachCounter, emailRows EmailSummaryReader) Handlers {
	store := NewStore(db).WithTagReach(tagReach).WithEmailSummaries(emailRows)
	// Embedder is nil, and stays nil: the only thing this retriever serves is
	// AssembleContext, which walks the context graph and never embeds. The
	// request-path embed lane compose binds is for the RANKED half, and
	// `GET /v1/search` does not use it — it calls the lexical lane directly,
	// which is what its contract describes.
	return Handlers{store: store, retriever: NewRetriever(store, nil)}
}

func (h Handlers) Search(w http.ResponseWriter, r *http.Request, params crmcontracts.SearchParams) {
	in := Input{Query: params.Q}
	if params.Types != nil {
		for _, t := range *params.Types {
			in.Types = append(in.Types, string(t))
		}
	}
	if params.Cursor != nil {
		in.Cursor = *params.Cursor
	}
	if params.Limit != nil {
		in.Limit = *params.Limit
	}

	page, err := h.store.Search(r.Context(), in)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}

	data := make([]crmcontracts.SearchResult, 0, len(page.Hits))
	for _, hit := range page.Hits {
		result := crmcontracts.SearchResult{
			Id:    openapi_types.UUID(hit.ID),
			Type:  crmcontracts.SearchResultType(hit.Type),
			Score: ptr(float32(hit.Score)),
		}
		if hit.Title != "" {
			result.Title = ptr(hit.Title)
		}
		if hit.Snippet != "" {
			result.Snippet = ptr(hit.Snippet)
		}
		// Copied, not aliased: `hit` is the loop's own variable and taking its
		// address would give every result the last hit's number.
		if hit.CarriedBy != nil {
			result.CarriedBy = ptr(*hit.CarriedBy)
		}
		// Same copy, same reason: the address of the loop's own variable would
		// give every result the last hit's row.
		if hit.EmailSummary != nil {
			result.EmailSummary = ptr(*hit.EmailSummary)
		}
		// native records are authoritative
		result.TrustTier = ptr(crmcontracts.SearchResultTrustTierAuthoritative)
		data = append(data, result)
	}
	pageInfo := crmcontracts.PageInfo{HasMore: page.HasMore}
	if page.NextCursor != "" {
		pageInfo.NextCursor = &page.NextCursor
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.SearchResponse{Data: data, Page: pageInfo})
}

func ptr[T any](v T) *T { return &v }
