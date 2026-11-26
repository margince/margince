// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
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

var parses = holder[parseKey, *ast.File]{entries: map[parseKey]*held[*ast.File]{}}

// ParseFile parses the Go file at path under mode, once per test binary for as
// long as the file on disk is unchanged. Hundreds of gates walk the same module,
// and each re-parsing it for itself was most of the gate lane's CPU.
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
	return entry.fill(func() (*ast.File, error) {
		src, err := ReadFile(file)
		if err != nil {
			return nil, err
		}
		return parser.ParseFile(sourceFiles, name, src, mode)
	})
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
