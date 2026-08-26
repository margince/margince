// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

// The transport surface: wire concerns only — decode, and map store errors onto
// the sentinel registry httperr.Write already resolves. The store owns the
// transactional write shape.

import (
	"net/http"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/platform/database"
	"github.com/gradionhq/margince/backend/internal/platform/httperr"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// Handlers is the knowledge module's transport surface.
type Handlers struct {
	store *Store
}

// NewHandlers wires the transport over the workspace-bound app pool.
func NewHandlers(db *database.DB) Handlers { return Handlers{store: NewStore(db)} }

// ListCorpora serves listCorpora.
func (h Handlers) ListCorpora(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.ListCorpora(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.KnowledgeCorpusList{Items: items})
}

// CreateCorpus serves createCorpus.
func (h Handlers) CreateCorpus(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.CreateCorpusJSONRequestBody
	if !httperr.Decode(w, r, &req) {
		return
	}
	corpus, err := h.store.CreateCorpus(r.Context(), NewCorpus{
		Name:           req.Name,
		Description:    req.Description,
		TopicStatement: req.TopicStatement,
		MinSimilarity:  req.MinSimilarity,
		DefaultAsk:     req.DefaultAsk != nil && *req.DefaultAsk,
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.Header().Set("Location", "/v1/knowledge/corpora/"+corpus.Id.String())
	httperr.WriteJSON(w, http.StatusCreated, corpus)
}

// ReadCorpus serves readCorpus.
func (h Handlers) ReadCorpus(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	corpus, err := h.store.ReadCorpus(r.Context(), ids.UUID(id))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, corpus)
}

// UpdateCorpus serves updateCorpus: a merge-PATCH.
func (h Handlers) UpdateCorpus(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.UpdateCorpusJSONRequestBody
	if !httperr.Decode(w, r, &req) {
		return
	}
	corpus, err := h.store.EditCorpus(r.Context(), ids.UUID(id), UpdateCorpus{
		Name:           req.Name,
		Description:    req.Description,
		TopicStatement: req.TopicStatement,
		MinSimilarity:  req.MinSimilarity,
		DefaultAsk:     req.DefaultAsk,
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, corpus)
}

// ArchiveCorpus serves archiveCorpus: 204, the corpus and everything filed in
// it archived together.
func (h Handlers) ArchiveCorpus(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if err := h.store.ArchiveCorpus(r.Context(), ids.UUID(id)); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
