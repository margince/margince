// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"runtime"
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
	if slot.base == 0 && tree != nil {
		slot.base = int(tree.FileStart)
	}
	if err != nil {
		return tree, err
	}
	slot.tree = weak.Make(tree)
	holdFacts(tree, slot.tree)
	return tree, nil
}

// treeFacts are what gatekit derives from one shared tree. Each is read once,
// on first use, and dropped with the tree, so it never pins one.
type treeFacts struct {
	statementsOnce sync.Once
	statements     []string
	literalsOnce   sync.Once
	literals       []string
}

// sharedTrees maps each tree ParseFile is sharing to its facts. A tree a caller
// parsed for itself is not here: that caller may still change it.
var sharedTrees sync.Map // weak.Pointer[ast.File] -> *treeFacts

func holdFacts(tree *ast.File, key weak.Pointer[ast.File]) {
	sharedTrees.Store(key, &treeFacts{})
	runtime.AddCleanup(tree, func(key weak.Pointer[ast.File]) { sharedTrees.Delete(key) }, key)
}

// factsOf answers the facts of a tree ParseFile shared.
func factsOf(file *ast.File) (*treeFacts, bool) {
	held, ok := sharedTrees.Load(weak.Make(file))
	if !ok {
		return nil, false
	}
	facts, ok := held.(*treeFacts)
	return facts, ok
}

// sqlStatementsOf is the file's statements, which every caller shares.
func (facts *treeFacts) sqlStatementsOf(file *ast.File) []string {
	facts.statementsOnce.Do(func() { facts.statements = sqlStatements(file, nil) })
	return facts.statements
}

// stringLiterals is the text of every string literal in the file, in source
// order, which every caller shares.
func (facts *treeFacts) stringLiterals(file *ast.File) []string {
	facts.literalsOnce.Do(func() { facts.literals = stringLiterals(file) })
	return facts.literals
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
// A whole-file tree always carries its comments and resolved objects, whatever
// mode asked: both only fill fields, so the module is parsed and held once. A
// caller that prints code without comments uses ParseFileWithoutComments.
func ParseFile(path string, mode parser.Mode) (*ast.File, error) {
	mode &^= parser.SkipObjectResolution
	if mode&(parser.PackageClauseOnly|parser.ImportsOnly) == 0 {
		mode |= parser.ParseComments
	}
	return parseShared(path, mode)
}

// ParseFileWithoutComments is ParseFile for a caller that prints code and must
// not print its comments: go/printer writes every comment a tree holds.
func ParseFileWithoutComments(path string) (*ast.File, error) {
	return parseShared(path, 0)
}

func parseShared(path string, mode parser.Mode) (*ast.File, error) {
	entry, err := parses.at(parseKey{spelled: path, mode: mode}, path, treeFixed.Load() && holdable(path))
	if err != nil {
		return nil, err
	}
	return entry.value.get(path, mode)
}

// SourceFileSet is the FileSet a ParseFile tree's positions belong to.
func SourceFileSet() *token.FileSet { return sourceFiles }
