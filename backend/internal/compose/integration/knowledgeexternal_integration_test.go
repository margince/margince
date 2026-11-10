// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A document an extension keeps in step with its source: one row per source id,
// replaced when the source changes, left alone when it does not.

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/knowledge"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var gdocsCorpus = knowledge.ManagedCorpus{Source: knowledge.GDocsSource, Name: "Google Docs", Topic: "Synced documents."}

func (ie *ingestEnv) file(t *testing.T, extID, body string, meta map[string]string) (ids.UUID, knowledge.ExternalOutcome, error) {
	t.Helper()
	doc, outcome, err := ie.store.UpsertExternal(ie.ctx, gdocsCorpus, knowledge.ExternalDocument{
		ExternalID: extID, Filename: "Leave policy [" + extID + "].md", ContentType: "text/markdown",
		Content: strings.NewReader(body), Meta: meta,
	}, ie.queue)
	return ids.UUID(doc.Id), outcome, err
}

func TestAnExternalDocumentIsReplacedBySourceIdAndSkippedWhenUnchanged(t *testing.T) {
	ie := newIngestEnv(t)
	meta := map[string]string{"owner": "Lan", "effective": "2026-01-01"}

	id, outcome, err := ie.file(t, "drive-1", prose(2), meta)
	if err != nil || outcome != knowledge.ExternalCreated {
		t.Fatalf("first filing = %v, %v; want created", outcome, err)
	}
	ie.ingest(t, id)
	oldKey := ie.env.WsScalar(t, `SELECT storage_key FROM knowledge_document WHERE id = $1`, id)

	queuedBefore := len(ie.queued)
	again, outcome, err := ie.file(t, "drive-1", prose(2), meta)
	if err != nil || outcome != knowledge.ExternalUnchanged || again != id {
		t.Fatalf("same bytes = %v, %v, id %v; want unchanged on the same document", outcome, err, again)
	}
	if len(ie.queued) != queuedBefore {
		t.Fatalf("unchanged bytes queued an ingest")
	}

	newMeta := map[string]string{"owner": "Lan", "effective": "2026-06-01"}
	replaced, outcome, err := ie.file(t, "drive-1", prose(3), newMeta)
	if err != nil || outcome != knowledge.ExternalReplaced || replaced != id {
		t.Fatalf("new bytes = %v, %v, id %v; want replaced on the same document", outcome, err, replaced)
	}
	if got := ie.env.WsCount(t, `SELECT count(*) FROM knowledge_chunk WHERE document_id = $1`, id); got != 0 {
		t.Fatalf("a replacement left %d old passages", got)
	}
	if _, _, err := ie.blob.Get(ie.ctx, oldKey); !errors.Is(err, blobstore.ErrNotFound) {
		t.Fatalf("the replaced bytes are still stored: %v", err)
	}
	if got := ie.env.WsCount(t, `SELECT count(*) FROM knowledge_document WHERE external_id = 'drive-1'`); got != 1 {
		t.Fatalf("%d documents for one source id", got)
	}
	n := ie.ingest(t, id)
	if got := ie.env.WsCount(t, `SELECT count(*) FROM knowledge_chunk WHERE document_id = $1`, id); got != n {
		t.Fatalf("after the re-ingest the document holds %d passages, want %d", got, n)
	}
	if got := ie.env.WsScalar(t, `SELECT meta->>'effective' FROM knowledge_document WHERE id = $1`, id); got != "2026-06-01" {
		t.Fatalf("the meta was not replaced: %q", got)
	}
}

func TestAnExternalDocumentBeingReadIsNotReplacedUnderTheReader(t *testing.T) {
	ie := newIngestEnv(t)
	id, _, err := ie.file(t, "drive-2", prose(2), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ie.store.BeginIngest(ie.ctx, id); err != nil {
		t.Fatal(err)
	}
	_, _, err = ie.file(t, "drive-2", prose(3), nil)
	var busy *knowledge.IngestBusyError
	if !errors.As(err, &busy) {
		t.Fatalf("replacing a running ingest = %v, want IngestBusyError", err)
	}
}

func TestAnExternalFilingIsHeldToTheUploadBounds(t *testing.T) {
	ie := newIngestEnv(t)
	_, _, err := ie.store.UpsertExternal(ie.ctx, gdocsCorpus, knowledge.ExternalDocument{
		ExternalID: "x", Filename: "scan.pdf", ContentType: "application/pdf", Content: strings.NewReader("%PDF"),
	}, ie.queue)
	var unsupported *knowledge.UnsupportedTypeError
	if !errors.As(err, &unsupported) {
		t.Fatalf("a PDF = %v, want UnsupportedTypeError", err)
	}
	if _, _, err := ie.file(t, "", "text", nil); err == nil {
		t.Fatal("an empty external id was accepted")
	}
	tooMany := map[string]string{}
	for i := range 17 {
		tooMany[strings.Repeat("k", i+1)] = "v"
	}
	if _, _, err := ie.file(t, "y", "text", tooMany); err == nil {
		t.Fatal("17 meta fields were accepted")
	}
}

func TestDeletingAnExternalDocumentRemovesItAndIsRepeatable(t *testing.T) {
	ie := newIngestEnv(t)
	id, _, err := ie.file(t, "drive-3", prose(2), nil)
	if err != nil {
		t.Fatal(err)
	}
	ie.ingest(t, id)
	if err := ie.store.DeleteExternal(ie.ctx, knowledge.GDocsSource, "drive-3"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if got := ie.env.WsCount(t, `SELECT count(*) FROM knowledge_document WHERE id = $1`, id); got != 0 {
		t.Fatal("the document survived its delete")
	}
	if err := ie.store.DeleteExternal(ie.ctx, knowledge.GDocsSource, "drive-3"); err != nil {
		t.Fatalf("a second delete = %v, want nil", err)
	}
}

func TestTheManagedSourceCheckAcceptsGdocsAndRefusesAnUnknownSource(t *testing.T) {
	e := Setup(t)
	e.WsExec(t, `INSERT INTO knowledge_corpus (name, topic_statement, managed_source, captured_by)
	             VALUES ('g', 't', 'gdocs', 'test')`)
	if err := e.WsExecErr(t, `INSERT INTO knowledge_corpus (name, topic_statement, managed_source, captured_by)
	             VALUES ('b', 't', 'bogus', 'test')`); err == nil {
		t.Fatal("an unknown managed source was accepted")
	}
}

func TestTheSourceIdsOfLiveExternalDocumentsAreListedAndShrinkOnDelete(t *testing.T) {
	ie := newIngestEnv(t)
	for _, id := range []string{"drive-b", "drive-a"} {
		if _, _, err := ie.file(t, id, prose(1)+id, nil); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ie.store.ExternalIDs(ie.ctx, knowledge.GDocsSource)
	if err != nil || strings.Join(got, ",") != "drive-a,drive-b" {
		t.Fatalf("ids = %v, %v", got, err)
	}
	if err := ie.store.DeleteExternal(ie.ctx, knowledge.GDocsSource, "drive-a"); err != nil {
		t.Fatal(err)
	}
	if got, _ := ie.store.ExternalIDs(ie.ctx, knowledge.GDocsSource); strings.Join(got, ",") != "drive-b" {
		t.Fatalf("after delete: %v", got)
	}
}
