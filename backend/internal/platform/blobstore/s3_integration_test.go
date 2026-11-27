// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package blobstore_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// testRegion is what this lane names its region. Any value satisfies MinIO; the
// point is that the lane chooses one, because blobstore.New refuses an empty
// region and a lane that relied on a default would stop exercising the product.
const testRegion = "us-east-1"

// newS3Store builds a real MinIO-backed store from the MARGINCE_TEST_BLOBSTORE_*
// contract. Like every integration test here, it fails loudly without its
// dependency — it never skips (a skipped blobstore test looks exactly like a
// passing one).
func newS3Store(t *testing.T) blobstore.Store {
	t.Helper()
	endpoint := os.Getenv("MARGINCE_TEST_BLOBSTORE_ENDPOINT")
	if endpoint == "" {
		t.Fatal("MARGINCE_TEST_BLOBSTORE_ENDPOINT not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	store, err := blobstore.New(t.Context(), blobstore.Config{
		Endpoint:  endpoint,
		AccessKey: os.Getenv("MARGINCE_TEST_BLOBSTORE_ACCESS_KEY"),
		SecretKey: os.Getenv("MARGINCE_TEST_BLOBSTORE_SECRET_KEY"),
		Bucket:    os.Getenv("MARGINCE_TEST_BLOBSTORE_BUCKET"),
		// MinIO ignores the value; New requires one because on real S3 it decides
		// where the bucket is created. The lane names it rather than inheriting a
		// default, which is the property under test everywhere else.
		Region: testRegion,
	})
	if err != nil {
		t.Fatalf("blobstore.New: %v", err)
	}
	return store
}

// TestNewFailsWhenStoreUnreachable is tagged because its subject IS a dial. It
// reaches a dead port rather than MinIO, so it touches no bucket — but the unit
// lane is hermetic: no test there opens a real connection at all. A dial belongs
// in the dialing lane whether or not anything answers.
func TestNewFailsWhenStoreUnreachable(t *testing.T) {
	t.Parallel()
	// A store that never comes up must fail the boot, not hang: the connect
	// retry is bounded by the context deadline.
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	_, err := blobstore.New(ctx, blobstore.Config{
		Endpoint: "127.0.0.1:1", AccessKey: "x", SecretKey: "y", Bucket: "b",
	})
	if err == nil {
		t.Fatal("New against an unreachable endpoint should fail within the deadline")
	}
}

func TestFromConfigConfiguredConnects(t *testing.T) {
	t.Parallel()
	endpoint := os.Getenv("MARGINCE_TEST_BLOBSTORE_ENDPOINT")
	if endpoint == "" {
		t.Fatal("MARGINCE_TEST_BLOBSTORE_ENDPOINT not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	// The suite's own MARGINCE_TEST_BLOBSTORE_* values are handed straight to
	// the package as the settings it would otherwise read, so the test no longer
	// rewrites the process environment to steer the code under it.
	store, configured, err := fromVars(t, map[string]string{
		blobstore.EnvEndpoint:  endpoint,
		blobstore.EnvAccessKey: os.Getenv("MARGINCE_TEST_BLOBSTORE_ACCESS_KEY"),
		blobstore.EnvSecretKey: os.Getenv("MARGINCE_TEST_BLOBSTORE_SECRET_KEY"),
		blobstore.EnvBucket:    os.Getenv("MARGINCE_TEST_BLOBSTORE_BUCKET"),
		blobstore.EnvRegion:    testRegion,
	})
	if err != nil {
		t.Fatalf("FromConfig: %v", err)
	}
	if !configured {
		t.Fatal("FromConfig reported not-configured with the endpoint set")
	}
	if err := store.Health(t.Context()); err != nil {
		t.Fatalf("Health of the env-built store: %v", err)
	}
}

func TestS3StoreRoundTrip(t *testing.T) {
	t.Parallel()
	store := newS3Store(t)
	ctx := t.Context()
	// A fresh workspace id keeps this test's keys clear of every other
	// test sharing the bucket.
	key := blobstore.WorkspaceKey(ids.New[ids.WorkspaceKind](), "attachment", "roundtrip")
	body := []byte("s3 round trip payload")

	if err := store.Put(ctx, key, bytes.NewReader(body), int64(len(body)), "application/octet-stream"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Delete(context.Background(), key); err != nil {
			t.Errorf("cleanup Delete: %v", err)
		}
	})

	r, obj, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, err := io.ReadAll(r)
	if cerr := r.Close(); cerr != nil {
		t.Errorf("Close: %v", cerr)
	}
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("bytes = %q, want %q", got, body)
	}
	if obj.Size != int64(len(body)) {
		t.Errorf("Object.Size = %d, want %d", obj.Size, len(body))
	}
	if obj.ContentType != "application/octet-stream" {
		t.Errorf("Object.ContentType = %q, want application/octet-stream", obj.ContentType)
	}

	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, _, err := store.Get(ctx, key); !errors.Is(err, blobstore.ErrNotFound) {
		t.Fatalf("Get after Delete: err = %v, want ErrNotFound", err)
	}
}

func TestS3StoreGetMissingReturnsNotFound(t *testing.T) {
	t.Parallel()
	store := newS3Store(t)
	key := blobstore.WorkspaceKey(ids.New[ids.WorkspaceKind](), "attachment", "absent")

	_, _, err := store.Get(t.Context(), key)
	if !errors.Is(err, blobstore.ErrNotFound) {
		t.Fatalf("Get on a missing key: err = %v, want ErrNotFound", err)
	}
}

func TestS3StoreDeleteIsIdempotent(t *testing.T) {
	t.Parallel()
	store := newS3Store(t)
	key := blobstore.WorkspaceKey(ids.New[ids.WorkspaceKind](), "attachment", "never-written")

	// Removing an object that was never written is a no-op, so a crash-retry
	// of an erasure is safe.
	if err := store.Delete(t.Context(), key); err != nil {
		t.Fatalf("Delete on a missing key: %v", err)
	}
}

func TestS3StoreHealthReadyAgainstLiveMinIO(t *testing.T) {
	t.Parallel()
	if err := newS3Store(t).Health(t.Context()); err != nil {
		t.Fatalf("Health against live MinIO: %v", err)
	}
}

func TestS3StoreDeletePrefixRemovesOnlyThatPrefix(t *testing.T) {
	t.Parallel()
	store := newS3Store(t)
	ctx := t.Context()
	// Two fresh workspace ids give two disjoint prefixes in the shared bucket,
	// so deleting one can never collide with another test's objects.
	wsA := ids.New[ids.WorkspaceKind]()
	wsB := ids.New[ids.WorkspaceKind]()
	keyA := blobstore.WorkspaceKey(wsA, "attachment", "a1")
	keyB := blobstore.WorkspaceKey(wsB, "attachment", "b1")

	if err := store.Put(ctx, keyA, bytes.NewReader([]byte("mine")), 4, ""); err != nil {
		t.Fatalf("Put A: %v", err)
	}
	if err := store.Put(ctx, keyB, bytes.NewReader([]byte("theirs")), 6, ""); err != nil {
		t.Fatalf("Put B: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Delete(context.Background(), keyB); err != nil {
			t.Errorf("cleanup Delete: %v", err)
		}
	})

	deleted, err := store.DeletePrefix(ctx, wsA.String()+"/")
	if err != nil {
		t.Fatalf("DeletePrefix: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}
	if _, _, err := store.Get(ctx, keyA); !errors.Is(err, blobstore.ErrNotFound) {
		t.Errorf("the prefixed object survived: %v", err)
	}
	if _, _, err := store.Get(ctx, keyB); err != nil {
		t.Errorf("an object outside the prefix was deleted: %v", err)
	}
}

// TestS3StoreDeletePrefixLeavesNoVersionOnAVersionedBucket proves the erasure
// reaches every version and delete marker. It owns a throwaway bucket because
// enabling versioning on the shared test bucket would change what every other
// test in the lane leaves behind.
func TestS3StoreDeletePrefixLeavesNoVersionOnAVersionedBucket(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	endpoint := os.Getenv("MARGINCE_TEST_BLOBSTORE_ENDPOINT")
	if endpoint == "" {
		t.Fatal("MARGINCE_TEST_BLOBSTORE_ENDPOINT not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	cfg := blobstore.Config{
		Endpoint:  endpoint,
		AccessKey: os.Getenv("MARGINCE_TEST_BLOBSTORE_ACCESS_KEY"),
		SecretKey: os.Getenv("MARGINCE_TEST_BLOBSTORE_SECRET_KEY"),
		Bucket:    "versioned-" + ids.New[ids.WorkspaceKind]().String(),
		Region:    testRegion,
	}
	store, err := blobstore.New(ctx, cfg)
	if err != nil {
		t.Fatalf("blobstore.New: %v", err)
	}
	raw, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Region: cfg.Region,
	})
	if err != nil {
		t.Fatalf("minio.New: %v", err)
	}
	t.Cleanup(func() {
		if err := raw.RemoveBucket(context.Background(), cfg.Bucket); err != nil {
			t.Errorf("cleanup RemoveBucket: %v", err)
		}
	})
	if err := raw.EnableVersioning(ctx, cfg.Bucket); err != nil {
		t.Fatalf("EnableVersioning: %v", err)
	}

	// Two versions per page, against five versions and a marker below: an
	// implementation that deletes only the first page leaves the rest.
	blobstore.WithListPageSize(store, 2)
	ws := ids.New[ids.WorkspaceKind]()
	overwritten := blobstore.WorkspaceKey(ws, "attachment", "overwritten")
	deleted := blobstore.WorkspaceKey(ws, "attachment", "deleted")
	for _, put := range []struct{ key, body string }{{overwritten, "v1"}, {overwritten, "v2"}, {overwritten, "v3"}, {overwritten, "v4"}, {deleted, "gone"}} {
		if err := store.Put(ctx, put.key, bytes.NewReader([]byte(put.body)), int64(len(put.body)), ""); err != nil {
			t.Fatalf("Put %s: %v", put.key, err)
		}
	}
	if err := store.Delete(ctx, deleted); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, err := store.DeletePrefix(ctx, ws.String()+"/")
	if err != nil {
		t.Fatalf("DeletePrefix: %v", err)
	}
	if got != 2 {
		t.Errorf("deleted = %d, want 2 distinct keys", got)
	}
	for obj := range raw.ListObjects(ctx, cfg.Bucket, minio.ListObjectsOptions{Prefix: ws.String() + "/", Recursive: true, WithVersions: true}) {
		if obj.Err != nil {
			t.Fatalf("listing versions: %v", obj.Err)
		}
		t.Errorf("version %q of %q (delete marker: %v) is still stored", obj.VersionID, obj.Key, obj.IsDeleteMarker)
	}
}

func TestS3StoreDeletePrefixOnAnEmptyPrefixReportsZero(t *testing.T) {
	t.Parallel()
	store := newS3Store(t)
	// A workspace that never had a key written has an empty prefix — the
	// sweep must report zero, not error, on a data reset that finds nothing
	// left to purge.
	empty := ids.New[ids.WorkspaceKind]()

	deleted, err := store.DeletePrefix(t.Context(), empty.String()+"/")
	if err != nil {
		t.Fatalf("DeletePrefix: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0", deleted)
	}
}

func TestS3StoreDeletePrefixRejectsEmptyPrefix(t *testing.T) {
	t.Parallel()
	store := newS3Store(t)
	ctx := t.Context()
	key := blobstore.WorkspaceKey(ids.New[ids.WorkspaceKind](), "attachment", "guarded")

	if err := store.Put(ctx, key, bytes.NewReader([]byte("mine")), 4, ""); err != nil {
		t.Fatalf("Put: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Delete(context.Background(), key); err != nil {
			t.Errorf("cleanup Delete: %v", err)
		}
	})

	deleted, err := store.DeletePrefix(ctx, "")
	if !errors.Is(err, blobstore.ErrInvalidPrefix) {
		t.Fatalf("DeletePrefix(\"\"): err = %v, want ErrInvalidPrefix", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0", deleted)
	}
	if _, _, err := store.Get(ctx, key); err != nil {
		t.Errorf("an object survived an empty prefix but Get failed: %v", err)
	}
}

func TestS3StoreWorkspaceKeyIsolation(t *testing.T) {
	t.Parallel()
	store := newS3Store(t)
	ctx := t.Context()
	// The same logical entity id under two workspaces must not collide:
	// putting under A's key leaves B's key empty.
	wsA := ids.New[ids.WorkspaceKind]()
	wsB := ids.New[ids.WorkspaceKind]()
	keyA := blobstore.WorkspaceKey(wsA, "attachment", "shared-id")
	keyB := blobstore.WorkspaceKey(wsB, "attachment", "shared-id")

	if err := store.Put(ctx, keyA, bytes.NewReader([]byte("A's bytes")), 9, ""); err != nil {
		t.Fatalf("Put A: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Delete(context.Background(), keyA); err != nil {
			t.Errorf("cleanup Delete: %v", err)
		}
	})

	if _, _, err := store.Get(ctx, keyB); !errors.Is(err, blobstore.ErrNotFound) {
		t.Fatalf("workspace B key resolved after only A was written: err = %v, want ErrNotFound", err)
	}
}
