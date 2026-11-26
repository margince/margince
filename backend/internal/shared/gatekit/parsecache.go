// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sync"
	"weak"
)

// sourceFiles is the one FileSet every ParseFile tree resolves positions
// against. FileSet is safe for concurrent use, so parallel gates share it.
var sourceFiles = token.NewFileSet()

// parseKey holds name beside file because a fixture's two spellings carry two
// position names, and so need two trees.
type parseKey struct {
	file string
	name string
	mode parser.Mode
}

var parses = holder[parseKey, parseSlot]{entries: map[parseKey]*held[parseSlot]{}}

// parseSlot holds one version's tree while some gate references it. A tree no
// gate holds any more is collected rather than kept to the end of the run.
// base is where the first parse placed the file in sourceFiles. Every later
// parse lands on the same positions, so a token.Pos outlives its tree.
type parseSlot struct {
	mu   sync.Mutex
	tree weak.Pointer[ast.File]
	base int
}

// get answers the live tree, or parses file again under name once the
// collector has taken it. A tree that failed to parse is never held, so its
// error is reported afresh.
func (slot *parseSlot) get(file, name string, mode parser.Mode) (*ast.File, error) {
	slot.mu.Lock()
	defer slot.mu.Unlock()
	if tree := slot.tree.Value(); tree != nil {
		return tree, nil
	}
	src, err := ReadFile(file)
	if err != nil {
		return nil, err
	}
	files := sourceFiles
	if slot.base > 0 {
		files = fileSetFrom(slot.base)
	}
	tree, err := parser.ParseFile(files, name, src, mode)
	if slot.base == 0 && tree != nil {
		slot.base = int(tree.FileStart)
	}
	if err != nil {
		return tree, err
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
// Positions resolve against SourceFileSet. Every spelling of a tree file shares
// one tree, named relative to the working directory. A temp-dir fixture keeps
// the caller's spelling, which its test compares positions against.
//
// SkipObjectResolution is honoured by sharing the resolved tree: it only leaves
// fields unset, so a caller that asked for less reads the same syntax, and the
// module is held once per comment mode rather than twice.
func ParseFile(path string, mode parser.Mode) (*ast.File, error) {
	mode &^= parser.SkipObjectResolution
	fixed := treeFixed.Load()
	file, err := heldPath(path, fixed)
	if err != nil {
		return nil, err
	}
	name, err := positionName(path, file, fixed)
	if err != nil {
		return nil, err
	}
	entry, err := parses.at(parseKey{file: file, name: name, mode: mode}, file, fixed && holdable(file))
	if err != nil {
		return nil, err
	}
	return entry.value.get(file, name, mode)
}

// positionName is the filename ParseFile's positions carry for path, held as file.
func positionName(path, file string, fixed bool) (string, error) {
	if !holdable(file) {
		return path, nil
	}
	dir, err := workingDir(fixed)
	if err != nil {
		return "", err
	}
	return filepath.Rel(dir, file)
}

// SourceFileSet is the FileSet a ParseFile tree's positions belong to.
func SourceFileSet() *token.FileSet { return sourceFiles }
