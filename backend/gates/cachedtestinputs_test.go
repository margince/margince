// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// treeReaders are the programs a test runs to read the tree for it. Go's test
// cache sees what the test opens and nothing a child process opens, so a cached
// pass over their answer replays after the tree has changed underneath it.
var treeReaders = map[string]bool{"git": true, "go": true}

// uncachedPackages reads UNCACHED_TEST_PKGS out of the Makefile that runs them,
// so this gate exempts exactly the packages that never replay.
func uncachedPackages(t *testing.T) map[string]bool {
	t.Helper()
	makefile, err := os.ReadFile(filepath.Join(repoRoot, "backend", "Makefile"))
	if err != nil {
		t.Fatalf("reading backend/Makefile for UNCACHED_TEST_PKGS: %v", err)
	}
	line := regexp.MustCompile(`(?m)^UNCACHED_TEST_PKGS\s*:=(.*)$`).FindSubmatch(makefile)
	if line == nil {
		t.Fatal("backend/Makefile declares no UNCACHED_TEST_PKGS — this gate cannot tell which packages replay")
	}
	dirs := map[string]bool{}
	for _, pkg := range strings.Fields(string(line[1])) {
		dirs[path.Join("backend", strings.TrimSuffix(pkg, "/"))] = true
	}
	return dirs
}

// shellsOutForTheTree reports the first process a test file runs that may read
// the tree for it: git or go by name, or a program it cannot name, which may be
// either. It answers "" when the file runs none.
func shellsOutForTheTree(file *ast.File) string {
	execName := importAliasOf(file, "os/exec")
	if execName == "" {
		return ""
	}
	found := ""
	ast.Inspect(file, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall || found != "" {
			return found == ""
		}
		sel, isSel := call.Fun.(*ast.SelectorExpr)
		if !isSel {
			return true
		}
		pkg, isIdent := sel.X.(*ast.Ident)
		if !isIdent || pkg.Name != execName {
			return true
		}
		programAt, runsOne := map[string]int{"Command": 0, "CommandContext": 1}[sel.Sel.Name]
		if !runsOne || len(call.Args) <= programAt {
			return true
		}
		program := "a program named at run time"
		if lit, isLit := call.Args[programAt].(*ast.BasicLit); isLit && lit.Kind == token.STRING {
			if name, err := strconv.Unquote(lit.Value); err == nil {
				if !treeReaders[path.Base(name)] {
					return true
				}
				program = name
			}
		}
		found = program
		return false
	})
	return found
}

// declaresItsInputs reports whether the file tells the test cache what the
// process read, through the one helper that does.
func declaresItsInputs(file *ast.File) bool {
	gk := importAliasOf(file, "github.com/margince/margince/backend/internal/shared/gatekit")
	declared := false
	ast.Inspect(file, func(node ast.Node) bool {
		if sel, isSel := node.(*ast.SelectorExpr); isSel && sel.Sel.Name == "DeclareInputs" {
			if pkg, isIdent := sel.X.(*ast.Ident); isIdent && gk != "" && pkg.Name == gk {
				declared = true
			}
		}
		return !declared
	})
	return declared
}

func TestEveryCachedTestThatShellsOutForTheTreeDeclaresWhatItRead(t *testing.T) {
	t.Parallel()
	uncached := uncachedPackages(t)
	read, shelling := 0, 0
	for _, f := range trackedFiles(t) {
		if f.symlink || !strings.HasSuffix(f.path, "_test.go") || uncached[path.Dir(f.path)] {
			continue
		}
		file, err := gatekit.ParseFile(filepath.Join(repoRoot, f.path), 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", f.path, err)
		}
		read++
		program := shellsOutForTheTree(file)
		if program == "" {
			continue
		}
		shelling++
		if !declaresItsInputs(file) {
			t.Errorf("%s runs %s, whose reads Go's test cache cannot see, and declares none of them — "+
				"a cached pass here replays after the files it depends on have changed. Pass the "+
				"files and directories the process reads to gatekit.DeclareInputs.", f.path, program)
		}
	}
	// A census that read a fraction of the tree certifies a fraction of it.
	if read < 2000 {
		t.Fatalf("read %d test files, too few to be this tree — the walk has lost part of it", read)
	}
	if shelling < 3 {
		t.Fatalf("found %d test files running git or go, and this tree has at least three — the "+
			"recognizer has stopped seeing the calls it exists to judge", shelling)
	}
}

func TestTheShellOutCensusJudgesTheSpellingsItMustSee(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, body string
		shells     bool
	}{
		{"git by name", `exec.Command("git", "ls-files")`, true},
		{"go through a context", `exec.CommandContext(ctx, "go", "list")`, true},
		{"a git binary by path", `exec.Command("/usr/bin/git", "status")`, true},
		{"a program named at run time", `exec.Command(tool, "x")`, true},
		{"a process that reads no tree", `exec.Command("/bin/sleep", "60")`, false},
	} {
		source := "package p\nimport \"os/exec\"\nfunc f() { _ = " + tc.body + " }\n"
		file, err := parser.ParseFile(gatekit.SourceFileSet(), tc.name+".go", source, 0)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got := shellsOutForTheTree(file) != ""; got != tc.shells {
			t.Errorf("%s: judged as shelling out for the tree = %v, want %v", tc.name, got, tc.shells)
		}
	}

	declared := "package p\nimport (\n\"os/exec\"\n\"github.com/margince/margince/backend/internal/shared/gatekit\"\n)\n" +
		"func f() { _ = gatekit.DeclareInputs(\".\"); _ = exec.Command(\"git\") }\n"
	file, err := parser.ParseFile(gatekit.SourceFileSet(), "declared.go", declared, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !declaresItsInputs(file) {
		t.Error("a file calling gatekit.DeclareInputs was not recognised as declaring its inputs")
	}
}
