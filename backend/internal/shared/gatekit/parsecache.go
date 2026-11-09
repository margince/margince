// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

import (
	"go/ast"
	"go/parser"
	"go/token"
)

// sourceFiles is the one FileSet every ParseFile tree resolves positions
// against. FileSet is safe for concurrent use, so parallel gates share it.
var sourceFiles = token.NewFileSet()

type parseKey struct {
	spelled string
	mode    parser.Mode
}

var parses = holder[parseKey, *ast.File]{entries: map[parseKey]*held[*ast.File]{}}

// ParseFile parses the Go file at path under mode, once per test binary for as
// long as the file on disk is unchanged. Hundreds of gates walk the same module,
// and each re-parsing it for itself was most of the gate lane's CPU.
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
	return entry.fill(func() (*ast.File, error) {
		src, err := ReadFile(path)
		if err != nil {
			return nil, err
		}
		return parser.ParseFile(sourceFiles, path, src, mode)
	})
}

// SourceFileSet is the FileSet a ParseFile tree's positions belong to.
func SourceFileSet() *token.FileSet { return sourceFiles }
