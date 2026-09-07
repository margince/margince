// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// A version guard that is not read is worse than none: the code says the write
// is protected, and it is not.
//
// These endpoints read optimistic concurrency from the `If-Match` HEADER only.
// An `if_version` in the JSON body is accepted and IGNORED — a PATCH carrying a
// version that cannot exist answers 200 and writes anyway — so a guard spelled
// that way is decorative, and the seeder's writes were last-write-wins while
// reading as though they were not.
//
// Nothing failed when they were: the seeder ran, the rows were written, and the
// symptom is an owner or a lifecycle stage that reverts for no visible reason
// on the run where two writers overlap. So the rule is held here rather than
// left to the next reviewer noticing the same thing a second time.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// bodyVersionKey is the spelling the wire accepts and the server ignores.
const bodyVersionKey = "if_version"

// guardedWriters are the client methods that put a version where the server
// reads it. A write needing a guard goes through one of these.
var guardedWriters = []string{"patchGuarded", "postGuarded"}

func TestNoSeederWriteSpellsItsVersionGuardInTheBody(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package directory: %v", err)
	}
	fset := token.NewFileSet()
	files := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		files++
		file, parseErr := parser.ParseFile(fset, name, nil, parser.ParseComments)
		if parseErr != nil {
			t.Fatalf("parsing %s: %v", name, parseErr)
		}
		for _, at := range bodyVersionKeys(file) {
			t.Errorf("%s:%d puts %q in a request body, where the server does not read it — the "+
				"write is unguarded and reads as though it is not. Pass the version to %s instead, "+
				"which sends the If-Match header",
				filepath.Join("backend/tools/seed-demo", name), fset.Position(at).Line,
				bodyVersionKey, strings.Join(guardedWriters, " or "))
		}
	}
	// A census that read no files certifies nothing, and this one walks a
	// directory: a package move would leave it green over a tree it never saw.
	if files == 0 {
		t.Fatal("no seeder source files were read — this census is judging nothing")
	}
}

// bodyVersionKeys reports every position where the version key appears as a
// STRING LITERAL, which is how it enters a request body.
//
// Literals rather than comments: this package explains the trap in prose beside
// the guarded writers, and a census that flagged the explanation would push the
// next author to delete the sentence that stops them repeating it.
func bodyVersionKeys(file *ast.File) []token.Pos {
	var found []token.Pos
	ast.Inspect(file, func(node ast.Node) bool {
		lit, isLit := node.(*ast.BasicLit)
		if !isLit || lit.Kind != token.STRING {
			return true
		}
		text, err := strconv.Unquote(lit.Value)
		if err == nil && text == bodyVersionKey {
			found = append(found, lit.Pos())
		}
		return true
	})
	return found
}
