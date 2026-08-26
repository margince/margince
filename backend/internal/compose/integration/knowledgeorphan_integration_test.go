// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The bytes an upload wrote must not outlive the write that failed.
//
// Every sweep in the knowledge module walks ROWS — abandoned ingests, drifted
// bindings, deleted documents. An object no row names is therefore reachable by
// nothing and collected by nothing: it is a permanent leak of a tenant's file
// content, still readable by anyone who can enumerate the bucket, long after
// the upload that produced it was refused.
//
// The duplicate race is the ordinary way to get one. Two uploads of identical
// bytes both clear the pre-flight check, both call Put, and the loser is
// refused by the unique index inside its transaction — after its own object
// is already stored.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gradionhq/margince/backend/internal/modules/knowledge"
	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/platform/blobstore"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// countingBlobstore records what was written and what was removed, so a test
// can assert the difference rather than the calls. It wraps the real memory
// store rather than replacing it: the upload's behaviour must be measured
// against a store that actually stores.
type countingBlobstore struct {
	blobstore.Store
	mu   sync.Mutex
	put  []string
	gone map[string]bool
}

func newCountingBlobstore() *countingBlobstore {
	return &countingBlobstore{Store: blobstore.NewMemory(), gone: map[string]bool{}}
}

func (c *countingBlobstore) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	c.mu.Lock()
	c.put = append(c.put, key)
	c.mu.Unlock()
	return c.Store.Put(ctx, key, r, size, contentType)
}

func (c *countingBlobstore) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	c.gone[key] = true
	c.mu.Unlock()
	return c.Store.Delete(ctx, key)
}

// orphans are keys that were written and never removed.
func (c *countingBlobstore) orphans(named map[string]bool) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var left []string
	for _, k := range c.put {
		if !c.gone[k] && !named[k] {
			left = append(left, k)
		}
	}
	return left
}

// A failure INSIDE the write transaction, after the bytes are already stored.
// The ingest queue refusing is the deterministic way to reach that state; the
// duplicate race reaches the same place by luck, and the cleanup is the same
// either way.
//
// The pre-flight duplicate check is deliberately NOT the case under test here:
// it refuses before Put and so never leaves anything behind. Asserting on it
// would pass without the cleanup code existing at all.
func TestAWriteThatFailsAfterStoringItsBytesRemovesThem(t *testing.T) {
	e := Setup(t)
	blobs := newCountingBlobstore()
	h := &knowledgeHTTP{}
	h.handlers = knowledge.NewHandlers(e.DB()).
		WithUploadLimit(knowledgeUploadCeiling).
		WithBlobstore(blobs).
		WithIngestQueue(func(context.Context, pgx.Tx, ids.UUID) error {
			return errors.New("the queue is refusing work")
		})
	ctx := e.As(e.Rep1, nil, corpusAdminPerms)
	made := httpCorpus(ctx, t, h)

	body, ctype := multipartDocument(t, "operating.md", "text/markdown", []byte(onePassage))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/knowledge/corpora/x/documents", body).WithContext(ctx)
	req.Header.Set("Content-Type", ctype)
	h.handlers.UploadCorpusDocument(rec, req, made.Id)

	if rec.Code < 500 {
		t.Fatalf("an upload whose queue refused answered %d, want a server fault; body %s", rec.Code, rec.Body.String())
	}

	// Put was reached — otherwise this test proves nothing about cleanup.
	blobs.mu.Lock()
	wrote := len(blobs.put)
	blobs.mu.Unlock()
	if wrote == 0 {
		t.Fatal("the upload never reached Put, so this case does not exercise the cleanup at all")
	}

	// And nothing survives it: no row was committed, so every key written here
	// is an orphan unless it was deleted.
	if left := blobs.orphans(map[string]bool{}); len(left) != 0 {
		t.Fatalf("a failed write left %d stored object(s) no row names: %v", len(left), left)
	}
}
