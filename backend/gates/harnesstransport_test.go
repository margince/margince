// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A harness transport stays in the harness: only the cert lane hands the router
// a client of its own, and only an allowlisted directory shells out.
//
// ai.WithHarnessClient is reachable from every binary that builds a DB-less
// router — cmd/worker does, through the site-read debug brain — so nothing in
// its type keeps the CLI judge out of a product process. This census does: it
// parses every hand-written, non-test Go file rather than grepping lines, so a
// call split over two lines or aliased import still counts.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	harnessClientOption = "WithHarnessClient"
	// harnessClientOwner is the file that declares the option; its declaration
	// is not a use of it.
	harnessClientOwner = "internal/modules/ai/localrouter.go"
	harnessClientUser  = "internal/compose/aicert"
)

// processSpawners are the directories whose non-test code may import os/exec,
// each with what running a subprocess there costs.
var processSpawners = gatekit.Waive(map[string]string{
	"internal/compose/aicert":      "the cert lane's claude_cli judge runs `claude -p`, a subscription-billed CLI a product process must never reach",
	"internal/compose/integration": "a benchmark record reads the host's CPU and memory through sysctl, in the integration-tagged harness only",
	"tools/gen-composition":        "the composition generator asks GOROOT's go for the module graph at build time, never at runtime",
	"../desktop/launcher":          "the desktop launcher supervises Postgres and the API as child processes, which is the whole of its job",
})

// harnessSourceFile is one parsed non-test Go file and the directory it sits in.
type harnessSourceFile struct {
	path, dir string
	file      *ast.File
}

func parseProductGoFiles(t *testing.T) []harnessSourceFile {
	t.Helper()
	var files []harnessSourceFile
	fset := token.NewFileSet()
	for _, tree := range licensedTrees {
		walkHandWrittenGoFiles(t, tree.root, func(filePath, text string) {
			if strings.HasSuffix(filePath, "_test.go") {
				return
			}
			parsed, err := parser.ParseFile(fset, filePath, text, parser.SkipObjectResolution)
			if err != nil {
				t.Fatalf("parsing %s: %v", filePath, err)
			}
			files = append(files, harnessSourceFile{path: filePath, dir: path.Dir(filePath), file: parsed})
		})
	}
	if len(files) == 0 {
		t.Fatal("the census parsed no Go file — a sweep that reads nothing passes exactly like a clean one")
	}
	return files
}

// harnessClientUses counts every identifier naming the option — a call, a
// selector or a function value — other than its own declaration.
func harnessClientUses(f harnessSourceFile) (uses int, declared bool) {
	ast.Inspect(f.file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			if node.Recv == nil && node.Name.Name == harnessClientOption {
				declared = true
				return false
			}
		case *ast.Ident:
			if node.Name == harnessClientOption {
				uses++
			}
		}
		return true
	})
	return uses, declared
}

func importsOSExec(f *ast.File) bool {
	return slices.ContainsFunc(f.Imports, func(spec *ast.ImportSpec) bool {
		importPath, err := strconv.Unquote(spec.Path.Value)
		return err == nil && importPath == "os/exec"
	})
}

func TestOnlyTheCertLaneHandsTheRouterAHarnessClient(t *testing.T) {
	t.Parallel()
	declared := false
	for _, f := range parseProductGoFiles(t) {
		uses, declares := harnessClientUses(f)
		if declares {
			declared = true
			if f.path != harnessClientOwner {
				t.Errorf("%s declares %s; the census expects it in %s — move the constant with it", f.path, harnessClientOption, harnessClientOwner)
			}
		}
		if uses > 0 && f.dir != harnessClientUser {
			t.Errorf("%s names ai.%s outside %s: a harness transport would reach a product router — "+
				"serve the provider through an adapter in internal/modules/ai instead", f.path, harnessClientOption, harnessClientUser)
		}
	}
	if !declared {
		t.Fatalf("no file declares %s any more — a renamed option leaves this census guarding nothing; point it at the new name", harnessClientOption)
	}
}

func TestOnlyAnAllowlistedDirectoryShellsOut(t *testing.T) {
	t.Parallel()
	defer processSpawners.AssertAllMatched(t)
	for _, f := range parseProductGoFiles(t) {
		if importsOSExec(f.file) && !processSpawners.Waived(t, f.dir) {
			t.Errorf("%s imports os/exec: product code does not shell out. If a subprocess is this directory's job, "+
				"add it to processSpawners with what it costs", f.path)
		}
	}
}
