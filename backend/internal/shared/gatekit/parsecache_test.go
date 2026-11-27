// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"slices"
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

func TestEveryWholeFileParseSharesOneTreeWithItsComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "commented.go")
	writeSource(t, path, "// Package commented says so.\npackage commented\n", time.Unix(1_700_000_000, 0))

	shared := map[string]*ast.File{}
	for name, mode := range map[string]parser.Mode{
		"no mode":            0,
		"comments":           parser.ParseComments,
		"no resolution":      parser.SkipObjectResolution,
		"every flag at once": parser.ParseComments | parser.SkipObjectResolution,
	} {
		file, err := ParseFile(path, mode)
		if err != nil {
			t.Fatal(err)
		}
		shared[name] = file
	}
	for name, file := range shared {
		if file != shared["no mode"] {
			t.Errorf("asking with %s parsed a second copy instead of sharing the one tree", name)
		}
	}
	if shared["no mode"].Doc == nil {
		t.Error("the shared tree dropped the comments a caller that asked for them needs")
	}

	bare, err := ParseFileWithoutComments(path)
	if err != nil {
		t.Fatal(err)
	}
	if bare.Doc != nil || len(bare.Comments) != 0 {
		t.Error("a caller that prints code was handed a tree carrying comments it would print")
	}
}

func TestAMissingFileIsAnErrorNotAnEmptyTree(t *testing.T) {
	if file, err := ParseFile(filepath.Join(t.TempDir(), "absent.go"), 0); err == nil {
		t.Errorf("parsing a file that does not exist returned %v and no error", file)
	}
}

func TestACollectedTreeIsParsedAgainOnTheSamePositions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "collected.go")
	writeSource(t, path, "package collected\n\nfunc First() {}\n\nfunc Second() {}\n", time.Unix(1_700_000_000, 0))

	before := declarationPositions(t, path)
	waitUntilCollected(t, path, parser.ParseComments)
	grown := sourceFiles.Base()
	after := declarationPositions(t, path)

	if !slices.Equal(before, after) {
		t.Errorf("a re-parse moved the declarations: %v, then %v", before, after)
	}
	if sourceFiles.Base() != grown {
		t.Errorf("a re-parse grew the shared FileSet from base %d to %d", grown, sourceFiles.Base())
	}
}

// declarationPositions answers each declaration's position as a number and as
// the file:line it resolves to, and keeps no reference to the tree.
func declarationPositions(t *testing.T, path string) []string {
	t.Helper()
	tree, err := ParseFile(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	var positions []string
	for _, decl := range tree.Decls {
		if fn, isFunc := decl.(*ast.FuncDecl); isFunc {
			positions = append(positions, fmt.Sprintf("%d=%s", fn.Pos(), sourceFiles.Position(fn.Pos())))
		}
	}
	if len(positions) != 2 {
		t.Fatalf("the fixture declares two functions, the tree holds %d", len(positions))
	}
	return positions
}

// waitUntilCollected runs the collector until the held tree of path is gone.
func waitUntilCollected(t *testing.T, path string, mode parser.Mode) {
	t.Helper()
	parses.mu.Lock()
	entry := parses.entries[parseKey{spelled: path, mode: mode}]
	parses.mu.Unlock()
	if entry == nil {
		t.Fatalf("%s was never held", path)
	}
	for range 10 {
		runtime.GC()
		entry.value.mu.Lock()
		collected := entry.value.tree.Value() == nil
		entry.value.mu.Unlock()
		if collected {
			return
		}
	}
	t.Fatalf("the tree of %s outlived every reference to it", path)
}

func TestAPathThatCannotBeReadIsAnErrorEachTimeItIsAsked(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "notafile.go")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if tree, err := ParseFile(dir, 0); err == nil || tree != nil {
			t.Fatalf("a directory parsed as a Go file: tree %v, error %v", tree, err)
		}
	}
}

func TestAFileThatDoesNotParseIsNeverHeld(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.go")
	writeSource(t, path, "package broken\n\nfunc {\n", time.Unix(1_700_000_000, 0))
	grown := 0
	for range 2 {
		if _, err := ParseFile(path, 0); err == nil {
			t.Fatal("a file with a syntax error parsed without one")
		}
		if grown == 0 {
			grown = sourceFiles.Base()
		}
	}
	if sourceFiles.Base() != grown {
		t.Errorf("parsing a broken file again grew the shared FileSet from base %d to %d", grown, sourceFiles.Base())
	}
	parses.mu.Lock()
	entry := parses.entries[parseKey{spelled: path, mode: parser.ParseComments}]
	parses.mu.Unlock()
	if entry.value.tree.Value() != nil {
		t.Error("a tree that failed to parse is held, so its error would not be reported again")
	}
}

const factsFixture = "package facts\n\nconst listQuery = `SELECT id FROM contact`\n\n" +
	"func update() string { return \"UPDATE contact \" + \"SET name = $1\" }\n"

// A shared tree's statements and literals are what a fresh walk reads. A
// change to one caller's answer leaves the next caller's as it was.
func TestASharedTreesFactsMatchAFreshWalkAndStayWithEachCaller(t *testing.T) {
	path := filepath.Join(t.TempDir(), "facts.go")
	writeSource(t, path, factsFixture, time.Unix(1_700_000_000, 0))
	tree, err := ParseFile(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, shared := factsOf(tree); !shared {
		t.Fatal("a tree ParseFile shares has no facts held for it")
	}
	private, err := parser.ParseFile(token.NewFileSet(), path, factsFixture, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, shared := factsOf(private); shared {
		t.Error("a tree its caller parsed is held as shared, so a change to it would be served stale")
	}
	fresh := sqlStatements(private, nil)
	first := SQLStatementsOf(tree)
	if !slices.Equal(first, fresh) || len(fresh) != 2 {
		t.Fatalf("the shared tree's statements are %q, a fresh walk reads %q", first, fresh)
	}
	first[0] = "DELETE FROM planted"
	if again := SQLStatementsOf(tree); !slices.Equal(again, fresh) {
		t.Errorf("a caller's change to its statements reached the next caller: %q", again)
	}
	contact := TableReadPattern("contact")
	if !FileReadsTable(path, tree, contact) || FileReadsTable(path, tree, TableReadPattern("planted")) {
		t.Error("the shared tree's literals answer a table read differently from its source")
	}
	if got, want := literalsOf(tree), stringLiterals(private); !slices.Equal(got, want) {
		t.Errorf("the shared tree's literals are %q, a fresh walk reads %q", got, want)
	}
}

// The facts go with their tree: holding them does not keep the tree alive.
func TestASharedTreesFactsLetTheTreeBeCollected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "collected_facts.go")
	writeSource(t, path, factsFixture, time.Unix(1_700_000_000, 0))
	readFacts := func() int {
		tree, err := ParseFile(path, 0)
		if err != nil {
			t.Fatal(err)
		}
		return len(SQLStatementsOf(tree)) + len(literalsOf(tree))
	}
	if readFacts() == 0 {
		t.Fatal("the fixture's facts are empty, so nothing was held")
	}
	waitUntilCollected(t, path, parser.ParseComments)
}
