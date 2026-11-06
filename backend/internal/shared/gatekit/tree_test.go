// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// visits records a walk as one line per callback, with what the callback saw.
func visits(t *testing.T, walk func(string, fs.WalkDirFunc) error, root string, skip func(string, fs.DirEntry) error) []string {
	t.Helper()
	var seen []string
	err := walk(root, func(path string, entry fs.DirEntry, err error) error {
		line := filepath.ToSlash(path)
		if entry != nil {
			line += " " + entry.Type().String()
		}
		if err != nil {
			line += " error"
		}
		seen = append(seen, line)
		if err != nil || entry == nil {
			return err
		}
		return skip(path, entry)
	})
	if err != nil {
		seen = append(seen, "returned "+err.Error())
	}
	return seen
}

func plantTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"a/deep", "b", "c"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{"a/one.go", "a/deep/two.go", "b/three.go", "b/four.go", "c/five.go", "top.go"} {
		if err := os.WriteFile(filepath.Join(root, file), []byte(file), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("a", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestWalkDirVisitsWhatFilepathWalkDirVisits(t *testing.T) {
	t.Parallel()
	root := plantTree(t)
	stop := errors.New("stop")
	cases := map[string]func(string, fs.DirEntry) error{
		"every entry": func(string, fs.DirEntry) error { return nil },
		"a skipped directory": func(path string, _ fs.DirEntry) error {
			if filepath.Base(path) == "a" {
				return fs.SkipDir
			}
			return nil
		},
		"a skipped file skips its siblings": func(path string, _ fs.DirEntry) error {
			if filepath.Base(path) == "four.go" {
				return fs.SkipDir
			}
			return nil
		},
		"skip all": func(path string, _ fs.DirEntry) error {
			if filepath.Base(path) == "b" {
				return fs.SkipAll
			}
			return nil
		},
		"an error ends the walk": func(path string, _ fs.DirEntry) error {
			if filepath.Base(path) == "two.go" {
				return stop
			}
			return nil
		},
	}
	for name, skip := range cases {
		for _, from := range []string{root, filepath.Join(root, "top.go"), filepath.Join(root, "absent")} {
			want := visits(t, filepath.WalkDir, from, skip)
			got := visits(t, WalkDir, from, skip)
			if !slices.Equal(got, want) {
				t.Errorf("%s from %s: gatekit.WalkDir visited\n%s\nwhere filepath.WalkDir visited\n%s",
					name, from, strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
		}
	}
}

func TestWalkDirOverAHeldTreeVisitsWhatTheDiskHolds(t *testing.T) {
	t.Parallel()
	every := func(string, fs.DirEntry) error { return nil }
	want := visits(t, filepath.WalkDir, ".", every)
	for range 2 {
		if got := visits(t, WalkDir, ".", every); !slices.Equal(got, want) {
			t.Fatalf("a walk of this package's directory visited\n%s\nwhere the disk holds\n%s",
				strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}
}

func TestATreeFileIsReadOnceAndShared(t *testing.T) {
	t.Parallel()
	first, err := ReadFile("tree.go")
	if err != nil {
		t.Fatal(err)
	}
	second, err := ReadFile("tree.go")
	if err != nil {
		t.Fatal(err)
	}
	disk, err := os.ReadFile("tree.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, disk) {
		t.Fatal("the shared bytes of tree.go differ from the file on disk")
	}
	if &first[0] != &second[0] {
		t.Error("the second read of an unchanged file went to the disk again instead of sharing the first")
	}
	if cap(first) != len(first) {
		t.Errorf("the shared bytes have room past their end (cap %d, len %d), so an append writes into them",
			cap(first), len(first))
	}
}

func TestAFixtureRewrittenWithinOneClockTickIsReadFresh(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "fixture.txt")
	stamp := time.Unix(1_700_000_000, 0)
	for _, body := range []string{"before", "after!"} {
		writeSource(t, path, body, stamp)
		got, err := ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != body {
			t.Fatalf("a fixture rewritten with the same size and time read %q, want %q: a file under the "+
				"temp directory was served from the cache", got, body)
		}
	}
}

func TestADirectoryListingIsTheCallersToChange(t *testing.T) {
	t.Parallel()
	first, err := ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	want := first[0].Name()
	first[0] = first[len(first)-1]
	second, err := ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	if got := second[0].Name(); got != want {
		t.Errorf("a caller's change to its listing reached the next caller: first entry %q, want %q", got, want)
	}
}

func TestAWriteIntoSharedBytesIsReported(t *testing.T) {
	t.Parallel()
	shared, err := ReadFile("tree_test.go")
	if err != nil {
		t.Fatal(err)
	}
	original := shared[0]
	shared[0] ^= 0x20
	err = CheckSharedBytes()
	shared[0] = original
	if err == nil || !strings.Contains(err.Error(), "tree_test.go") {
		t.Errorf("a write into the shared bytes of tree_test.go was reported as %v, want an error naming the file", err)
	}
	if err := CheckSharedBytes(); err != nil {
		t.Errorf("with the bytes restored, the check still reports %v", err)
	}
}

func TestAFixedTreeServesAHeldEntryWithoutAskingTheDisk(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "held.txt")
	writeSource(t, path, "first", time.Unix(1_700_000_000, 0))
	versions := holder[string, string]{entries: map[string]*held[string]{}}
	first, err := versions.at(path, path, false)
	if err != nil {
		t.Fatal(err)
	}

	writeSource(t, path, "second, longer", time.Unix(1_700_000_001, 0))
	if fixed, err := versions.at(path, path, true); err != nil || fixed != first {
		t.Errorf("a fixed tree answered %p, %v for a held path, want the held entry %p", fixed, err, first)
	}
	if fresh, err := versions.at(path, path, false); err != nil || fresh == first {
		t.Errorf("an unfixed tree served the held entry for a rewritten file (%v)", err)
	}
}
