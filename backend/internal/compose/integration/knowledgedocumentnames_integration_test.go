// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What a document row tells a reader: its name and whether Margince ships it.
// It also says how much of it is searchable, on the corpus coverage's terms.

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/knowledge"
	"github.com/margince/margince/backend/internal/modules/knowledge/handbook"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func documentsByFilename(t *testing.T, ie *ingestEnv, store *knowledge.Store, corpus ids.UUID) map[string]crmcontracts.KnowledgeDocument {
	t.Helper()
	docs, err := store.ListDocuments(ie.ctx, corpus)
	if err != nil {
		t.Fatalf("list documents: %v", err)
	}
	out := make(map[string]crmcontracts.KnowledgeDocument, len(docs))
	for _, doc := range docs {
		out[doc.Filename] = doc
	}
	return out
}

func TestOnlyAShippedPageIsNamedByItsHeading(t *testing.T) {
	ie := newIngestEnv(t)
	uploaded := ie.upload(t, "README.md", "text/markdown", prose(1))
	mine := documentsByFilename(t, ie, ie.store, ie.corpus)["README.md"]
	if mine.Id != openapi_types.UUID(uploaded) || mine.Title != nil || mine.Shipped {
		t.Fatalf("an upload named like a handbook page came back title=%v shipped=%v, want no title and not shipped", mine.Title, mine.Shipped)
	}

	bootCtx := handbookBootCtx(ie.env.WS)
	var q recordingQueue
	if _, err := ie.store.ReconcileHandbook(bootCtx, noSerialize, q.queue()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	shippedCorpus := handbookCorpusID(bootCtx, t, ie.env)
	ie.env.WsExec(t, `INSERT INTO knowledge_document
	    (corpus_id, filename, content_type, byte_size, storage_key, checksum, managed_source, captured_by)
	  VALUES ($1, 'withdrawn.md', 'text/markdown', 12, 'handbook:withdrawn.md', 'old-release', $2, 'system:test')`,
		shippedCorpus, knowledge.HandbookSource)

	shipped := documentsByFilename(t, ie, ie.store, shippedCorpus)
	pages, err := handbook.Pages()
	if err != nil {
		t.Fatalf("read the embedded handbook: %v", err)
	}
	for _, page := range pages {
		doc := shipped[page.Filename]
		if !doc.Shipped || doc.Title == nil || *doc.Title != handbook.Title(page.Filename) {
			t.Errorf("%s: shipped=%v title=%v, want shipped under its heading %q", page.Filename, doc.Shipped, doc.Title, handbook.Title(page.Filename))
		}
	}
	if withdrawn := shipped["withdrawn.md"]; !withdrawn.Shipped || withdrawn.Title != nil {
		t.Errorf("a shipped row whose page this build lacks: shipped=%v title=%v, want shipped and no title", withdrawn.Shipped, withdrawn.Title)
	}
}

// assertDocumentsSumToTheirCorpus holds the row and the set to one rule. The
// documents' vectored passages add up to the corpus figure. The corpus is
// complete when every document is, and only then.
func assertDocumentsSumToTheirCorpus(t *testing.T, ee *embedEnv, store *knowledge.Store, step string) map[ids.UUID]crmcontracts.KnowledgeDocument {
	t.Helper()
	corpus, err := store.ReadCorpus(ee.ctx, ee.corpus)
	if err != nil {
		t.Fatalf("%s: read corpus: %v", step, err)
	}
	docs, err := store.ListDocuments(ee.ctx, ee.corpus)
	if err != nil {
		t.Fatalf("%s: list documents: %v", step, err)
	}
	byID := make(map[ids.UUID]crmcontracts.KnowledgeDocument, len(docs))
	sum, allComplete := 0, true
	for _, doc := range docs {
		byID[ids.UUID(doc.Id)] = doc
		sum += doc.ChunksEmbedded
		allComplete = allComplete && doc.EmbedState == crmcontracts.KnowledgeEmbedStateComplete
	}
	if sum != corpus.Coverage.ChunksEmbedded {
		t.Errorf("%s: documents carry %d vectored passages, the corpus reports %d", step, sum, corpus.Coverage.ChunksEmbedded)
	}
	if corpusComplete := corpus.Coverage.EmbedState == crmcontracts.KnowledgeEmbedStateComplete; corpusComplete != allComplete {
		t.Errorf("%s: corpus complete = %v while every document complete = %v", step, corpusComplete, allComplete)
	}
	return byID
}

func TestADocumentsSearchableCountAgreesWithItsCorpus(t *testing.T) {
	ee := newEmbedEnv(t)
	second := ee.upload(t, "second.md", "text/markdown", prose(1))
	ee.ingest(t, second)
	bound := ee.store.WithEmbedIdentity(func() string { return ee.embedder.identity })
	if _, err := bound.EmbedDocument(ee.ctx, ee.doc, ee.embedder); err != nil {
		t.Fatalf("embed: %v", err)
	}

	half := assertDocumentsSumToTheirCorpus(t, ee, bound, "one of two embedded")
	first := half[ee.doc]
	if first.ChunkCount == nil || first.ChunksEmbedded != *first.ChunkCount || first.EmbedState != crmcontracts.KnowledgeEmbedStateComplete {
		t.Errorf("the embedded document: %d of %v passages, state %q, want all of them and complete", first.ChunksEmbedded, first.ChunkCount, first.EmbedState)
	}
	if waiting := half[second]; waiting.ChunksEmbedded != 0 || waiting.EmbedState != crmcontracts.KnowledgeEmbedStateIndexing {
		t.Errorf("the unembedded document: %d passages, state %q, want 0 and indexing", waiting.ChunksEmbedded, waiting.EmbedState)
	}

	if _, err := bound.EmbedDocument(ee.ctx, second, ee.embedder); err != nil {
		t.Fatalf("embed the second: %v", err)
	}
	for _, step := range []struct {
		name  string
		store *knowledge.Store
		want  crmcontracts.KnowledgeEmbedState
	}{
		{"both embedded", bound, crmcontracts.KnowledgeEmbedStateComplete},
		{"vectors under a superseded model", ee.store.WithEmbedIdentity(func() string { return "fake/other@8" }), crmcontracts.KnowledgeEmbedStateIndexing},
		{"no model bound", ee.store, crmcontracts.KnowledgeEmbedStateNoEmbeddingModel},
	} {
		for id, doc := range assertDocumentsSumToTheirCorpus(t, ee, step.store, step.name) {
			if doc.EmbedState != step.want {
				t.Errorf("%s: document %s is %q, want %q", step.name, id, doc.EmbedState, step.want)
			}
		}
	}
}

func TestAShippedPageCannotBeDeleted(t *testing.T) {
	e := Setup(t)
	bootCtx := handbookBootCtx(e.WS)
	var q recordingQueue
	if _, err := knowledge.NewStore(e.DB()).ReconcileHandbook(bootCtx, noSerialize, q.queue()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	page := filedPages(bootCtx, t, e)["README.md"]
	human := e.As(e.Rep1, nil, corpusAdminPerms)

	stored := knowledge.NewStore(e.DB()).WithBlobstore(blobstore.NewMemory())
	if err := stored.DeleteDocument(human, page); !errors.Is(err, knowledge.ErrDocumentShipped) {
		t.Fatalf("delete a shipped page = %v, want ErrDocumentShipped", err)
	}
	// An installation with no object storage is where the handbook matters most,
	// and the refusal there must still name the page, not the storage.
	if err := knowledge.NewStore(e.DB()).DeleteDocument(human, page); !errors.Is(err, knowledge.ErrDocumentShipped) {
		t.Fatalf("delete a shipped page with no object store = %v, want ErrDocumentShipped", err)
	}

	rec := httptest.NewRecorder()
	newKnowledgeHTTP(e).handlers.DeleteCorpusDocument(rec,
		httptest.NewRequest(http.MethodDelete, "/v1/knowledge/documents/x", nil).WithContext(human), openapi_types.UUID(page))
	var problem struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode the refusal: %v (%s)", err, rec.Body.String())
	}
	if rec.Code != http.StatusConflict || problem.Code != "document_shipped" || problem.Detail == "" {
		t.Fatalf("delete over HTTP = %d %+v, want 409 document_shipped with a reader sentence", rec.Code, problem)
	}
	if _, still := filedPages(bootCtx, t, e)["README.md"]; !still {
		t.Fatal("the refused delete removed the page anyway")
	}
}
