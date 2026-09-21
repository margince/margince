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
	// THE CEILING RIDES THE HANDLE THE CALLER PASSES, and compose passes a
	// bounded one (server.go). It is not armed here, and that is deliberate:
	// the ceiling is a statement about who is WAITING, and the same constructor
	// would otherwise impose a request-path budget on any surface that reuses
	// it. Arming it at the wiring site puts it where every other seam of this
	// server is chosen, and lets a caller that answers nobody say so.
	//
	// Riding the HANDLE rather than one call site is what reaches both lanes
	// this surface opens — the lexical ranking, and the vector one through the
	// retriever. A ceiling armed at a single Query would leave the other lane as
	// unbounded as it was before anybody thought about it.
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

	pageInfo := crmcontracts.PageInfo{HasMore: page.HasMore}
	if page.NextCursor != "" {
		pageInfo.NextCursor = &page.NextCursor
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.SearchResponse{Data: wireHits(page.Hits), Page: pageInfo})
}

// wireHits renders a page of hits as the contract's results.
//
// Its own function rather than a loop in the handler, so the one claim the
// contract makes about every result — that it carries a trust tier, and that
// the tier is `authoritative` — is a property a test can assert without a
// database behind it.
func wireHits(hits []Hit) []crmcontracts.SearchResult {
	data := make([]crmcontracts.SearchResult, 0, len(hits))
	for _, hit := range hits {
		result := crmcontracts.SearchResult{
			Id:    openapi_types.UUID(hit.ID),
			Type:  crmcontracts.SearchResultType(hit.Type),
			Score: ptr(float32(hit.Score)),
			// Unconditionally, on every hit: this search reads the store the
			// record lives in, so a hit is never a copy of somebody else's
			// and there is no case here that could be anything but
			// authoritative. Leaving it nil would say UNKNOWN, which is a
			// different and weaker statement than the one we can make.
			TrustTier: ptr(crmcontracts.SearchResultTrustTierSearchResultTrustTierAuthoritative),
		}
		if hit.Title != "" {
			result.Title = ptr(hit.Title)
		}
		if hit.Snippet != "" {
			result.Snippet = ptr(hit.Snippet)
		}
		// Copied by VALUE, so the response owns what it carries and shares no
		// pointee with the page it was rendered from.
		if hit.CarriedBy != nil {
			result.CarriedBy = ptr(*hit.CarriedBy)
		}
		if hit.EmailSummary != nil {
			result.EmailSummary = ptr(*hit.EmailSummary)
		}
		data = append(data, result)
	}
	return data
}

func ptr[T any](v T) *T { return &v }
