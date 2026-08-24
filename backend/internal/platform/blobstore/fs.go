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
	// sync makes a rename durable. A field rather than a direct call because a
	// failure AFTER the rename is a state this store has to answer for — the
	// file is already published — and a test that cannot produce it is a test
	// that cannot hold the answer.
	sync func(string) error
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
	return &fsStore{root: abs, sync: syncDir}, nil
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
		// A leading dot covers "." and ".." and one more thing: it is what makes
		// this store's own scratch distinguishable from an object. countFiles
		// skips dotfiles so a `.tmp-*` staging file is never counted as an
		// object deleted, and that reasoning is only sound while no object can
		// be named like one.
		if strings.HasPrefix(element, ".") {
			return "", fmt.Errorf("%w: %q", ErrInvalidKey, key)
		}
	}
	local := filepath.FromSlash(key)
	if !filepath.IsLocal(local) {
		return "", fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	return filepath.Join(f.root, tree, local), nil
}

// Put publishes the object: metadata first, then the bytes, and puts the
// previous metadata back if the bytes fail.
//
// The order is what makes the pair consistent. The BLOB is the existence record
// — Get reports ErrNotFound from it and nothing else — so publishing it last
// means an object is never VISIBLE without the content type that describes it.
// The reverse order has a window in which a reader sees new bytes and no
// metadata, and the object it gets back claims a content type the bytes do not
// have.
//
// The order alone is not enough when the key already holds an object, which is
// what the restore is for: without it, a failed overwrite would leave the OLD
// bytes with the new metadata deleted, so a Put that changed nothing would have
// changed the stored object's content type to "". Reading the previous value
// first and writing it back costs one small read on a path that is already doing
// IO, and it means a failed Put leaves the previous generation exactly as it was
// — or, for a key that had nothing, nothing.
//
// Each write is itself atomic (temporary file in the same directory, fsync,
// rename, directory sync), so a reader sees a whole generation, never a
// truncated file.
//
// What this still does not give is ONE atomic publish of both. A reader that
// arrives between the two renames of an overwrite sees the old bytes with the
// new content type. Closing that needs a generation manifest or a single file
// carrying both, and the second costs the property that makes a local store
// worth having — that the object on disk IS the file, recoverable with a file
// browser and nothing else. It is left open deliberately, and narrowly: no
// writer in this product overwrites a key (each mints a fresh uuidv7 into it),
// so the window has no caller today.
func (f *fsStore) Put(_ context.Context, key string, r io.Reader, _ int64, contentType string) error {
	blob, err := f.path(fsBlobDir, key)
	if err != nil {
		return err
	}
	meta, err := f.path(fsMetaDir, key)
	if err != nil {
		return err
	}
	previous, hadPrevious, err := readMeta(meta)
	if err != nil {
		return fmt.Errorf("blobstore: read the stored content type for %q: %w", key, err)
	}

	// An empty content type stores no metadata rather than an empty file: Get
	// reports "" for a missing one, so the two spellings would mean the same
	// thing and only one of them costs an inode.
	if contentType == "" {
		if rmErr := removeStaging(meta); rmErr != nil {
			return fmt.Errorf("blobstore: clear the content type for %q: %w", key, rmErr)
		}
	} else if published, metaErr := writeFileAtomic(meta, strings.NewReader(contentType), f.sync); metaErr != nil {
		// Published-but-unsynced metadata is VISIBLE, and the bytes it claims to
		// describe are not written yet — so it is undone here rather than left
		// for a reader to find. Unpublished, there is nothing to undo.
		if published {
			return errors.Join(metaErr, f.restoreMeta(meta, previous, hadPrevious))
		}
		return metaErr
	}

	published, blobErr := writeFileAtomic(blob, r, f.sync)
	if blobErr == nil {
		return nil
	}
	// The bytes are visible: the metadata beside them describes THEM, and taking
	// it away would leave a readable object with no content type. The error still
	// goes back — a caller that heard "failed" and finds an object is better off
	// than one that heard nothing.
	if published {
		return blobErr
	}
	return errors.Join(blobErr, f.restoreMeta(meta, previous, hadPrevious))
}

// readMeta reads a key's stored content type, reporting whether one was there.
// Absence is not an error: an object stored without a content type has none.
func readMeta(meta string) (string, bool, error) {
	data, err := os.ReadFile(meta) //nolint:gosec // the path is bounded to the store root by f.path
	if err != nil {
		if isAbsent(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return string(data), true, nil
}

// restoreMeta puts a key's previous content type back after a failed write —
// or removes the one just published, when the key had none. It is the second
// half of Put's failure path: the metadata is published before the bytes, so a
// failure there has to unpublish it without disturbing what was already stored.
//
// A restore that is itself published-but-unsynced is still a restore: the
// previous value is what a reader sees, which is the outcome this is for.
func (f *fsStore) restoreMeta(meta, previous string, hadPrevious bool) error {
	if !hadPrevious {
		return removeStaging(meta)
	}
	if _, err := writeFileAtomic(meta, strings.NewReader(previous), f.sync); err != nil {
		return fmt.Errorf("blobstore: restore the previous content type at %s: %w", meta, err)
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
	value, _, err := readMeta(meta)
	if err != nil {
		return "", fmt.Errorf("blobstore: read content type for %q: %w", key, err)
	}
	return value, nil
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

// errorsJoin is errors.Join, re-exported for the per-platform syncDir files so
// each one imports only what it uses.
func errorsJoin(errs ...error) error { return errors.Join(errs...) }

// countFiles counts the OBJECTS under dir, treating an absent dir as empty —
// DeletePrefix on a prefix with no objects reports zero, not an error.
//
// Objects only: this store's own scratch — a `.tmp-*` staging file from a Put
// that is in flight or died mid-write, a `.health-*` probe — is not an object and
// must not be reported as one deleted. Telling them apart is not a guess, because
// no object can look like them: fsStore.path refuses a key whose element starts
// with a dot, so every dotfile in these trees is this package's own.
func countFiles(dir string) (int, error) {
	count := 0
	err := filepath.WalkDir(dir, func(_ string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() && !strings.HasPrefix(entry.Name(), ".") {
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
// directory and renames it into place, reporting whether the file was
// PUBLISHED.
//
// The bool is the whole point of the signature. Rename publishes; the directory
// sync that follows can still fail, and at that moment the file is visible to
// every reader while this function returns an error. A caller that reads
// "error" as "nothing happened" then undoes something that did happen — which
// is exactly how removing the metadata of an already-published blob would leave
// an object readable with no content type.
//
// Failures BEFORE the rename remove the staging file and report what removing it
// cost, if anything: a returned error must not also leave litter for the next
// operator to wonder about.
func writeFileAtomic(path string, r io.Reader, sync func(string) error) (published bool, err error) {
	dir := filepath.Dir(path)
	if mkErr := os.MkdirAll(dir, fsDirPerm); mkErr != nil {
		return false, fmt.Errorf("blobstore: create %s: %w", dir, mkErr)
	}
	staging, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return false, fmt.Errorf("blobstore: stage a write in %s: %w", dir, err)
	}
	name := staging.Name()
	if fillErr := fillStaging(staging, r); fillErr != nil {
		return false, errors.Join(fillErr, removeStaging(name))
	}
	if renErr := os.Rename(name, path); renErr != nil {
		return false, errors.Join(fmt.Errorf("blobstore: rename into %s: %w", path, renErr), removeStaging(name))
	}
	// Published from here on, whatever the sync says. The rename is a
	// directory-entry change and fillStaging's fsync covered only the staging
	// file's bytes, so without this a power loss can lose the rename while the
	// contents survive — no object under a key a row already points at. See
	// syncDir, which differs by platform.
	if syncErr := sync(dir); syncErr != nil {
		return true, syncErr
	}
	return true, nil
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
