// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// DeclareInputs makes Go's test cache key a result on files it would not
// otherwise recheck, so a replayed pass cannot outlive a change to them. Two
// kinds exist, and each path is declared the way its kind needs:
//
//   - inside the test's module, files a child process read (`git ls-files`,
//     `go list`): the cache records only what the test itself opens, so a
//     directory is walked — its listing and every entry's stat — and a file is
//     stated, a missing one too, since creating it must invalidate as surely as
//     editing it;
//   - outside the test's module, anything at all: the cache never rechecks such
//     a path, so lookup — os.Getenv, passed by the test, since configuration is
//     read at a root — reads the variable CI sets to a digest of the path's
//     top-level tree (TreeDigestVar), and the cache keys on its value.
func DeclareInputs(lookup func(string) string, paths ...string) error {
	for _, path := range paths {
		declared, err := declareOutsideModule(lookup, path)
		if err != nil {
			return err
		}
		if declared {
			continue
		}
		info, err := os.Stat(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("declaring %s as a test input: %w", path, err)
		}
		if !info.IsDir() {
			continue
		}
		if err := filepath.WalkDir(path, func(_ string, _ fs.DirEntry, err error) error { return err }); err != nil {
			return fmt.Errorf("declaring the tree under %s as a test input: %w", path, err)
		}
	}
	return nil
}

// DeclareListings is DeclareInputs for directories whose own entries are the
// input and whose subdirectories are not — a package directory, whose export
// data depends on its files and on no package nested beneath it.
func DeclareListings(lookup func(string) string, dirs ...string) error {
	for _, dir := range dirs {
		declared, err := declareOutsideModule(lookup, dir)
		if err != nil {
			return err
		}
		if declared {
			continue
		}
		if _, err := os.ReadDir(dir); err != nil {
			return fmt.Errorf("declaring the listing of %s as a test input: %w", dir, err)
		}
	}
	return nil
}

// TreeDigestVar names the environment variable scripts/ci-stable-mtimes.sh
// sets to a digest of one top-level entry of the repository: TREE_DIGEST_
// and the entry's name, upper-cased, every other byte an underscore.
func TreeDigestVar(top string) string {
	var name strings.Builder
	name.WriteString("TREE_DIGEST_")
	for _, b := range []byte(strings.ToUpper(top)) {
		if (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') {
			name.WriteByte(b)
		} else {
			name.WriteByte('_')
		}
	}
	return name.String()
}

// declareOutsideModule reads the digest variable for path when it lies in the
// repository but outside the test's module, and reports whether it did.
func declareOutsideModule(lookup func(string) string, path string) (bool, error) {
	module, _, found := moduleRootAndPath()
	repo := repositoryRoot()
	if !found || repo == "" {
		return false, nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false, fmt.Errorf("resolving %s as a test input: %w", path, err)
	}
	slashed, moduleDir, repoDir := filepath.ToSlash(abs), filepath.ToSlash(module), filepath.ToSlash(repo)
	if Under(slashed, moduleDir) || !Under(slashed, repoDir) || slashed == repoDir {
		return false, nil
	}
	top, _, _ := strings.Cut(strings.TrimPrefix(slashed, repoDir+"/"), "/")
	// The read is the declaration: the test log records the variable and its value.
	lookup(TreeDigestVar(top))
	return true, nil
}

// repositoryRoot is the nearest directory above the module holding .git — a
// directory in a clone, a file in a worktree.
var repositoryRoot = sync.OnceValue(func() string {
	module, _, found := moduleRootAndPath()
	if !found {
		return ""
	}
	for dir := module; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		if filepath.Dir(dir) == dir {
			return ""
		}
	}
})
