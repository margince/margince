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
func (f *fsStore) path(tree, key string) (string, error) {
	if key == "" || strings.HasPrefix(key, "/") {
		return "", fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	for _, element := range strings.Split(key, "/") {
		if element == ".." {
			return "", fmt.Errorf("%w: %q", ErrInvalidKey, key)
		}
	}
	// FromSlash, not Join alone: keys are always "/"-separated, and on a
	// platform whose separator differs a key element must not be read as a path
	// element boundary by accident.
	return filepath.Join(f.root, tree, filepath.FromSlash(key)), nil
}

// Put writes the object atomically: bytes land in a temporary file beside the
// destination and are renamed into place, so a reader sees the whole previous
// object or the whole new one. A crash mid-upload leaves a temporary file, never
// a truncated attachment that the row still points at.
func (f *fsStore) Put(_ context.Context, key string, r io.Reader, _ int64, contentType string) error {
	blob, err := f.path(fsBlobDir, key)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(blob, r); err != nil {
		return err
	}
	meta, err := f.path(fsMetaDir, key)
	if err != nil {
		return err
	}
	// An empty content type writes no metadata rather than an empty file: Get
	// reports "" for a missing one, so the two spellings would mean the same
	// thing and only one of them costs an inode.
	if contentType == "" {
		if err := os.Remove(meta); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("blobstore: clear content type for %q: %w", key, err)
		}
		return nil
	}
	return writeFileAtomic(meta, strings.NewReader(contentType))
}

func (f *fsStore) Get(_ context.Context, key string) (io.ReadCloser, Object, error) {
	blob, err := f.path(fsBlobDir, key)
	if err != nil {
		return nil, Object{}, err
	}
	file, err := os.Open(blob) //nolint:gosec // the path is bounded to the store root by f.path
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
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
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("blobstore: read content type for %q: %w", key, err)
	}
	return string(data), nil
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

// Health reports whether the root is still a directory this process can write
// to. It writes and removes a probe file rather than only stat-ing, because a
// root that has become read-only — an unmounted volume, a permission change —
// stats perfectly and fails every upload.
func (f *fsStore) Health(_ context.Context) error {
	blob := filepath.Join(f.root, fsBlobDir)
	info, err := os.Stat(blob)
	if err != nil {
		return fmt.Errorf("blobstore: reach %s: %w", blob, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("blobstore: %s is not a directory", blob)
	}
	probe, err := os.CreateTemp(blob, ".health-*")
	if err != nil {
		return fmt.Errorf("blobstore: write to %s: %w", blob, err)
	}
	name := probe.Name()
	if err := probe.Close(); err != nil {
		return fmt.Errorf("blobstore: write to %s: %w", blob, err)
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
