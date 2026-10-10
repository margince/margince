// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A failed ingest deletes the stored bytes and keeps the row, and the advice on
// the failed document is to upload the file again.
//
// Downloading it answered 500: the object store reported a missing key for a
// state the product had itself created.
//
// Uploading the same bytes again was refused as already filed, so the advice
// could not be followed.

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/knowledge"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestDownloadingAFailedDocumentIsNotAServerFault(t *testing.T) {
	ie := newIngestEnv(t)
	docID := ie.upload(t, "unreadable.txt", "text/plain", prose(2))
	if err := ie.store.FailIngest(ie.ctx, docID, "the stored file could not be read"); err != nil {
		t.Fatalf("failing the ingest: %v", err)
	}

	_, body, err := ie.store.OpenForDownload(ie.ctx, docID)
	if body != nil {
		t.Cleanup(func() {
			if cerr := body.Close(); cerr != nil {
				t.Errorf("closing the body: %v", cerr)
			}
		})
	}
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("download of a failed document → %v, want ErrNotFound; the contract allows 200, 401, 403 or 404", err)
	}
}

// The advice on a failed document is to upload the file again, so the same
// bytes are accepted and the row is re-used.
func TestReuploadingAFailedDocumentsBytesIsAccepted(t *testing.T) {
	ie := newIngestEnv(t)
	body := prose(2)
	docID := ie.upload(t, "operating.md", "text/markdown", body)
	if err := ie.store.FailIngest(ie.ctx, docID, "the stored file could not be read"); err != nil {
		t.Fatalf("failing the ingest: %v", err)
	}

	again, err := ie.store.UploadDocument(ie.ctx, knowledge.NewDocument{
		CorpusID:    ie.corpus,
		Filename:    "operating.md",
		ContentType: "text/markdown",
		Content:     strings.NewReader(body),
	}, ie.queue)
	if err != nil {
		t.Fatalf("re-uploading a failed document's bytes: %v, want it accepted", err)
	}
	// The failed row is gone rather than sitting beside the new one, so the
	// corpus does not list the same bytes twice.
	if ids.UUID(again.Id) == docID {
		t.Error("the failed row was kept, and its stored bytes were already deleted")
	}
	// queued is what a fresh upload carries, and the CHECK on the column
	// permits only queued, running, done and failed.
	if again.IngestStatus != "queued" {
		t.Errorf("ingest_status = %q, want queued so the worker tries it again", again.IngestStatus)
	}
	if again.IngestDetail != nil && *again.IngestDetail != "" {
		t.Errorf("ingest_detail = %q, want no reason carried onto a fresh attempt", *again.IngestDetail)
	}
	docs, err := ie.store.ListDocuments(ie.ctx, ie.corpus)
	if err != nil {
		t.Fatalf("listing the corpus: %v", err)
	}
	if len(docs) != 1 {
		t.Errorf("the corpus lists %d documents, want 1", len(docs))
	}
	// And it downloads, which is what the failed row could not do.
	_, content, err := ie.store.OpenForDownload(ie.ctx, ids.UUID(again.Id))
	if err != nil {
		t.Fatalf("downloading the re-uploaded document: %v", err)
	}
	if cerr := content.Close(); cerr != nil {
		t.Errorf("closing the re-uploaded document: %v", cerr)
	}
}

// A live document still refuses the same bytes: the relaxation is for a row
// that holds none, not for duplicates.
func TestAnIngestedDocumentStillRefusesTheSameBytes(t *testing.T) {
	ie := newIngestEnv(t)
	body := prose(2)
	ie.upload(t, "operating.md", "text/markdown", body)

	_, err := ie.store.UploadDocument(ie.ctx, knowledge.NewDocument{
		CorpusID:    ie.corpus,
		Filename:    "copy.md",
		ContentType: "text/markdown",
		Content:     strings.NewReader(body),
	}, ie.queue)
	if _, isFiled := errors.AsType[*knowledge.AlreadyFiledError](err); !isFiled {
		t.Fatalf("err = %v, want AlreadyFiledError for bytes a live document holds", err)
	}
}
