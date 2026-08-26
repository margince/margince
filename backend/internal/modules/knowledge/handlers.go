// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

// The transport surface: wire concerns only — decode, and map store errors onto
// the sentinel registry httperr.Write already resolves. The store owns the
// transactional write shape.

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/platform/blobstore"
	"github.com/gradionhq/margince/backend/internal/platform/database"
	"github.com/gradionhq/margince/backend/internal/platform/httperr"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// Handlers is the knowledge module's transport surface.
type Handlers struct {
	store *Store
	// uploadLimit is the ceiling compose granted this route. Zero means nobody
	// wired one, which the upload answers as OUR fault rather than carrying on
	// with a bound that refuses every file and blames the caller's.
	uploadLimit int64
	// queue enqueues a document's ingest inside the transaction that writes its
	// row. Nil on a composition with no job runner, and the upload says so
	// rather than accepting a file nothing will ever read.
	queue QueueIngest
}

// multipartSpillBytes is how much of an upload is held in memory before the
// parse spills to disk. Deliberately far below any route ceiling: it is the
// resident-memory threshold, not the bound, and the bound is the
// MaxBytesReader the upload installs.
const multipartSpillBytes = 1 << 20

// errUploadLimitUnset reports that this composition never told the handler what
// a document may weigh.
var errUploadLimitUnset = errors.New("knowledge: no upload ceiling configured for this role")

// errIngestUnavailable reports a composition with no job runner behind the
// upload. Accepting the file anyway would leave a document queued forever
// behind a worker that does not exist, which reads to the uploader as success.
var errIngestUnavailable = errors.New("knowledge: document ingest is not available on this role")

// WithUploadLimit grants this handler set the ceiling compose resolved for the
// upload route.
func (h Handlers) WithUploadLimit(bytes int64) Handlers {
	h.uploadLimit = bytes
	return h
}

// WithIngestQueue wires the enqueue the upload runs inside its write.
func (h Handlers) WithIngestQueue(queue QueueIngest) Handlers {
	h.queue = queue
	return h
}

// NewHandlers wires the transport over the workspace-bound app pool.
func NewHandlers(db *database.DB) Handlers { return Handlers{store: NewStore(db)} }

// WithBlobstore binds where uploaded document bytes live.
func (h Handlers) WithBlobstore(blob blobstore.Store) Handlers {
	h.store = h.store.WithBlobstore(blob)
	return h
}

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

// UploadCorpusDocument serves uploadCorpusDocument: multipart is parsed here
// (the JSON decoder cannot carry bytes); the store owns the RBAC gate, the type
// refusal, provenance and the write shape.
func (h Handlers) UploadCorpusDocument(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if h.uploadLimit <= 0 {
		httperr.Write(w, r, errUploadLimitUnset)
		return
	}
	if h.queue == nil {
		httperr.Write(w, r, errIngestUnavailable)
		return
	}
	// The same ceiling the chassis already applied, applied again: it is what
	// makes this handler correct when mounted without that middleware, and a
	// MaxBytesReader can only ever tighten a body an outer one already bounded.
	r.Body = http.MaxBytesReader(w, r.Body, h.uploadLimit)
	// upload:route /v1/knowledge/corpora/{id}/documents — the ceiling this parse runs
	// under is granted to that path in compose.uploadCeilings, and
	// TestEveryMultipartParseNamesItsRoute holds the two together.
	//nolint:gosec // G120 wants a bound here, and the bound is the MaxBytesReader above: this argument is only the in-memory/spill threshold, and it is deliberately far below the ceiling so the parse spills rather than holding the upload resident.
	if err := r.ParseMultipartForm(multipartSpillBytes); err != nil {
		httperr.WriteMultipartRefusal(w, r, err, h.uploadLimit)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		httperr.Write(w, r, httperr.Validation("file", "required", "a file part is required"))
		return
	}
	defer func(ctx context.Context) {
		if cerr := file.Close(); cerr != nil {
			slog.WarnContext(ctx, "closing the uploaded corpus document", "err", cerr)
		}
	}(r.Context())

	doc, err := h.store.UploadDocument(r.Context(), NewDocument{
		CorpusID:    ids.UUID(id),
		Filename:    header.Filename,
		ContentType: header.Header.Get("Content-Type"),
		Content:     file,
	}, h.queue)
	if err != nil {
		writeKnowledgeErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusAccepted, doc)
}

// ListCorpusDocuments serves listCorpusDocuments.
func (h Handlers) ListCorpusDocuments(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	items, err := h.store.ListDocuments(r.Context(), ids.UUID(id))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.KnowledgeDocumentList{Items: items})
}

// writeKnowledgeErr maps this module's typed refusals onto the wire shapes the
// contract names, then falls through to httperr.Write's sentinel registry —
// which already resolves ErrNotFound, ErrPermissionDenied and the rest.
func writeKnowledgeErr(w http.ResponseWriter, r *http.Request, err error) {
	var unsupported *UnsupportedTypeError
	if errors.As(err, &unsupported) {
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusUnsupportedMediaType,
			Code:   "unsupported_media_type",
			// The detail NAMES what is accepted. A bare rejection leaves the
			// uploader nothing to act on, and the alternative — taking the bytes
			// and ingesting nothing from them — reads to them exactly like
			// success.
			Detail: unsupported.Error(),
		})
		return
	}
	var filed *AlreadyFiledError
	if errors.As(err, &filed) {
		httperr.Write(w, r, httperr.Validation("file", "already_filed", filed.Error()))
		return
	}
	var full *CorpusFullError
	if errors.As(err, &full) {
		httperr.Write(w, r, httperr.Validation("file", "corpus_full", full.Error()))
		return
	}
	httperr.Write(w, r, err)
}

// DeleteCorpusDocument serves deleteCorpusDocument: 204, a hard delete that
// takes the passages, their vectors and the stored file with the row.
func (h Handlers) DeleteCorpusDocument(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if err := h.store.DeleteDocument(r.Context(), ids.UUID(id)); err != nil {
		writeKnowledgeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
