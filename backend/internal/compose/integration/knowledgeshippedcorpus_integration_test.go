// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The handbook set refuses what the next boot would undo or break, and keeps
// the edits that boot honours.

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/modules/knowledge"
	"github.com/margince/margince/backend/internal/modules/knowledge/handbook"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheShippedSetCannotBeArchivedButKeepsItsEdits(t *testing.T) {
	e := Setup(t)
	bootCtx := handbookBootCtx(e.WS)
	store := knowledge.NewStore(e.DB())
	var q recordingQueue
	if _, err := store.ReconcileHandbook(bootCtx, noSerialize, q.queue()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	shipped := handbookCorpusID(bootCtx, t, e)
	human := e.As(e.Rep1, nil, corpusAdminPerms)

	own, err := store.CreateCorpus(human, howTo("Our own pricing notes"))
	if err != nil {
		t.Fatalf("create the workspace's own corpus: %v", err)
	}
	read, err := store.ReadCorpus(human, shipped)
	if err != nil {
		t.Fatalf("read the shipped set: %v", err)
	}
	if !read.Shipped || own.Shipped {
		t.Fatalf("shipped set reports shipped=%v, own set shipped=%v; want true and false", read.Shipped, own.Shipped)
	}

	if err := store.ArchiveCorpus(human, shipped); !errors.Is(err, knowledge.ErrCorpusShipped) {
		t.Fatalf("archive the shipped set = %v, want ErrCorpusShipped", err)
	}
	rec := httptest.NewRecorder()
	newKnowledgeHTTP(e).handlers.ArchiveCorpus(rec,
		httptest.NewRequest(http.MethodDelete, "/v1/knowledge/corpora/x", nil).WithContext(human), openapi_types.UUID(shipped))
	if code, detail := problemOf(t, rec); rec.Code != http.StatusConflict || code != "corpus_shipped" || detail == "" {
		t.Fatalf("archive over HTTP = %d %s, want 409 corpus_shipped with a reader sentence", rec.Code, code)
	}
	if err := store.ArchiveCorpus(human, ids.UUID(own.Id)); err != nil {
		t.Fatalf("a workspace's own set must still archive: %v", err)
	}

	// The boot leaves name and default alone after creation, so an edit stands.
	renamed := "Our handbook"
	if _, err := store.EditCorpus(human, shipped, knowledge.UpdateCorpus{Name: &renamed}); err != nil {
		t.Fatalf("rename the shipped set: %v", err)
	}
	if _, err := store.ReconcileHandbook(bootCtx, noSerialize, q.queue()); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if handbookCorpusID(bootCtx, t, e) != shipped {
		t.Fatal("the refused archive or the reboot replaced the shipped set")
	}
	if after, err := store.ReadCorpus(human, shipped); err != nil || after.Name != renamed {
		t.Fatalf("after a reboot the shipped set is named %q (err %v), want the edit %q kept", after.Name, err, renamed)
	}
}

// An upload into the shipped set is refused. Otherwise a failed page's row is
// swapped for an unshipped copy, and the next boot's reconcile fails on it.
func TestAnUploadCannotReplaceAFailedShippedPage(t *testing.T) {
	e := Setup(t)
	bootCtx := handbookBootCtx(e.WS)
	store := knowledge.NewStore(e.DB()).WithBlobstore(blobstore.NewMemory())
	var q recordingQueue
	if _, err := store.ReconcileHandbook(bootCtx, noSerialize, q.queue()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	shipped := handbookCorpusID(bootCtx, t, e)
	page := filedPages(bootCtx, t, e)["README.md"]
	e.WsExec(t, `UPDATE knowledge_document SET ingest_status = 'failed', ingest_detail = 'The page could not be read.' WHERE id = $1`, page)
	readme, ok := handbook.Open("README.md")
	if !ok {
		t.Fatal("this build ships no README.md, so the scenario cannot be planted")
	}

	human := e.As(e.Rep1, nil, corpusAdminPerms)
	_, err := store.UploadDocument(human, knowledge.NewDocument{
		CorpusID: shipped, Filename: "README.md", ContentType: "text/markdown",
		Content: strings.NewReader(string(readme)),
	}, q.queue())
	if !errors.Is(err, knowledge.ErrCorpusShipped) {
		t.Fatalf("upload into the shipped set = %v, want ErrCorpusShipped", err)
	}
	body, ctype := multipartDocument(t, "README.md", "text/markdown", readme)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/knowledge/corpora/x/documents", body).WithContext(human)
	req.Header.Set("Content-Type", ctype)
	newKnowledgeHTTP(e).handlers.UploadCorpusDocument(rec, req, openapi_types.UUID(shipped))
	if code, _ := problemOf(t, rec); rec.Code != http.StatusConflict || code != "corpus_shipped" {
		t.Fatalf("upload over HTTP = %d %s, want 409 corpus_shipped", rec.Code, code)
	}

	if filedPages(bootCtx, t, e)["README.md"] != page {
		t.Fatal("the failed shipped page was replaced by the refused upload")
	}
	if _, err := store.ReconcileHandbook(bootCtx, noSerialize, q.queue()); err != nil {
		t.Fatalf("the next boot's reconcile failed: %v", err)
	}
}
