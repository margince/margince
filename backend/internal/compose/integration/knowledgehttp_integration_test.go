// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// HTTP-level coverage for the /knowledge operations: the handler and its wire
// mapping, which the store suites never drive because they call the store
// directly.
//
// What only exists at this layer, and is therefore only testable here: the
// multipart parse, the STATUS CODES the contract publishes (202 for an upload
// that is accepted-for-ingest and not 201, 204 for both deletes, 415 for a file
// this product has no reader for), the Location header, and the refusals a
// composition answers when it was wired without an object store or without a
// job runner — both of which are legitimate deployment shapes rather than
// faults, and both of which must say so rather than 500.

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/knowledge"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// knowledgeUploadCeiling is what this suite grants the route. Well above every
// fixture here: the ceiling's own refusal is the chassis's to produce, and a
// suite that tripped it accidentally would be testing the wrong thing.
const knowledgeUploadCeiling = 5_000_000

func multipartDocument(t *testing.T, filename string, contentType string, data []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreatePart(textproto(filename, contentType))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, mw.FormDataContentType()
}

// textproto builds the file part's headers by hand because the CONTENT TYPE is
// the subject of two cases below, and multipart.CreateFormFile hard-codes
// application/octet-stream — which would make the type gate untestable through
// the real parse.
func textproto(filename, contentType string) map[string][]string {
	return map[string][]string{
		"Content-Disposition": {`form-data; name="file"; filename="` + filename + `"`},
		"Content-Type":        {contentType},
	}
}

// knowledgeHTTP is the handler set under test, with the seams a composition
// supplies: an object store, and an enqueue that records rather than queues.
type knowledgeHTTP struct {
	handlers knowledge.Handlers
	queued   []ids.UUID
}

func newKnowledgeHTTP(e *Env) *knowledgeHTTP {
	return newKnowledgeHTTPWithBlobs(e, blobstore.NewMemory())
}

// newKnowledgeHTTPWithBlobs is the same wiring against a CALLER'S object store,
// for the one suite that has to observe what the upload wrote and removed.
func newKnowledgeHTTPWithBlobs(e *Env, blobs blobstore.Store) *knowledgeHTTP {
	h := &knowledgeHTTP{}
	h.handlers = knowledge.NewHandlers(e.DB()).
		WithUploadLimit(knowledgeUploadCeiling).
		WithBlobstore(blobs).
		WithIngestQueue(func(_ context.Context, _ pgx.Tx, documentID ids.UUID) error {
			h.queued = append(h.queued, documentID)
			return nil
		})
	return h
}

func TestKnowledgeHandlersHTTPRoundTrip(t *testing.T) {
	e := Setup(t)
	h := newKnowledgeHTTP(e)
	ctx := e.As(e.Rep1, nil, corpusAdminPerms)

	// Create → 201 + Location + the stored entity.
	createRec := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/v1/knowledge/corpora",
		strings.NewReader(`{"name":"How-to","topic_statement":"How this product is operated."}`)).WithContext(ctx)
	h.handlers.CreateCorpus(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create: status %d, body %s", createRec.Code, createRec.Body.String())
	}
	var made crmcontracts.KnowledgeCorpus
	if err := json.Unmarshal(createRec.Body.Bytes(), &made); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if createRec.Header().Get("Location") == "" {
		t.Fatal("create answered no Location")
	}
	// The default floor is a SERVER decision and the wire reports it, so a
	// screen showing "0.35" is showing what the row holds rather than a
	// constant of its own.
	if made.MinSimilarity != knowledge.DefaultMinSimilarity {
		t.Fatalf("created floor = %v, want the default", made.MinSimilarity)
	}

	// List → 200 with the one set.
	listRec := httptest.NewRecorder()
	h.handlers.ListCorpora(listRec, httptest.NewRequest(http.MethodGet, "/v1/knowledge/corpora", nil).WithContext(ctx))
	if listRec.Code != http.StatusOK {
		t.Fatalf("list: status %d", listRec.Code)
	}
	var listed crmcontracts.KnowledgeCorpusList
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listed.Items) != 1 || listed.Items[0].Id != made.Id {
		t.Fatalf("list returned %d items", len(listed.Items))
	}

	// Read → 200.
	readRec := httptest.NewRecorder()
	h.handlers.ReadCorpus(readRec, httptest.NewRequest(http.MethodGet, "/v1/knowledge/corpora/x", nil).WithContext(ctx), made.Id)
	if readRec.Code != http.StatusOK {
		t.Fatalf("read: status %d", readRec.Code)
	}

	// Patch → 200 with the moved value.
	patchRec := httptest.NewRecorder()
	patchReq := httptest.NewRequest(http.MethodPatch, "/v1/knowledge/corpora/x",
		strings.NewReader(`{"min_similarity":0.62}`)).WithContext(ctx)
	h.handlers.UpdateCorpus(patchRec, patchReq, made.Id)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("patch: status %d, body %s", patchRec.Code, patchRec.Body.String())
	}
	var patched crmcontracts.KnowledgeCorpus
	if err := json.Unmarshal(patchRec.Body.Bytes(), &patched); err != nil {
		t.Fatalf("decode patch response: %v", err)
	}
	if patched.MinSimilarity != 0.62 {
		t.Fatalf("patched floor = %v", patched.MinSimilarity)
	}

	// Upload → 202, ACCEPTED FOR INGEST rather than 201 created: the document
	// exists but is not yet answerable, and the two statuses say different
	// things to a client deciding whether to poll.
	body, ctype := multipartDocument(t, "operating.md", "text/markdown", []byte(onePassage))
	upRec := httptest.NewRecorder()
	upReq := httptest.NewRequest(http.MethodPost, "/v1/knowledge/corpora/x/documents", body).WithContext(ctx)
	upReq.Header.Set("Content-Type", ctype)
	h.handlers.UploadCorpusDocument(upRec, upReq, made.Id)
	if upRec.Code != http.StatusAccepted {
		t.Fatalf("upload: status %d, body %s", upRec.Code, upRec.Body.String())
	}
	var doc crmcontracts.KnowledgeDocument
	if err := json.Unmarshal(upRec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	if doc.IngestStatus != "queued" {
		t.Fatalf("uploaded document status = %q, want queued", doc.IngestStatus)
	}
	if len(h.queued) != 1 {
		t.Fatalf("the upload queued %d ingests", len(h.queued))
	}

	// List documents → 200.
	docsRec := httptest.NewRecorder()
	h.handlers.ListCorpusDocuments(docsRec, httptest.NewRequest(http.MethodGet, "/v1/knowledge/corpora/x/documents", nil).WithContext(ctx), made.Id)
	if docsRec.Code != http.StatusOK {
		t.Fatalf("list documents: status %d", docsRec.Code)
	}
	var docs crmcontracts.KnowledgeDocumentList
	if err := json.Unmarshal(docsRec.Body.Bytes(), &docs); err != nil {
		t.Fatalf("decode documents response: %v", err)
	}
	if len(docs.Items) != 1 {
		t.Fatalf("listed %d documents", len(docs.Items))
	}

	// Delete the document → 204 and no body.
	delRec := httptest.NewRecorder()
	h.handlers.DeleteCorpusDocument(delRec, httptest.NewRequest(http.MethodDelete, "/v1/knowledge/documents/x", nil).WithContext(ctx), doc.Id)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("delete document: status %d, body %s", delRec.Code, delRec.Body.String())
	}
	if delRec.Body.Len() != 0 {
		t.Fatalf("a 204 carried a body: %s", delRec.Body.String())
	}

	// Archive the set → 204.
	archRec := httptest.NewRecorder()
	h.handlers.ArchiveCorpus(archRec, httptest.NewRequest(http.MethodDelete, "/v1/knowledge/corpora/x", nil).WithContext(ctx), made.Id)
	if archRec.Code != http.StatusNoContent {
		t.Fatalf("archive: status %d", archRec.Code)
	}
}

// A file this product has no reader for is 415 with the accepted types NAMED,
// not a 422 about a form field and not a 500. The refusal is about the FILE.
func TestUploadingAPDFOverHTTPIs415NamingTheAcceptedTypes(t *testing.T) {
	e := Setup(t)
	h := newKnowledgeHTTP(e)
	ctx := e.As(e.Rep1, nil, corpusAdminPerms)
	made := httpCorpus(ctx, t, h)

	body, ctype := multipartDocument(t, "handbook.pdf", "application/pdf", []byte("%PDF-1.7"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/knowledge/corpora/x/documents", body).WithContext(ctx)
	req.Header.Set("Content-Type", ctype)
	h.handlers.UploadCorpusDocument(rec, req, made.Id)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("PDF upload: status %d, want 415. body %s", rec.Code, rec.Body.String())
	}
	var problem struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem.Code != "unsupported_media_type" {
		t.Fatalf("problem code = %q", problem.Code)
	}
	// The detail names what IS accepted. A bare rejection leaves the uploader
	// nothing to act on, and the alternative — taking the bytes and ingesting
	// nothing from them — reads to them exactly like success.
	for _, accepted := range []string{"text/plain", "text/markdown", "text/csv", "application/json"} {
		if !strings.Contains(problem.Detail, accepted) {
			t.Fatalf("the 415 detail does not name %s: %q", accepted, problem.Detail)
		}
	}
	if len(h.queued) != 0 {
		t.Fatalf("a refused upload queued %d ingests", len(h.queued))
	}
}

// A request with no file part is a 422 about the FIELD, which is a different
// refusal from the 415 above and must not be collapsed into it.
func TestUploadingWithNoFilePartIs422(t *testing.T) {
	e := Setup(t)
	h := newKnowledgeHTTP(e)
	ctx := e.As(e.Rep1, nil, corpusAdminPerms)
	made := httpCorpus(ctx, t, h)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/knowledge/corpora/x/documents", &buf).WithContext(ctx)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	h.handlers.UploadCorpusDocument(rec, req, made.Id)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("upload with no file: status %d, want 422", rec.Code)
	}
}

// A composition wired without an object store answers as OUR fault rather than
// blaming the caller's file. Carrying on would write a row promising bytes that
// are nowhere.
func TestUploadingWithNoObjectStoreWiredDoesNotBlameTheCaller(t *testing.T) {
	e := Setup(t)
	h := knowledge.NewHandlers(e.DB()).
		WithUploadLimit(knowledgeUploadCeiling).
		WithIngestQueue(func(context.Context, pgx.Tx, ids.UUID) error { return nil })
	ctx := e.As(e.Rep1, nil, corpusAdminPerms)
	store := knowledge.NewStore(e.DB())
	made, err := store.CreateCorpus(ctx, howTo("How-to"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	body, ctype := multipartDocument(t, "operating.md", "text/markdown", []byte(onePassage))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/knowledge/corpora/x/documents", body).WithContext(ctx)
	req.Header.Set("Content-Type", ctype)
	h.UploadCorpusDocument(rec, req, made.Id)

	if rec.Code < 500 {
		t.Fatalf("upload with no object store: status %d, want a server-side refusal", rec.Code)
	}
}

// A composition with no job runner refuses rather than accepting a file nothing
// will ever read. An accepted upload that is never ingested reads to the
// uploader exactly like success.
func TestUploadingWithNoJobRunnerRefusesRatherThanAcceptingTheFile(t *testing.T) {
	e := Setup(t)
	h := knowledge.NewHandlers(e.DB()).
		WithUploadLimit(knowledgeUploadCeiling).
		WithBlobstore(blobstore.NewMemory())
	ctx := e.As(e.Rep1, nil, corpusAdminPerms)
	store := knowledge.NewStore(e.DB())
	made, err := store.CreateCorpus(ctx, howTo("How-to"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	body, ctype := multipartDocument(t, "operating.md", "text/markdown", []byte(onePassage))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/knowledge/corpora/x/documents", body).WithContext(ctx)
	req.Header.Set("Content-Type", ctype)
	h.UploadCorpusDocument(rec, req, made.Id)

	if rec.Code < 500 {
		t.Fatalf("upload with no job runner: status %d, want a server-side refusal", rec.Code)
	}
	if n := liveDocumentCount(t, e, ids.UUID(made.Id)); n != 0 {
		t.Fatalf("%d document row(s) written with nothing to ingest them", n)
	}
}

// An unknown set is 404 on every operation that names one, and the same 404 a
// foreign tenant's id would get: existence stays hidden.
func TestKnowledgeOperationsOnAnUnknownSetAre404(t *testing.T) {
	e := Setup(t)
	h := newKnowledgeHTTP(e)
	ctx := e.As(e.Rep1, nil, corpusAdminPerms)
	unknown := crmcontracts.Id(ids.NewV7())

	for name, call := range map[string]func(*httptest.ResponseRecorder){
		"read": func(rec *httptest.ResponseRecorder) {
			h.handlers.ReadCorpus(rec, httptest.NewRequest(http.MethodGet, "/x", nil).WithContext(ctx), unknown)
		},
		"listDocuments": func(rec *httptest.ResponseRecorder) {
			h.handlers.ListCorpusDocuments(rec, httptest.NewRequest(http.MethodGet, "/x", nil).WithContext(ctx), unknown)
		},
		"archive": func(rec *httptest.ResponseRecorder) {
			h.handlers.ArchiveCorpus(rec, httptest.NewRequest(http.MethodDelete, "/x", nil).WithContext(ctx), unknown)
		},
		"deleteDocument": func(rec *httptest.ResponseRecorder) {
			h.handlers.DeleteCorpusDocument(rec, httptest.NewRequest(http.MethodDelete, "/x", nil).WithContext(ctx), unknown)
		},
	} {
		rec := httptest.NewRecorder()
		call(rec)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s of an unknown set: status %d, want 404", name, rec.Code)
		}
	}
}

// A rep is refused with 403 on every write, and the refusal is an
// authorization one rather than a 404: the object gate denies, and hiding the
// OBJECT's existence would be a different claim from hiding a row's.
func TestARepIsRefused403OnEveryKnowledgeWrite(t *testing.T) {
	e := Setup(t)
	h := newKnowledgeHTTP(e)
	admin := e.As(e.Rep1, nil, corpusAdminPerms)
	made := httpCorpus(admin, t, h)
	rep := e.As(e.Rep1, nil, corpusRepPerms)

	createRec := httptest.NewRecorder()
	h.handlers.CreateCorpus(createRec, httptest.NewRequest(http.MethodPost, "/x",
		strings.NewReader(`{"name":"Rep's","topic_statement":"anything"}`)).WithContext(rep))
	if createRec.Code != http.StatusForbidden {
		t.Errorf("rep create: status %d, want 403", createRec.Code)
	}

	archRec := httptest.NewRecorder()
	h.handlers.ArchiveCorpus(archRec, httptest.NewRequest(http.MethodDelete, "/x", nil).WithContext(rep), made.Id)
	if archRec.Code != http.StatusForbidden {
		t.Errorf("rep archive: status %d, want 403", archRec.Code)
	}

	// And the reads a rep DOES hold still answer 200, or the ask would be an
	// admin tool.
	readRec := httptest.NewRecorder()
	h.handlers.ReadCorpus(readRec, httptest.NewRequest(http.MethodGet, "/x", nil).WithContext(rep), made.Id)
	if readRec.Code != http.StatusOK {
		t.Errorf("rep read: status %d, want 200", readRec.Code)
	}
}

func httpCorpus(ctx context.Context, t *testing.T, h *knowledgeHTTP) crmcontracts.KnowledgeCorpus {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/knowledge/corpora",
		strings.NewReader(`{"name":"How-to","topic_statement":"How this product is operated."}`)).WithContext(ctx)
	h.handlers.CreateCorpus(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status %d, body %s", rec.Code, rec.Body.String())
	}
	var made crmcontracts.KnowledgeCorpus
	if err := json.Unmarshal(rec.Body.Bytes(), &made); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	return made
}
