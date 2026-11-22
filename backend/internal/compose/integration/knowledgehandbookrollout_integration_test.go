// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A rolling update runs two releases at once: the api files this release's
// pages while an older worker or api still holds the previous bytes.

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/knowledge"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const newerRelease = "sha256:a-newer-release"

// filedHandbookPage files the handbook and returns one page with its checksum.
func (ie *ingestEnv) filedHandbookPage(t *testing.T) (page ids.UUID, checksum string, q *recordingQueue) {
	t.Helper()
	q = &recordingQueue{}
	if _, err := ie.store.ReconcileHandbook(handbookBootCtx(ie.env.WS), noSerialize, q.queue()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	page = q.queued[0]
	if err := ie.env.Pool.QueryRow(context.Background(), `SELECT checksum FROM knowledge_document WHERE id = $1`, page).Scan(&checksum); err != nil {
		t.Fatalf("read the checksum: %v", err)
	}
	return page, checksum, q
}

func (ie *ingestEnv) setPage(t *testing.T, page ids.UUID, checksum, status, detail string) {
	t.Helper()
	if _, err := ie.env.Pool.Exec(context.Background(),
		`UPDATE knowledge_document SET checksum = $2, ingest_status = $3, ingest_detail = NULLIF($4, '') WHERE id = $1`,
		page, checksum, status, detail); err != nil {
		t.Fatalf("set the page: %v", err)
	}
}

func (ie *ingestEnv) pageStatus(t *testing.T, page ids.UUID) (status string, chunks int) {
	t.Helper()
	if err := ie.env.Pool.QueryRow(context.Background(),
		`SELECT d.ingest_status, (SELECT count(*) FROM knowledge_chunk c WHERE c.document_id = d.id) FROM knowledge_document d WHERE d.id = $1`,
		page).Scan(&status, &chunks); err != nil {
		t.Fatalf("read the page: %v", err)
	}
	return status, chunks
}

func TestAPageReplacedWhileAnOlderWorkerIngestsItIsNotMarkedDoneByThatWorker(t *testing.T) {
	ie := newIngestEnv(t)
	page, current, _ := ie.filedHandbookPage(t)
	old, err := ie.store.BeginIngest(ie.ctx, page)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	ie.setPage(t, page, newerRelease, "queued", "")

	chunks := knowledge.ChunkText(prose(2))
	if err := ie.store.WriteChunks(ie.ctx, page, old, chunks); !errors.Is(err, knowledge.ErrIngestSuperseded) {
		t.Fatalf("writing old passages after the replacement: %v, want ErrIngestSuperseded", err)
	}
	if err := ie.store.FinishIngest(ie.ctx, page, old, len(chunks)); !errors.Is(err, knowledge.ErrIngestSuperseded) {
		t.Fatalf("finishing after the replacement: %v, want ErrIngestSuperseded", err)
	}
	if status, n := ie.pageStatus(t, page); status != "queued" || n != 0 {
		t.Fatalf("the page is %q with %d passages, want the replacement's queued state untouched", status, n)
	}

	ie.setPage(t, page, current, "queued", "")
	fresh, err := ie.store.BeginIngest(ie.ctx, page)
	if err != nil {
		t.Fatalf("the replacement job begins: %v", err)
	}
	if err := ie.store.WriteChunks(ie.ctx, page, fresh, chunks); err != nil {
		t.Fatalf("the replacement writes: %v", err)
	}
	if err := ie.store.FinishIngest(ie.ctx, page, fresh, len(chunks)); err != nil {
		t.Fatalf("the replacement finishes: %v", err)
	}
	if status, n := ie.pageStatus(t, page); status != "done" || n != len(chunks) {
		t.Fatalf("the page is %q with %d passages, want done with %d", status, n, len(chunks))
	}
}

func TestOnlyAReleaseMismatchFailureIsQueuedAgainByTheNextBoot(t *testing.T) {
	ie := newIngestEnv(t)
	page, current, q := ie.filedHandbookPage(t)
	boot := func() []ids.UUID {
		q.queued = nil
		if _, err := ie.store.ReconcileHandbook(handbookBootCtx(ie.env.WS), noSerialize, q.queue()); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		return q.queued
	}

	ie.setPage(t, page, current, "failed", "The AI provider was rate limiting this installation.")
	if slices.Contains(boot(), page) {
		t.Fatal("a page failed by a provider error was queued again on every boot")
	}
	ie.setPage(t, page, current, "failed", knowledge.HandbookPageStaleDetail)
	if !slices.Contains(boot(), page) {
		t.Fatal("a page failed by the release mismatch was not queued again")
	}
	if status, _ := ie.pageStatus(t, page); status != "queued" {
		t.Fatalf("the page is %q after the boot, want queued", status)
	}
}

func TestADownloadOfAHandbookPageThisBinaryHoldsDifferentlyIsNotFound(t *testing.T) {
	ie := newIngestEnv(t)
	page, current, _ := ie.filedHandbookPage(t)
	_, body, err := ie.store.OpenForDownload(ie.ctx, page)
	if err != nil {
		t.Fatalf("download of the page this binary ships: %v", err)
	}
	if err := body.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	ie.setPage(t, page, newerRelease, "done", "")
	if _, _, err := ie.store.OpenForDownload(ie.ctx, page); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("download of older bytes under a newer checksum: %v, want ErrNotFound", err)
	}
	ie.setPage(t, page, current, "done", "")
}
