// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

import (
	"errors"
	"fmt"
	"hash/maphash"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
)

// The tree the gates read, held once per test binary. ReadFile, ReadDir and
// WalkDir answer what their os and filepath namesakes answer. So a gate keeps
// its own corpus and skip rules, and stops paying for the disk again.
//
// A path under the temp directory is never held. A fixture there can change
// within one tick of the file clock, and no stat tells two such versions apart.

// held is one version of one file, filled once by whichever caller asks first.
type held[T any] struct {
	info  os.FileInfo
	once  sync.Once
	value T
	err   error
	// filled lets a reader outside once see whether value is safe to read.
	filled atomic.Bool
}

// fill runs read once, and answers what the first run produced.
func (entry *held[T]) fill(read func() (T, error)) (T, error) {
	entry.once.Do(func() {
		entry.value, entry.err = read()
		entry.filled.Store(true)
	})
	return entry.value, entry.err
}

type holder[K comparable, T any] struct {
	mu      sync.Mutex
	entries map[K]*held[T]
}

// treeFixed is set by FixTree: a held entry is then served without a stat.
var treeFixed atomic.Bool

// FixTree declares that, for the rest of this binary's run, nothing rewrites
// the tree outside the temp directory and the working directory stays put.
func FixTree() { treeFixed.Store(true) }

// at answers the entry for path's current version. When fixed, a held entry is
// current; otherwise a fresh stat decides, and a changed file gets a new one.
func (h *holder[K, T]) at(key K, path string, fixed bool) (*held[T], error) {
	if fixed {
		h.mu.Lock()
		entry, ok := h.entries[key]
		h.mu.Unlock()
		if ok {
			return entry, nil
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	entry, ok := h.entries[key]
	if !ok || !sameVersion(entry.info, info) {
		entry = &held[T]{info: info}
		h.entries[key] = entry
	}
	return entry, nil
}

// sameVersion reports whether held and now describe one version of one file.
// The identity check covers a relative path meaning another file once the
// working directory moves; size and time cover a gate that rewrites a file.
func sameVersion(held, now os.FileInfo) bool {
	return os.SameFile(held, now) && held.Size() == now.Size() && held.ModTime().Equal(now.ModTime())
}

// fixtureDir is the temp directory, spelled as t.TempDir spells its children.
var fixtureDir = sync.OnceValue(func() string { return filepath.Clean(os.TempDir()) })

func holdable(path string) bool {
	return !Under(filepath.ToSlash(filepath.Clean(path)), filepath.ToSlash(fixtureDir()))
}

type heldBody struct {
	bytes []byte
	sum   uint64
}

var (
	files    = holder[string, heldBody]{entries: map[string]*held[heldBody]{}}
	dirs     = holder[string, []fs.DirEntry]{entries: map[string]*held[[]fs.DirEntry]{}}
	bodySeed = maphash.MakeSeed()
)

// ReadFile is os.ReadFile, read from the disk once per version of the file.
//
// The bytes are SHARED with every concurrent gate: read them, never modify
// them. Their capacity is their length, so an append copies rather than writes
// into a neighbour's view. CheckSharedBytes checks the promise after a run.
func ReadFile(path string) ([]byte, error) {
	if !holdable(path) {
		return os.ReadFile(path) // #nosec G304 -- a gate names the files of its own tree or fixture
	}
	entry, err := files.at(path, path, treeFixed.Load())
	if err != nil || !entry.info.Mode().IsRegular() {
		return os.ReadFile(path) // #nosec G304 -- the os error is the answer a caller already handles
	}
	body, err := entry.fill(func() (heldBody, error) {
		body, readErr := os.ReadFile(path) // #nosec G304 -- see above
		body = slices.Clip(body)
		return heldBody{bytes: body, sum: maphash.Bytes(bodySeed, body)}, readErr
	})
	return body.bytes, err
}

// ReadDir is os.ReadDir, listed from the disk once per version of the
// directory. The slice is the caller's own; the entries are shared and
// immutable.
func ReadDir(dir string) ([]fs.DirEntry, error) {
	if !holdable(dir) {
		return os.ReadDir(dir)
	}
	entry, err := dirs.at(dir, dir, treeFixed.Load())
	if err != nil || !entry.info.IsDir() {
		return os.ReadDir(dir)
	}
	entries, err := entry.fill(func() ([]fs.DirEntry, error) { return os.ReadDir(dir) })
	return slices.Clone(entries), err
}

// WalkDir is filepath.WalkDir over ReadDir. It visits the same entries in the
// same order, and SkipDir and SkipAll mean what they mean there.
func WalkDir(root string, visit fs.WalkDirFunc) error {
	info, err := os.Lstat(root)
	if err != nil {
		err = visit(root, nil, err)
	} else {
		err = walkEntry(root, fs.FileInfoToDirEntry(info), visit)
	}
	if errors.Is(err, fs.SkipDir) || errors.Is(err, fs.SkipAll) {
		return nil
	}
	return err
}

func walkEntry(path string, entry fs.DirEntry, visit fs.WalkDirFunc) error {
	if err := visit(path, entry, nil); err != nil || !entry.IsDir() {
		if errors.Is(err, fs.SkipDir) && entry.IsDir() {
			return nil
		}
		return err
	}
	children, err := ReadDir(path)
	if err != nil {
		// A second call reports the listing error, as filepath.WalkDir does.
		if err = visit(path, entry, err); err != nil {
			if errors.Is(err, fs.SkipDir) {
				return nil
			}
			return err
		}
	}
	for _, child := range children {
		if err := walkEntry(filepath.Join(path, child.Name()), child, visit); err != nil {
			if errors.Is(err, fs.SkipDir) {
				break
			}
			return err
		}
	}
	return nil
}

// CheckSharedBytes reports every held file whose bytes no longer hash to what
// was read: a gate wrote into bytes ReadFile shared with other gates.
func CheckSharedBytes() error {
	files.mu.Lock()
	var altered []string
	for path, entry := range files.entries {
		if entry.filled.Load() && maphash.Bytes(bodySeed, entry.value.bytes) != entry.value.sum {
			altered = append(altered, path)
		}
	}
	files.mu.Unlock()
	if len(altered) == 0 {
		return nil
	}
	slices.Sort(altered)
	return fmt.Errorf("a gate modified the bytes gatekit.ReadFile shares, so every gate that read them later "+
		"judged altered text: copy before writing (bytes.Clone) in the gate that reads %s",
		strings.Join(altered, ", "))
}
