// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What happens to a message's files when the capture that stored them ahead of
// its transaction does not keep them.

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// prestageBlob counts puts and deletes and remembers the keys it was given.
type prestageBlob struct {
	blobstore.Store
	beforePut func()

	mu            sync.Mutex
	puts, deletes int
	keys          []string
}

func (b *prestageBlob) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	b.mu.Lock()
	b.puts++
	b.keys = append(b.keys, key)
	before := b.beforePut
	b.beforePut = nil
	b.mu.Unlock()
	if before != nil {
		before()
	}
	return b.Store.Put(ctx, key, r, size, contentType)
}

func (b *prestageBlob) Delete(ctx context.Context, key string) error {
	b.mu.Lock()
	b.deletes++
	b.mu.Unlock()
	return b.Store.Delete(ctx, key)
}

// recordHook runs during the capture transaction, once its files are named.
type recordHook struct {
	capture.FileKeeper
	during func() error
}

func (k recordHook) Record(ctx context.Context, tx pgx.Tx, activity ids.ActivityID, from capture.FileSource, staged []capture.StagedFile) error {
	if err := k.FileKeeper.Record(ctx, tx, activity, from, staged); err != nil {
		return err
	}
	return k.during()
}

func provisionalOf(ctx context.Context, t *testing.T, db *database.DB, keys []string) int {
	t.Helper()
	var n int
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM stored_object_intent WHERE storage_key = ANY($1)`, keys).Scan(&n)
	}); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAFileStoredForACaptureThatFailsAfterNamingItIsDeleted(t *testing.T) {
	ctx, db, tag := captureWorkspace(t)
	blob := &prestageBlob{Store: blobstore.NewMemory()}
	failed := errors.New("the capture fails after naming its files")
	sink := capture.NewSink(db).WithFileKeeper(recordHook{FileKeeper: fileKeeper(db, blob), during: func() error { return failed }})

	if _, err := sink.Upsert(ctx, withFiles(mailRecord("msg-failed-"+tag), onePDF())); !errors.Is(err, failed) {
		t.Fatalf("capture = %v, want the injected failure", err)
	}
	if blob.puts != 1 || blob.deletes != 1 {
		t.Fatalf("put %d and deleted %d objects for a capture that rolled back, want 1 and 1", blob.puts, blob.deletes)
	}
	if n := provisionalOf(ctx, t, db, blob.keys); n != 0 {
		t.Fatalf("%d provisional objects remain after the rollback, want 0", n)
	}
}

func TestAFileStoredBeforeItsThreadTurnedPrivateIsDeleted(t *testing.T) {
	ctx, db, tag := captureWorkspace(t)
	threadKey := "turning-private-" + tag
	blob := &prestageBlob{Store: blobstore.NewMemory()}
	// The verdict lands between the short read and the capture transaction.
	blob.beforePut = func() { markThreadPrivate(ctx, t, db, threadKey) }
	sink := capture.NewSink(db).WithFileKeeper(fileKeeper(db, blob))

	rec := withFiles(mailRecord("msg-turned-"+tag), onePDF())
	rec.ThreadKey = threadKey
	if _, err := sink.Upsert(ctx, rec); err != nil {
		t.Fatalf("capture: %v", err)
	}
	if blob.puts != 1 || blob.deletes != 1 {
		t.Fatalf("put %d and deleted %d objects for a message that turned private, want 1 and 1", blob.puts, blob.deletes)
	}
	requireNamedWithoutBytes(t, filesFor(ctx, t, db, "msg-turned-"+tag))
	if n := provisionalOf(ctx, t, db, blob.keys); n != 0 {
		t.Fatalf("%d provisional objects remain, want 0 (keys %v)", n, blob.keys)
	}
}
