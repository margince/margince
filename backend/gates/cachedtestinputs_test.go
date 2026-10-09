// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

import (
	"go/ast"
	"go/parser"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// A cached test file that runs a process to read the tree for it declares what
// that process read through gatekit. Go's test cache sees what the test opens
// and nothing a child process opens, so a cached pass over the process's answer
// replays after the tree has changed underneath it. The census judges test
// files, file by file: a production helper that shells out is out of its sight.
var treeReaders = map[string]bool{"git": true, "go": true}

// launchers run whatever their arguments name, so they read the tree when an
// argument names a tree reader or cannot be read at all.
var launchers = map[string]bool{"sh": true, "bash": true, "env": true}

var namesATreeReader = regexp.MustCompile(`\b(git|go)\b`)

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
	for pkg := range strings.FieldsSeq(string(line[1])) {
		dirs[path.Join("backend", strings.TrimSuffix(pkg, "/"))] = true
	}
	return dirs
}

// shellsOutForTheTree reports the first process a test file runs that may read
// the tree for it, or "" when the file runs none.
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
		if runsOne && len(call.Args) > programAt {
			found = treeReaderIn(call.Args[programAt:])
		}
		return found == ""
	})
	return found
}

// treeReaderIn judges one command line: a tree reader by name, a launcher whose
// arguments name one or cannot be read, or a program named at run time.
func treeReaderIn(command []ast.Expr) string {
	program, literal := gatekit.StringExpr(command[0], nil, gatekit.FoldStrict)
	if !literal {
		return "a program named at run time"
	}
	switch name := path.Base(program); {
	case treeReaders[name]:
		return program
	case launchers[name]:
		for _, arg := range command[1:] {
			text, isLiteral := gatekit.StringExpr(arg, nil, gatekit.FoldStrict)
			if !isLiteral || namesATreeReader.MatchString(text) {
				return program + " running a tree reader"
			}
		}
	}
	return ""
}

// declaresItsInputs reports whether the file tells the test cache what the
// process read, through gatekit's declarations.
func declaresItsInputs(file *ast.File) bool {
	gk := importAliasOf(file, "github.com/margince/margince/backend/internal/shared/gatekit")
	if gk == "" {
		return false
	}
	declared := false
	ast.Inspect(file, func(node ast.Node) bool {
		// A call, not a mention: `_ = gatekit.DeclareInputs` declares nothing.
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return !declared
		}
		sel, isSel := call.Fun.(*ast.SelectorExpr)
		if isSel && (sel.Sel.Name == "DeclareInputs" || sel.Sel.Name == "DeclareListings") {
			if pkg, isIdent := sel.X.(*ast.Ident); isIdent && pkg.Name == gk {
				declared = true
			}
		}
		return !declared
	})
	return declared
}

func TestEveryCachedTestFileThatShellsOutForTheTreeDeclaresItsInputs(t *testing.T) {
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

// The script exports a digest under the name it derives and the test reads the
// name gatekit derives; one differing character and the test reads an unset
// variable, keying the cache on nothing while the census reports it declared.
func TestTheScriptAndGatekitNameEveryTreeDigestAlike(t *testing.T) {
	t.Parallel()
	out, err := exec.Command("bash", filepath.Join(repoRoot, "scripts", "ci-stable-mtimes.sh"), "--names").Output()
	if err != nil {
		t.Fatalf("asking scripts/ci-stable-mtimes.sh for its digest names: %v", err)
	}
	named := 0
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		top, name, found := strings.Cut(line, "\t")
		if !found {
			t.Fatalf("scripts/ci-stable-mtimes.sh --names printed %q, not an entry and its variable", line)
		}
		if want := gatekit.TreeDigestVar(top); name != want {
			t.Errorf("the script exports %s's digest as %s and gatekit.TreeDigestVar reads %s", top, name, want)
		}
		named++
	}
	if named < 10 {
		t.Fatalf("the script named %d top-level entries, too few to be this repository", named)
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
		{"git behind a shell", `exec.Command("sh", "-c", "git ls-files | wc -l")`, true},
		{"go behind env", `exec.Command("/usr/bin/env", "go", "list")`, true},
		{"a shell running a script named at run time", `exec.Command("bash", "-c", script)`, true},
		{"a shell that reads no tree", `exec.Command("/bin/sh", "-c", "exit 3")`, false},
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
}

func TestTheShellOutCensusKnowsADeclarationFromALookalike(t *testing.T) {
	t.Parallel()
	const gatekitImport = `"github.com/margince/margince/backend/internal/shared/gatekit"`
	for _, tc := range []struct {
		name, imports, body string
		declares            bool
	}{
		{"a declared tree", gatekitImport, `gatekit.DeclareInputs(".")`, true},
		{"a declared listing", gatekitImport, `gatekit.DeclareListings(".")`, true},
		{"an aliased import", "gk " + gatekitImport, `gk.DeclareInputs(".")`, true},
		{"another gatekit helper", gatekitImport, `gatekit.SourceFileSet()`, false},
		{"a mention that is never called", gatekitImport, `gatekit.DeclareInputs`, false},
		{"another package's DeclareInputs", `gatekit "example.com/other"`, `gatekit.DeclareInputs(".")`, false},
	} {
		source := "package p\nimport (\n\"os/exec\"\n" + tc.imports + "\n)\n" +
			"func f() { _ = " + tc.body + "; _ = exec.Command(\"git\") }\n"
		file, err := parser.ParseFile(gatekit.SourceFileSet(), tc.name+".go", source, 0)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got := declaresItsInputs(file); got != tc.declares {
			t.Errorf("%s: judged as declaring its inputs = %v, want %v", tc.name, got, tc.declares)
		}
	}
}
