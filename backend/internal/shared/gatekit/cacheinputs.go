// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// DeclareInputs makes Go's test cache key a result on files the test read
// through another process — `git ls-files`, `go list`, `git check-ignore` —
// whose reads the cache cannot see, so a replayed pass cannot outlive a change
// to them. The cache records what the test itself opens: a directory by its
// listing and every entry's size, mode and mtime, a file by its stat. So a
// directory is walked, and a file is stated — a missing one too, because
// creating it must invalidate the result as surely as editing it.
func DeclareInputs(paths ...string) error {
	for _, path := range paths {
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
