// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

import (
	"go/parser"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeSource(t *testing.T, path, body string, modified time.Time) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
}

func TestAnUnchangedFileIsParsedOnceAndSharedByEveryCaller(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shared.go")
	writeSource(t, path, "package shared\n", time.Unix(1_700_000_000, 0))

	first, err := ParseFile(path, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ParseFile(path, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Error("the second read of an unchanged file parsed it again instead of sharing the first tree")
	}
	if got := SourceFileSet().Position(first.Package).Filename; got != path {
		t.Errorf("a position resolved against SourceFileSet names %q, want the path as spelled, %q", got, path)
	}
}

func TestARewrittenFileIsParsedAgainRatherThanServedStale(t *testing.T) {
	path := filepath.Join(t.TempDir(), "planted.go")
	writeSource(t, path, "package before\n", time.Unix(1_700_000_000, 0))
	if _, err := ParseFile(path, 0); err != nil {
		t.Fatal(err)
	}

	writeSource(t, path, "package after\n", time.Unix(1_700_000_001, 0))
	file, err := ParseFile(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if file.Name.Name != "after" {
		t.Errorf("a planted rewrite read as package %q: the cache served the tree from before the rewrite", file.Name.Name)
	}
}

func TestARelativePathNamesTheFileUnderTheCurrentDirectory(t *testing.T) {
	stamp := time.Unix(1_700_000_000, 0)
	first, second := t.TempDir(), t.TempDir()
	writeSource(t, filepath.Join(first, "same.go"), "package first\n", stamp)
	writeSource(t, filepath.Join(second, "same.go"), "package other\n", stamp)

	t.Chdir(first)
	if _, err := ParseFile("same.go", 0); err != nil {
		t.Fatal(err)
	}
	t.Chdir(second)
	file, err := ParseFile("same.go", 0)
	if err != nil {
		t.Fatal(err)
	}
	if file.Name.Name != "other" {
		t.Errorf("same.go under another directory read as package %q: the cache served the file the "+
			"spelling named before the directory changed", file.Name.Name)
	}
}

func TestEachParseModeGetsItsOwnTree(t *testing.T) {
	path := filepath.Join(t.TempDir(), "commented.go")
	writeSource(t, path, "// Package commented says so.\npackage commented\n", time.Unix(1_700_000_000, 0))

	bare, err := ParseFile(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	commented, err := ParseFile(path, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	if bare.Doc != nil {
		t.Error("a caller that did not ask for comments was handed a tree carrying them")
	}
	if commented.Doc == nil {
		t.Error("a caller that asked for comments was handed a tree without them")
	}

	unresolved, err := ParseFile(path, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	if unresolved != bare {
		t.Error("skipping object resolution parsed a second copy instead of sharing the resolved tree")
	}
}

func TestAMissingFileIsAnErrorNotAnEmptyTree(t *testing.T) {
	if file, err := ParseFile(filepath.Join(t.TempDir(), "absent.go"), 0); err == nil {
		t.Errorf("parsing a file that does not exist returned %v and no error", file)
	}
}
