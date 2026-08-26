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

	"github.com/margince/margince/backend/internal/platform/blobstore"
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
	// Where the write would ACTUALLY land if the refusal regressed:
	// filepath.Join(root, "blob", "../escaped") collapses to <root>/escaped,
	// because the tree name is itself a component. Asserting on the parent of
	// root instead looked defensive and checked nothing.
	outside := filepath.Join(root, "escaped")

	err := store.Put(ctx, "../escaped", bytes.NewReader([]byte("x")), 1, "")
	if err == nil {
		t.Fatal("Put with a traversing key: err = nil, want a refusal")
	}
	if _, statErr := os.Stat(outside); statErr == nil {
		t.Errorf("Put escaped its tree, writing %s", outside)
	}
	if _, _, err := store.Get(ctx, "../escaped"); err == nil {
		t.Error("Get with a traversing key: err = nil, want a refusal")
	}
}

// A backslash is never legitimate in a key — WorkspaceKey builds "/"-separated
// keys — and on Windows it IS a separator, so `ws-a\..\ws-b\x` walks out of
// the tenant prefix past a "/"-only element scan. The refusal is the same on
// every platform on purpose: a key one host accepts is a key the other accepts.
func TestFilesystemStoreRefusesAKeyWithWindowsSeparators(t *testing.T) {
	store := newFilesystem(t)
	ctx := t.Context()

	for _, key := range []string{
		`ws-a\..\ws-b\attachment\x`,
		`ws-a\attachment\x`,
		`C:\ws-a\attachment\x`,
	} {
		if err := store.Put(ctx, key, bytes.NewReader([]byte("x")), 1, ""); !errors.Is(err, blobstore.ErrInvalidKey) {
			t.Errorf("Put(%q): err = %v, want ErrInvalidKey", key, err)
		}
		if _, _, err := store.Get(ctx, key); !errors.Is(err, blobstore.ErrInvalidKey) {
			t.Errorf("Get(%q): err = %v, want ErrInvalidKey", key, err)
		}
	}
}

// The metadata is published BEFORE the bytes, so the blob stays the existence
// record and no object is ever visible without the content type describing it.
// The cost of that order is a metadata file with no object behind it when the
// bytes fail, so the failure path removes it again.
//
// The failure is forced the way it can actually happen: a key that shadows an
// existing object's path. "ws/a" is a FILE, so creating the directory "ws/a"
// that "ws/a/b" needs cannot succeed.
func TestFilesystemStorePutLeavesNoMetadataWhenTheBytesFail(t *testing.T) {
	root := t.TempDir()
	store := newFilesystemAt(t, root)
	ctx := t.Context()
	put(ctx, t, store, "ws/a", "the object in the way")

	err := store.Put(ctx, "ws/a/b", bytes.NewReader([]byte("x")), 1, "text/plain")
	if err == nil {
		t.Fatal("Put under a key shadowed by an existing object succeeded")
	}
	if _, statErr := os.Stat(filepath.Join(root, "meta", "ws", "a", "b")); statErr == nil {
		t.Error("the metadata written before the bytes survived their failure")
	}
	if _, _, getErr := store.Get(ctx, "ws/a/b"); !errors.Is(getErr, blobstore.ErrNotFound) {
		t.Errorf("Get after a failed Put: err = %v, want ErrNotFound", getErr)
	}
}

// Health has to answer for both trees: a Put writes metadata first, so a
// read-only meta tree fails every typed upload before the blob is attempted —
// and a readiness probe that only checked blob would report the store healthy
// the whole time.
func TestFilesystemStoreHealthReportsAnUnwritableMetaTree(t *testing.T) {
	root := t.TempDir()
	store := newFilesystemAt(t, root)
	if err := store.Health(t.Context()); err != nil {
		t.Fatalf("Health on a writable store: %v", err)
	}

	if err := os.RemoveAll(filepath.Join(root, "meta")); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	if err := store.Health(t.Context()); err == nil {
		t.Error("Health with the meta tree gone: err = nil, want a failure")
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

// countFiles skips dotfiles so this store's own scratch — a `.tmp-*` staging
// file from a Put that died mid-write, a `.health-*` probe — is never reported as
// an object deleted. That is only sound while no OBJECT can be named like one, so
// the key refusal and the count are one rule in two places.
func TestFilesystemStoreRefusesADotLedKeyElement(t *testing.T) {
	store := newFilesystem(t)
	ctx := t.Context()

	for _, key := range []string{".tmp-x", "ws/.health-x", "ws/./attachment/x", "ws/.hidden/x"} {
		if err := store.Put(ctx, key, bytes.NewReader([]byte("x")), 1, ""); !errors.Is(err, blobstore.ErrInvalidKey) {
			t.Errorf("Put(%q): err = %v, want ErrInvalidKey", key, err)
		}
	}
}

// A staging file left behind by a Put that died mid-write sits in the same
// directory as the objects, under the prefix being swept. It is litter, not an
// object, and reporting it as one deleted makes the count a number nobody can
// reconcile with what the caller had.
func TestFilesystemStoreDeletePrefixCountsObjectsNotScratch(t *testing.T) {
	root := t.TempDir()
	store := newFilesystemAt(t, root)
	ctx := t.Context()
	put(ctx, t, store, "ws-a/attachment/a1", "an object")

	// Exactly what a crashed Put leaves: os.CreateTemp's own naming, beside the
	// object, inside the swept prefix.
	staging, err := os.CreateTemp(filepath.Join(root, "blob", "ws-a", "attachment"), ".tmp-*")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if err := staging.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	deleted, err := store.DeletePrefix(ctx, "ws-a/")
	if err != nil {
		t.Fatalf("DeletePrefix: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1 — the staging file was counted as an object", deleted)
	}
	if _, statErr := os.Stat(staging.Name()); statErr == nil {
		t.Error("the staging file survived the prefix sweep")
	}
}

// failingReader fails after handing over some bytes, which is what a truncated
// upload or a dropped connection looks like from inside Put.
type failingReader struct{ read bool }

func (f *failingReader) Read(p []byte) (int, error) {
	if f.read {
		return 0, errors.New("the upload stopped")
	}
	f.read = true
	copy(p, "half")
	return 4, nil
}

// A failed OVERWRITE must leave the stored object exactly as it was — bytes and
// content type both. Publishing the metadata first is what keeps a new object
// from ever being visible untyped, and it is also what makes this case possible:
// without the restore, the failure would delete the published metadata while the
// old bytes stayed, so a Put that stored nothing would still have changed the
// stored object's content type to "".
func TestFilesystemStoreFailedOverwriteKeepsThePreviousObject(t *testing.T) {
	store := newFilesystem(t)
	ctx := t.Context()
	const key = "ws/attachment/a"
	if err := store.Put(ctx, key, bytes.NewReader([]byte("the original")), 12, "application/pdf"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	if err := store.Put(ctx, key, &failingReader{}, 4, "image/png"); err == nil {
		t.Fatal("Put with a failing reader succeeded")
	}

	r, obj, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get after the failed overwrite: %v", err)
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
	if string(got) != "the original" {
		t.Errorf("bytes = %q, want the original object", got)
	}
	if obj.ContentType != "application/pdf" {
		t.Errorf("Object.ContentType = %q, want the original application/pdf", obj.ContentType)
	}
}

// The same failure on a key that held NOTHING must leave nothing: the metadata
// published before the bytes is removed rather than restored, so a failed first
// write does not leave a content type with no object under it.
func TestFilesystemStoreFailedFirstWriteLeavesNothing(t *testing.T) {
	root := t.TempDir()
	store := newFilesystemAt(t, root)
	ctx := t.Context()

	if err := store.Put(ctx, "ws/attachment/new", &failingReader{}, 4, "image/png"); err == nil {
		t.Fatal("Put with a failing reader succeeded")
	}

	if _, statErr := os.Stat(filepath.Join(root, "meta", "ws", "attachment", "new")); statErr == nil {
		t.Error("the metadata published before the bytes survived their failure")
	}
	if _, _, err := store.Get(ctx, "ws/attachment/new"); !errors.Is(err, blobstore.ErrNotFound) {
		t.Errorf("Get after a failed first write: err = %v, want ErrNotFound", err)
	}
}
