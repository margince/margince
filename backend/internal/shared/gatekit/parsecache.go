// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sync"
	"weak"
)

// sourceFiles is the one FileSet every ParseFile tree resolves positions
// against. FileSet is safe for concurrent use, so parallel gates share it.
var sourceFiles = token.NewFileSet()

type parseKey struct {
	spelled string
	mode    parser.Mode
}

var parses = holder[parseKey, *parseSlot]{entries: map[parseKey]*held[*parseSlot]{}}

// parseSlot holds one version's tree while some gate references it. A tree no
// gate holds any more is collected rather than kept to the end of the run.
// base is where the first parse placed the file in sourceFiles. Every later
// parse lands on the same positions, so a token.Pos outlives its tree.
type parseSlot struct {
	mu   sync.Mutex
	tree weak.Pointer[ast.File]
	base int
}

// get answers the live tree, or parses again once the collector has taken it.
// A tree that failed to parse is never held, so its error is reported afresh.
func (slot *parseSlot) get(path string, mode parser.Mode) (*ast.File, error) {
	slot.mu.Lock()
	defer slot.mu.Unlock()
	if tree := slot.tree.Value(); tree != nil {
		return tree, nil
	}
	src, err := ReadFile(path)
	if err != nil {
		return nil, err
	}
	files := sourceFiles
	if slot.base > 0 {
		files = fileSetFrom(slot.base)
	}
	tree, err := parser.ParseFile(files, path, src, mode)
	if err != nil {
		return tree, err
	}
	if slot.base == 0 {
		slot.base = int(tree.FileStart)
	}
	slot.tree = weak.Make(tree)
	return tree, nil
}

// fileSetFrom is a throwaway FileSet whose next file starts at base. A re-parse
// then reuses the positions sourceFiles already resolves. Its line table goes
// with the tree, so sourceFiles does not grow.
func fileSetFrom(base int) *token.FileSet {
	files := token.NewFileSet()
	if pad := base - files.Base() - 1; pad >= 0 {
		files.AddFile("", -1, pad)
	}
	return files
}

// ParseFile parses the Go file at path under mode. It shares the tree with every
// caller while one of them holds it and the file is unchanged.
//
// The tree is SHARED with every concurrent gate: read it, never modify it.
// Positions resolve against SourceFileSet, and a position's filename is path as
// spelled here.
//
// SkipObjectResolution is honoured by sharing the resolved tree: it only leaves
// fields unset, so a caller that asked for less reads the same syntax, and the
// module is held once per comment mode rather than twice.
func ParseFile(path string, mode parser.Mode) (*ast.File, error) {
	mode &^= parser.SkipObjectResolution
	entry, err := parses.at(parseKey{spelled: path, mode: mode}, path, treeFixed.Load() && holdable(path))
	if err != nil {
		return nil, err
	}
	slot, err := entry.fill(func() (*parseSlot, error) { return &parseSlot{}, nil })
	if err != nil {
		return nil, err
	}
	return slot.get(path, mode)
}

// SourceFileSet is the FileSet a ParseFile tree's positions belong to.
func SourceFileSet() *token.FileSet { return sourceFiles }
