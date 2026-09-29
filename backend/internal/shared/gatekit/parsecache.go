// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sync"
)

// sourceFiles is the one FileSet every ParseFile tree resolves positions
// against. FileSet is safe for concurrent use, so parallel gates share it.
var sourceFiles = token.NewFileSet()

type parseKey struct {
	abs, spelled string
	mode         parser.Mode
}

// fileStamp is what a cached parse is valid for. A gate that plants a case by
// rewriting a file reads the rewrite, not the tree it parsed before.
type fileStamp struct {
	size, modified int64
}

type parsedEntry struct {
	stamp fileStamp
	once  sync.Once
	file  *ast.File
	err   error
}

var (
	parsesMu sync.Mutex
	parses   = map[parseKey]*parsedEntry{}
)

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
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	key := parseKey{abs: abs, spelled: path, mode: mode}
	stamp := fileStamp{size: info.Size(), modified: info.ModTime().UnixNano()}

	parsesMu.Lock()
	entry, held := parses[key]
	if !held || entry.stamp != stamp {
		entry = &parsedEntry{stamp: stamp}
		parses[key] = entry
	}
	parsesMu.Unlock()

	entry.once.Do(func() {
		entry.file, entry.err = parser.ParseFile(sourceFiles, path, nil, mode)
	})
	return entry.file, entry.err
}

// SourceFileSet is the FileSet a ParseFile tree's positions belong to.
func SourceFileSet() *token.FileSet { return sourceFiles }
