// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A gate reads the tree through gatekit's shared view, never from the disk on
// its own.
//
// A gate that reads the disk itself still passes. The cost shows only as a
// slower CI job, since every gate rereads the same files.
//
// It sees the four calls below under any import name. It cannot see a read
// through another package, by a child process, or through os.Open.

import (
	"go/ast"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// diskReads are the calls the shared view stands in for, by import path.
var diskReads = map[string][]string{
	"os":            {"ReadFile", "ReadDir"},
	"path/filepath": {"WalkDir", "Walk"},
}

// sharedTreeReaders are the gatekit calls a gate reads the tree through.
var sharedTreeReaders = []string{"ReadFile", "ReadDir", "WalkDir"}

// directDiskReads are the functions that must reach the disk themselves,
// keyed "path:function".
var directDiskReads = gatekit.Waive(map[string]string{
	"internal/shared/gatekit/tree.go:ReadFile": "is the shared view itself, and reads each file version once",
	"internal/shared/gatekit/tree.go:ReadDir":  "is the shared view itself, and lists each directory version once",
	"internal/shared/gatekit/cacheinputs.go:DeclareInputs": "must open the directory itself, since Go's test cache " +
		"records only what the test process opens",
	"internal/shared/gatekit/cacheinputs.go:DeclareListings": "must open the directory itself, since Go's test cache " +
		"records only what the test process opens",
})

// Below the real counts: about 700 files and 750 shared reads.
const (
	sharedTreeFileFloor = 600
	sharedTreeReadFloor = 600
)

func TestEveryGateReadsTheTreeThroughTheSharedView(t *testing.T) {
	t.Parallel()
	files := sharedTreeCensusFiles(t)
	if len(files) < sharedTreeFileFloor {
		t.Fatalf("the census read %d Go files under gates/ and internal/shared/gatekit/, below the floor of %d: "+
			"the listing went short, and a short census passes over the gates it never read", len(files), sharedTreeFileFloor)
	}
	shared := 0
	for _, path := range files {
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		reads := treeReadsIn(file)
		shared += reads.shared
		for _, read := range reads.direct {
			key := filepath.ToSlash(path) + ":" + read.function
			if directDiskReads.Waived(t, key) {
				continue
			}
			t.Errorf("%s:%d: %s calls %s, which reads the disk on its own: use gatekit.%s, which answers the "+
				"same and reads each file once for every gate in the run", filepath.ToSlash(path),
				gatekit.SourceFileSet().Position(read.at.Pos()).Line, read.function, read.call, sharedName(read.call))
		}
	}
	if shared < sharedTreeReadFloor {
		t.Errorf("the census found %d reads through gatekit's shared view, below the floor of %d: the matcher "+
			"stopped recognising them, so it no longer proves it can see a read at all", shared, sharedTreeReadFloor)
	}
	directDiskReads.AssertAllMatched(t)
}

// sharedTreeCensusFiles lists the Go files of this package and gatekit's
// hand-written sources, which use the shared view and implement it.
func sharedTreeCensusFiles(t *testing.T) []string {
	t.Helper()
	var files []string
	for _, dir := range []string{"gates", "internal/shared/gatekit"} {
		entries, err := gatekit.ReadDir(dir)
		if err != nil {
			t.Fatalf("listing %s: %v", dir, err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasSuffix(name, ".go") || (dir != "gates" && strings.HasSuffix(name, "_test.go")) {
				continue
			}
			files = append(files, filepath.Join(dir, name))
		}
	}
	return files
}

type directRead struct {
	function, call string
	at             ast.Node
}

type treeReads struct {
	direct []directRead
	shared int
}

// treeReadsIn finds every disk read and shared read in one file. A reference
// counts whether it is called or passed as a value.
func treeReadsIn(file *ast.File) treeReads {
	imported := map[string]string{}
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		name := path[strings.LastIndex(path, "/")+1:]
		if spec.Name != nil {
			name = spec.Name.Name
		}
		imported[name] = path
	}
	var reads treeReads
	for _, decl := range file.Decls {
		function := declName(decl)
		ast.Inspect(decl, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Obj != nil {
				return true
			}
			path := imported[pkg.Name]
			switch {
			case path == gatekitImportPath && slices.Contains(sharedTreeReaders, sel.Sel.Name):
				reads.shared++
			case slices.Contains(diskReads[path], sel.Sel.Name):
				reads.direct = append(reads.direct, directRead{function: function, call: path[strings.LastIndex(path, "/")+1:] + "." + sel.Sel.Name, at: sel})
			}
			return true
		})
		if file.Name.Name == "gatekit" {
			reads.shared += unqualifiedSharedCalls(decl)
		}
	}
	return reads
}

// unqualifiedSharedCalls counts gatekit's own calls of its shared view, which
// it spells without a package name.
func unqualifiedSharedCalls(decl ast.Decl) int {
	found := 0
	ast.Inspect(decl, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ident, ok := call.Fun.(*ast.Ident); ok && slices.Contains(sharedTreeReaders, ident.Name) {
			found++
		}
		return true
	})
	return found
}

// sharedName is the gatekit call that stands in for a disk read; filepath.Walk
// becomes WalkDir, whose callback takes a DirEntry.
func sharedName(call string) string {
	name := call[strings.LastIndex(call, ".")+1:]
	if name == "Walk" {
		return "WalkDir"
	}
	return name
}
