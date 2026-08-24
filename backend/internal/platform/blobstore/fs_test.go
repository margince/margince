// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package blobstore_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/gradionhq/margince/backend/internal/platform/blobstore"
)

func TestFilesystemStorePutGetRoundTrip(t *testing.T) {
	store := newFilesystem(t)
	ctx := t.Context()
	body := []byte("the quick brown fox")

	if err := store.Put(ctx, "ws/attachment/a", bytes.NewReader(body), int64(len(body)), "text/plain"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	r, obj, err := store.Get(ctx, "ws/attachment/a")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer func() {
		if cerr := r.Close(); cerr != nil {
			t.Errorf("Close: %v", cerr)
		}
	}()
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("bytes = %q, want %q", got, body)
	}
	if obj.Size != int64(len(body)) {
		t.Errorf("Object.Size = %d, want %d", obj.Size, len(body))
	}
	if obj.ContentType != "text/plain" {
		t.Errorf("Object.ContentType = %q, want text/plain", obj.ContentType)
	}
}

// The content type is metadata the filesystem does not carry, so it survives a
// process restart only if the store persisted it beside the bytes. A fresh
// store over the same root is what a restart looks like from here.
func TestFilesystemStoreContentTypeSurvivesAFreshStore(t *testing.T) {
	root := t.TempDir()
	ctx := t.Context()
	first := newFilesystemAt(t, root)
	put(ctx, t, first, "ws/attachment/a", "hello")
	if err := first.Put(ctx, "ws/attachment/b", bytes.NewReader([]byte("pdf bytes")), 9, "application/pdf"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	second := newFilesystemAt(t, root)
	r, obj, err := second.Get(ctx, "ws/attachment/b")
	if err != nil {
		t.Fatalf("Get from a fresh store over the same root: %v", err)
	}
	if cerr := r.Close(); cerr != nil {
		t.Errorf("Close: %v", cerr)
	}
	if obj.ContentType != "application/pdf" {
		t.Errorf("Object.ContentType = %q, want application/pdf", obj.ContentType)
	}
}

func TestFilesystemStoreGetMissingReturnsNotFound(t *testing.T) {
	_, _, err := newFilesystem(t).Get(t.Context(), "ws/attachment/absent")
	if !errors.Is(err, blobstore.ErrNotFound) {
		t.Fatalf("Get on a missing key: err = %v, want ErrNotFound", err)
	}
}

func TestFilesystemStoreDeleteIsIdempotent(t *testing.T) {
	store := newFilesystem(t)
	ctx := t.Context()
	put(ctx, t, store, "ws/attachment/a", "body")

	if err := store.Delete(ctx, "ws/attachment/a"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := store.Delete(ctx, "ws/attachment/a"); err != nil {
		t.Fatalf("Delete on an absent key: %v, want nil (idempotent)", err)
	}
	if _, _, err := store.Get(ctx, "ws/attachment/a"); !errors.Is(err, blobstore.ErrNotFound) {
		t.Errorf("the object survived Delete: %v", err)
	}
}

func TestFilesystemStorePutOverwrites(t *testing.T) {
	store := newFilesystem(t)
	ctx := t.Context()
	put(ctx, t, store, "ws/attachment/a", "first")
	put(ctx, t, store, "ws/attachment/a", "second")

	r, obj, err := store.Get(ctx, "ws/attachment/a")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer func() {
		if cerr := r.Close(); cerr != nil {
			t.Errorf("Close: %v", cerr)
		}
	}()
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "second" {
		t.Errorf("bytes = %q, want %q", got, "second")
	}
	if obj.Size != int64(len("second")) {
		t.Errorf("Object.Size = %d, want %d", obj.Size, len("second"))
	}
}

func TestFilesystemStoreDeletePrefixRemovesOnlyThatPrefix(t *testing.T) {
	store := newFilesystem(t)
	ctx := t.Context()
	const mine = "ws-a/attachment/a1"
	put(ctx, t, store, mine, "mine")
	put(ctx, t, store, "ws-abc/attachment/a2", "a sibling tenant whose id extends mine")

	deleted, err := store.DeletePrefix(ctx, "ws-a/")
	if err != nil {
		t.Fatalf("DeletePrefix: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}
	if _, _, err := store.Get(ctx, mine); !errors.Is(err, blobstore.ErrNotFound) {
		t.Errorf("the prefixed object survived: %v", err)
	}
	if _, _, err := store.Get(ctx, "ws-abc/attachment/a2"); err != nil {
		t.Errorf("a sibling tenant's object was deleted: %v", err)
	}
}

func TestFilesystemStoreDeletePrefixRejectsAnUnboundedPrefix(t *testing.T) {
	store := newFilesystem(t)
	ctx := t.Context()
	const key = "ws-a/attachment/a1"
	put(ctx, t, store, key, "mine")

	for _, prefix := range []string{"", "ws-a"} {
		deleted, err := store.DeletePrefix(ctx, prefix)
		if !errors.Is(err, blobstore.ErrInvalidPrefix) {
			t.Errorf("DeletePrefix(%q): err = %v, want ErrInvalidPrefix", prefix, err)
		}
		if deleted != 0 {
			t.Errorf("DeletePrefix(%q): deleted = %d, want 0", prefix, deleted)
		}
	}
	if _, _, err := store.Get(ctx, key); err != nil {
		t.Errorf("the object did not survive the rejected prefixes: %v", err)
	}
}

// A key is opaque to the store and becomes a path, so "../" in one would
// otherwise address bytes outside the root — the store's own tenant boundary is
// the key prefix, and a traversal walks straight through it.
func TestFilesystemStoreRefusesAKeyThatEscapesTheRoot(t *testing.T) {
	root := t.TempDir()
	store := newFilesystemAt(t, root)
	ctx := t.Context()
	outside := filepath.Join(filepath.Dir(root), "escaped")

	err := store.Put(ctx, "../escaped", bytes.NewReader([]byte("x")), 1, "")
	if err == nil {
		t.Fatal("Put with a traversing key: err = nil, want a refusal")
	}
	if _, statErr := os.Stat(outside); statErr == nil {
		t.Errorf("Put wrote outside the root, at %s", outside)
	}
	if _, _, err := store.Get(ctx, "../escaped"); err == nil {
		t.Error("Get with a traversing key: err = nil, want a refusal")
	}
}

func TestFilesystemStoreHealthReportsAnUnwritableRoot(t *testing.T) {
	root := t.TempDir()
	store := newFilesystemAt(t, root)
	if err := store.Health(t.Context()); err != nil {
		t.Fatalf("Health on a writable root: %v", err)
	}

	// The root is the store: taken away, it is not reachable, which is what
	// /readyz needs to be told rather than discovering on the next upload.
	if err := os.RemoveAll(root); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	if err := store.Health(t.Context()); err == nil {
		t.Error("Health with the root gone: err = nil, want a failure")
	}
}

func TestNewFilesystemRefusesAnEmptyRoot(t *testing.T) {
	if _, err := blobstore.NewFilesystem(""); err == nil {
		t.Fatal("NewFilesystem(\"\"): err = nil, want a refusal")
	}
}

func newFilesystem(t *testing.T) blobstore.Store {
	t.Helper()
	return newFilesystemAt(t, t.TempDir())
}

func newFilesystemAt(t *testing.T, root string) blobstore.Store {
	t.Helper()
	store, err := blobstore.NewFilesystem(root)
	if err != nil {
		t.Fatalf("NewFilesystem(%q): %v", root, err)
	}
	return store
}
