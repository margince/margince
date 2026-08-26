// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/modules/knowledge"
	"github.com/gradionhq/margince/backend/internal/platform/blobstore"
	"github.com/gradionhq/margince/backend/internal/platform/httperr"
)

// knowledgeHandlers is the knowledge module's seat on Server: it forwards the
// operations the module answers today and 501s the ones whose code has not
// landed yet.
//
// The stubs are here because Server carries a compile-time assertion that it
// satisfies the whole generated ServerInterface, so a contract operation with
// no method breaks the build rather than serving a 404 nobody can explain. The
// contract is written first on purpose — the handler signatures, request and
// response types are generated from it — which leaves exactly this window where
// a route is declared and the code answering it is not yet written.
//
// The module's surface is a NAMED field rather than an embed, and the forwards
// below are written out. Embedding would promote its methods through Server
// itself, so `s.knowledgeHandlers.WithBlobstore(...)` and `s.WithBlobstore(...)`
// would both compile and mean the same thing — and which one the compiler picks
// changes as this seat gains and loses stubs.
//
// Each stub is deleted by the change that implements its operation, and a
// forward takes its place. When the last stub goes, so does this type.
type knowledgeHandlers struct {
	module knowledge.Handlers
	// ask is nil until WithCorpusAsk wires the engine.
	ask corpusAskFunc
}

// corpusAskFunc is the ask engine's entry point, held as a function so this
// seat carries no dependency it cannot construct.
type corpusAskFunc func(w http.ResponseWriter, r *http.Request, id crmcontracts.Id)

// The three wiring edges, as functions rather than methods. Server embeds this
// seat, so a uniquely-named METHOD here is promoted to Server itself and
// `s.withBlobstore(store)` compiles — reading as though the server carried the
// setting rather than the corpus surface did. A function takes the seat as an
// argument and cannot be reached that way.

func knowledgeWithUploadLimit(h knowledgeHandlers, bytes int64) knowledgeHandlers {
	h.module = h.module.WithUploadLimit(bytes)
	return h
}

func knowledgeWithBlobstore(h knowledgeHandlers, store blobstore.Store) knowledgeHandlers {
	h.module = h.module.WithBlobstore(store)
	return h
}

func knowledgeWithIngestQueue(h knowledgeHandlers, queue knowledge.QueueIngest) knowledgeHandlers {
	h.module = h.module.WithIngestQueue(queue)
	return h
}

// Answered by the module.

func (h knowledgeHandlers) ListCorpora(w http.ResponseWriter, r *http.Request) {
	h.module.ListCorpora(w, r)
}

func (h knowledgeHandlers) CreateCorpus(w http.ResponseWriter, r *http.Request) {
	h.module.CreateCorpus(w, r)
}

func (h knowledgeHandlers) ReadCorpus(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	h.module.ReadCorpus(w, r, id)
}

func (h knowledgeHandlers) UpdateCorpus(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	h.module.UpdateCorpus(w, r, id)
}

func (h knowledgeHandlers) ArchiveCorpus(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	h.module.ArchiveCorpus(w, r, id)
}

func (h knowledgeHandlers) UploadCorpusDocument(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	h.module.UploadCorpusDocument(w, r, id)
}

func (h knowledgeHandlers) ListCorpusDocuments(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	h.module.ListCorpusDocuments(w, r, id)
}

// Not yet written.

func (knowledgeHandlers) DeleteCorpusDocument(w http.ResponseWriter, r *http.Request, _ crmcontracts.Id) {
	httperr.NotImplemented(w, r, "deleteCorpusDocument")
}

// AskCorpus is answered by the compose-level engine rather than by the module:
// the ask joins the knowledge module's retrieval to the AI router's chat lane,
// and a module may join neither to the other. It keeps the 501 until
// WithCorpusAsk wires one, because an installation that composed no retrieval
// cannot search and pretending to would be worse than saying so.
func (h knowledgeHandlers) AskCorpus(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if h.ask == nil {
		httperr.NotImplemented(w, r, "askCorpus (no retrieval configured)")
		return
	}
	h.ask(w, r, id)
}

func knowledgeWithAsk(h knowledgeHandlers, ask corpusAskFunc) knowledgeHandlers {
	h.ask = ask
	return h
}
