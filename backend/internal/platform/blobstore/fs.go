// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package blobstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// ErrInvalidKey reports a key that cannot become a path under the store's root:
// empty, absolute, or containing a ".." element. Callers errors.Is against it.
//
// It exists because keys are opaque to the store (WorkspaceKey only builds
// them) and this provider turns them into paths. Tenant isolation is a property
// of the key prefix, so a key that walks out of the root walks straight through
// that boundary — "ws-a/../ws-b/attachment/x" would otherwise read a sibling
// tenant's bytes with ws-a's own prefix. Refusing is the only safe answer: a
// sanitising rewrite would silently serve a DIFFERENT object than the row named.
var ErrInvalidKey = errors.New("blobstore: key must be relative, non-empty and free of '..'")

// The two trees under the root. Bytes and their content type live in parallel
// paths rather than in one file, so Get can stream the object without parsing a
// header and Size is the file size the filesystem already knows.
//
// A sidecar beside the object (say "<key>.type") would be simpler by one
// directory and wrong in a way that only shows up later: keys are opaque, so
// nothing stops an object whose key IS "<other key>.type", and DeletePrefix
// would have to count files it must not count.
const (
	fsBlobDir = "blob"
	fsMetaDir = "meta"
)

// Directory and file permissions. The store holds tenant attachment bytes on a
// single machine — a desktop installation's own folder — so it is owner-only:
// this is the provider whose objects are readable with a file browser.
const (
	fsDirPerm  os.FileMode = 0o700
	fsFilePerm os.FileMode = 0o600
)

// fsStore is the filesystem Store: object bytes in a directory tree under one
// root, for an installation that has local disk and no object storage service.
//
// It exists because the desktop bundle ships no S3 server and cannot
// reasonably bundle one (MinIO is AGPLv3, awkward inside a BUSL-1.1 product —
// see explanation/desktop-distribution.md), so its attachment and logo paths
// answered 501 with no configuration a user could supply that did not involve
// running a separate service. Speaking S3 to a server on localhost so that
// server can write to local disk is a hop this seam does not need: Store is
// four bytes-level methods, and a directory satisfies them.
//
// It is deliberately not a distributed store: no replication, no versioning, no
// signed URLs. An installation that outgrows one machine configures the S3
// provider, which is why both live behind the same seam.
type fsStore struct {
	root string
}

// NewFilesystem returns a Store that keeps object bytes under root, creating
// the tree if it is absent. root must be non-empty; it is resolved to an
// absolute path so a later working-directory change cannot move the store.
//
//nolint:ireturn // the seam has three providers (memory + filesystem + s3) behind one Store; returning the interface is the design.
func NewFilesystem(root string) (Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("blobstore: filesystem root is required — set %s", EnvPath)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("blobstore: resolve %q: %w", root, err)
	}
	for _, dir := range []string{fsBlobDir, fsMetaDir} {
		if err := os.MkdirAll(filepath.Join(abs, dir), fsDirPerm); err != nil {
			return nil, fmt.Errorf("blobstore: create %s: %w", filepath.Join(abs, dir), err)
		}
	}
	return &fsStore{root: abs}, nil
}

// path turns a key into a path under one of the store's trees, refusing any key
// that would not stay inside it.
//
// Three refusals, and the third is the one that is easy to miss. A key is always
// "/"-separated (WorkspaceKey builds it), so a BACKSLASH in one is never
// legitimate — and on Windows it is a separator, which means `ws-a\..\ws-b\x`
// passes a "/"-only element scan untouched and then Join+Clean walks it out of
// the tree. Rejecting the character outright rather than only on Windows keeps
// the refusal identical on both platforms: a key this store accepts on one host
// is a key it accepts on the other.
//
// filepath.IsLocal is the last word — it answers "does this stay inside the
// directory it is joined to" for the host's own rules, including a volume-
// qualified path ("C:x") and a reserved device name that no explicit list here
// would keep up with.
func (f *fsStore) path(tree, key string) (string, error) {
	if key == "" || strings.HasPrefix(key, "/") || strings.ContainsRune(key, '\\') {
		return "", fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	for _, element := range strings.Split(key, "/") {
		if element == ".." {
			return "", fmt.Errorf("%w: %q", ErrInvalidKey, key)
		}
	}
	local := filepath.FromSlash(key)
	if !filepath.IsLocal(local) {
		return "", fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	return filepath.Join(f.root, tree, local), nil
}

// Put publishes the object: metadata first, then the bytes.
//
// That order is the whole of what makes the pair consistent. The BLOB is the
// existence record — Get reports ErrNotFound from it and nothing else — so
// publishing it last means an object is never visible without the content type
// that describes it. The reverse order has a window in which a reader sees the
// new bytes and no metadata, and the object it gets back claims a content type
// the bytes do not have.
//
// Each write is itself atomic (temporary file in the same directory, then
// rename), so a reader sees a whole generation or the previous one, never a
// truncated file. If the bytes fail after the metadata landed, the metadata is
// removed again: a failed Put leaves nothing new behind.
//
// What this does NOT give is one atomic publish of both. Overwriting a key that
// already has an object would briefly pair the new content type with the old
// bytes. It is left as it is deliberately: no writer in this product overwrites
// a key — every call site mints a fresh uuidv7 into it (activities/capturedfiles,
// compose/sitelogo, compose/csvimport) — and the alternative, one file carrying a
// header, costs the property that makes a local store worth having, that the
// object on disk IS the file, recoverable with a file browser and nothing else.
func (f *fsStore) Put(_ context.Context, key string, r io.Reader, _ int64, contentType string) error {
	blob, err := f.path(fsBlobDir, key)
	if err != nil {
		return err
	}
	meta, err := f.path(fsMetaDir, key)
	if err != nil {
		return err
	}
	// An empty content type stores no metadata rather than an empty file: Get
	// reports "" for a missing one, so the two spellings would mean the same
	// thing and only one of them costs an inode. A key being overwritten from a
	// typed object to an untyped one has its old metadata removed FIRST, for the
	// same reason the write order is what it is.
	if contentType == "" {
		if rmErr := os.Remove(meta); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			return fmt.Errorf("blobstore: clear content type for %q: %w", key, rmErr)
		}
	} else if err := writeFileAtomic(meta, strings.NewReader(contentType)); err != nil {
		return err
	}
	if err := writeFileAtomic(blob, r); err != nil {
		if contentType == "" {
			return err
		}
		return errors.Join(err, removeStaging(meta))
	}
	return nil
}

func (f *fsStore) Get(_ context.Context, key string) (io.ReadCloser, Object, error) {
	blob, err := f.path(fsBlobDir, key)
	if err != nil {
		return nil, Object{}, err
	}
	file, err := os.Open(blob) //nolint:gosec // the path is bounded to the store root by f.path
	if err != nil {
		if isAbsent(err) {
			return nil, Object{}, ErrNotFound
		}
		return nil, Object{}, fmt.Errorf("blobstore: open %q: %w", key, err)
	}
	info, err := file.Stat()
	if err != nil {
		return nil, Object{}, errors.Join(fmt.Errorf("blobstore: stat %q: %w", key, err), file.Close())
	}
	contentType, err := f.contentType(key)
	if err != nil {
		return nil, Object{}, errors.Join(err, file.Close())
	}
	return file, Object{Key: key, Size: info.Size(), ContentType: contentType}, nil
}

// contentType reads the metadata written beside the object. A missing one is
// not an error: an object stored with no content type has none, and one written
// by an older build predates the metadata tree.
func (f *fsStore) contentType(key string) (string, error) {
	meta, err := f.path(fsMetaDir, key)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(meta) //nolint:gosec // the path is bounded to the store root by f.path
	if err != nil {
		if isAbsent(err) {
			return "", nil
		}
		return "", fmt.Errorf("blobstore: read content type for %q: %w", key, err)
	}
	return string(data), nil
}

// isAbsent reports whether an error means "there is nothing at this path".
//
// ErrNotExist is the ordinary answer. ENOTDIR is the other one: a key whose
// PARENT is an object rather than a directory ("ws/a/b" where "ws/a" holds
// bytes) cannot have an object either, and the filesystem says so with a
// different errno. Both are absence, and a caller must read them the same way —
// otherwise a missing object answers 500 instead of 404 on the one shape of key
// that is easiest to construct by accident.
func isAbsent(err error) bool {
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)
}

// Delete removes the object and its metadata. Idempotent, as the seam requires:
// a key with no object is not an error, so a crash-retry is safe.
func (f *fsStore) Delete(_ context.Context, key string) error {
	for _, tree := range []string{fsBlobDir, fsMetaDir} {
		path, err := f.path(tree, key)
		if err != nil {
			return err
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("blobstore: delete %q: %w", key, err)
		}
	}
	return nil
}

// DeletePrefix removes every object under prefix and reports how many. Only the
// blob tree is counted, so the number is objects deleted and not files removed.
//
// The prefix rule is the seam's, enforced before anything is touched: a prefix
// must be non-empty and end at "/", because an empty one addresses the whole
// store and a truncated one ("ws-a" for "ws-a/") reaches into a sibling tenant
// whose id extends it.
func (f *fsStore) DeletePrefix(_ context.Context, prefix string) (int, error) {
	if prefix == "" || !strings.HasSuffix(prefix, "/") {
		return 0, ErrInvalidPrefix
	}
	blob, err := f.path(fsBlobDir, prefix)
	if err != nil {
		return 0, err
	}
	deleted, err := countFiles(blob)
	if err != nil {
		return 0, err
	}
	for _, tree := range []string{fsBlobDir, fsMetaDir} {
		path, pathErr := f.path(tree, prefix)
		if pathErr != nil {
			return 0, pathErr
		}
		if rmErr := os.RemoveAll(path); rmErr != nil {
			return 0, fmt.Errorf("blobstore: delete prefix %q: %w", prefix, rmErr)
		}
	}
	return deleted, nil
}

// Health reports whether the store's trees are still directories this process
// can write to.
//
// BOTH trees, because a Put needs both: with meta unwritable, readiness would
// pass while every typed upload failed — after the metadata write, so the blob
// would not even be attempted. It writes and removes a probe file rather than
// only stat-ing, because a tree that has become read-only — an unmounted volume,
// a permission change — stats perfectly and fails every write.
func (f *fsStore) Health(_ context.Context) error {
	for _, tree := range []string{fsMetaDir, fsBlobDir} {
		if err := probeWritable(filepath.Join(f.root, tree)); err != nil {
			return err
		}
	}
	return nil
}

// probeWritable is the per-tree half of Health.
func probeWritable(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("blobstore: reach %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("blobstore: %s is not a directory", dir)
	}
	probe, err := os.CreateTemp(dir, ".health-*")
	if err != nil {
		return fmt.Errorf("blobstore: write to %s: %w", dir, err)
	}
	name := probe.Name()
	if err := probe.Close(); err != nil {
		return errors.Join(fmt.Errorf("blobstore: write to %s: %w", dir, err), removeStaging(name))
	}
	if err := os.Remove(name); err != nil {
		return fmt.Errorf("blobstore: clean up %s: %w", name, err)
	}
	return nil
}

// countFiles counts the regular files under dir, treating an absent dir as
// empty — DeletePrefix on a prefix with no objects reports zero, not an error.
func countFiles(dir string) (int, error) {
	count := 0
	err := filepath.WalkDir(dir, func(_ string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			count++
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("blobstore: walk %s: %w", dir, err)
	}
	return count, nil
}

// writeFileAtomic writes r to path through a temporary file in the same
// directory and renames it into place. Rename within one directory is atomic,
// which is what keeps a half-written object from ever being readable.
//
// Every failure path removes the staging file and reports what removing it
// cost, if anything: a returned error must not also leave litter for the next
// operator to wonder about, and a cleanup that itself failed is part of what
// went wrong rather than something to hide.
func writeFileAtomic(path string, r io.Reader) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, fsDirPerm); err != nil {
		return fmt.Errorf("blobstore: create %s: %w", dir, err)
	}
	staging, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("blobstore: stage a write in %s: %w", dir, err)
	}
	name := staging.Name()
	if err := fillStaging(staging, r); err != nil {
		return errors.Join(err, removeStaging(name))
	}
	if err := os.Rename(name, path); err != nil {
		return errors.Join(fmt.Errorf("blobstore: rename into %s: %w", path, err), removeStaging(name))
	}
	return nil
}

// fillStaging writes r into the staging file, then makes it durable and closes
// it. The close lives here so that every way this can fail has exactly one
// owner for it, which is what keeps the caller free of best-effort closes.
//
// Sync before the caller's rename: the rename is atomic with respect to
// readers, not with respect to power loss, and an attachment a row already
// points at is worth the fsync.
func fillStaging(staging *os.File, r io.Reader) error {
	name := staging.Name()
	if _, err := io.Copy(staging, r); err != nil {
		return errors.Join(fmt.Errorf("blobstore: write %s: %w", name, err), staging.Close())
	}
	if err := staging.Chmod(fsFilePerm); err != nil {
		return errors.Join(fmt.Errorf("blobstore: chmod %s: %w", name, err), staging.Close())
	}
	if err := staging.Sync(); err != nil {
		return errors.Join(fmt.Errorf("blobstore: sync %s: %w", name, err), staging.Close())
	}
	if err := staging.Close(); err != nil {
		return fmt.Errorf("blobstore: close %s: %w", name, err)
	}
	return nil
}

// removeStaging deletes a staging file that will not be renamed into place. An
// absent one is success: the rename may have moved it already.
func removeStaging(name string) error {
	if err := os.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("blobstore: remove the staging file %s: %w", name, err)
	}
	return nil
}
