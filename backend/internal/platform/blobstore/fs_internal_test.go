// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package blobstore

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A directory sync fails AFTER the rename, which is the state these tests exist
// for: the file is already visible to every reader while the write reports an
// error. Injected through fsStore.sync because there is no way to make a real
// filesystem fail exactly there.
var errSyncFailed = errors.New("the directory sync failed")

func failingSync(string) error { return errSyncFailed }

func storeWithFailingSync(t *testing.T, root string) *fsStore {
	t.Helper()
	for _, dir := range []string{fsBlobDir, fsMetaDir} {
		if err := os.MkdirAll(filepath.Join(root, dir), fsDirPerm); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}
	return &fsStore{root: root, sync: failingSync}
}

// writeFileAtomic's bool is what keeps a caller from undoing something that
// happened. A sync failure comes after the rename, so the file IS published and
// the error is about durability, not about whether the write landed.
func TestWriteFileAtomicReportsPublicationWhenTheSyncFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "object")

	published, err := writeFileAtomic(path, strings.NewReader("bytes"), failingSync)

	if !errors.Is(err, errSyncFailed) {
		t.Fatalf("err = %v, want the sync failure", err)
	}
	if !published {
		t.Error("published = false after a successful rename, so a caller would undo a file that is already readable")
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Errorf("the file is not there despite the rename succeeding: %v", statErr)
	}
}

// The metadata is published first, so a sync failure on IT leaves a content type
// visible for bytes that were never written. Put has to take that back.
func TestPutUndoesPublishedMetadataWhenItsSyncFails(t *testing.T) {
	root := t.TempDir()
	store := storeWithFailingSync(t, root)

	err := store.Put(t.Context(), "ws/attachment/a", bytes.NewReader([]byte("bytes")), 5, "application/pdf")

	if !errors.Is(err, errSyncFailed) {
		t.Fatalf("err = %v, want the sync failure", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, fsMetaDir, "ws", "attachment", "a")); statErr == nil {
		t.Error("the published metadata survived, so a key with no object has a content type")
	}
}

// The mirror case: an OVERWRITE whose metadata sync fails must leave the stored
// object's own content type, not the half-published new one.
func TestPutRestoresThePreviousMetadataWhenItsSyncFails(t *testing.T) {
	root := t.TempDir()
	stored := &fsStore{root: root, sync: syncDir}
	for _, dir := range []string{fsBlobDir, fsMetaDir} {
		if err := os.MkdirAll(filepath.Join(root, dir), fsDirPerm); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}
	ctx := t.Context()
	if err := stored.Put(ctx, "ws/attachment/a", bytes.NewReader([]byte("original")), 8, "application/pdf"); err != nil {
		t.Fatalf("the first Put: %v", err)
	}

	failing := &fsStore{root: root, sync: failingSync}
	if err := failing.Put(ctx, "ws/attachment/a", bytes.NewReader([]byte("replacement")), 11, "image/png"); !errors.Is(err, errSyncFailed) {
		t.Fatalf("err = %v, want the sync failure", err)
	}

	_, obj, err := stored.Get(ctx, "ws/attachment/a")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if obj.ContentType != "application/pdf" {
		t.Errorf("Object.ContentType = %q, want the stored object's own application/pdf", obj.ContentType)
	}
}
