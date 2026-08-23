// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Where a runner.Job may be BUILT, held against the tree rather than against a
// list beside it.
//
// Job.Tools empty is read as NO narrowing (runner/job.go), and that reading is
// safe only while every construction site is one somebody has looked at. The
// existing gate in agentspectools_test.go proves the two production Jobs carry
// an allowlist; it reads ONE file by name, so a third construction anywhere
// else is invisible to it. This is the other half: the set of files allowed to
// build one at all.
//
// The certification lane is on the list deliberately. It builds a Job with no
// Tools because its fixture IS the offered surface, and the restraint band
// scores whether a turn declines a tempting tool — narrow that surface with an
// allowlist and the band passes for no reason, so the empty case is the honest
// one there.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// buildsAJobLegitimately are the files that may construct a runner.Job.
// Adding one is a decision about the catalog boundary, so it is made here in
// review rather than discovered later in a sweep.
var buildsAJobLegitimately = map[string]string{
	"internal/compose/runnerservice.go":      "the two production paths, each carrying a spec's own allowlist",
	"internal/compose/certcase_agentloop.go": "the certification lane, whose fixture is the offered surface",
}

// jobConstructionForms are the ways a runner.Job comes into existence. A gate
// that knew only the composite literal would be evaded by three of these while
// reporting the tree clean, which is worse than not having it.
func jobConstructionForms(file *ast.File) []token.Pos {
	var at []token.Pos
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CompositeLit: // runner.Job{...}
			if isRunnerJob(node.Type) {
				at = append(at, node.Pos())
			}
		case *ast.CallExpr: // new(runner.Job)
			if ident, ok := node.Fun.(*ast.Ident); ok && ident.Name == "new" &&
				len(node.Args) == 1 && isRunnerJob(node.Args[0]) {
				at = append(at, node.Pos())
			}
		case *ast.ValueSpec: // var job runner.Job
			if node.Type != nil && isRunnerJob(node.Type) {
				at = append(at, node.Pos())
			}
		case *ast.FuncDecl: // func ...() runner.Job
			if node.Type.Results == nil {
				return true
			}
			for _, result := range node.Type.Results.List {
				if isRunnerJob(result.Type) {
					at = append(at, node.Pos())
				}
			}
		}
		return true
	})
	return at
}

func TestOnlySanctionedFilesBuildARunnerJob(t *testing.T) {
	root := filepath.Join("..", "..")
	var offenders []string
	seen := map[string]bool{}

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// The runner's own package builds Jobs constantly and is the one
			// place that legitimately does: it OWNS the type, and its tests
			// exercise the empty case on purpose.
			switch d.Name() {
			case ".git", "node_modules", "runner":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, perr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if perr != nil {
			return nil // not this gate's subject; the build catches an unparseable file
		}
		if len(jobConstructionForms(parsed)) == 0 {
			return nil
		}
		rel := filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator)))
		seen[rel] = true
		if _, sanctioned := buildsAJobLegitimately[rel]; !sanctioned {
			offenders = append(offenders, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the tree: %v", err)
	}

	sort.Strings(offenders)
	for _, file := range offenders {
		t.Errorf("%s builds a runner.Job — Job.Tools empty is read as NO narrowing, so a construction "+
			"nobody reviewed hands its run every verb the passport admits. Route it through "+
			"compose's scheduledAgents(), or add the file to buildsAJobLegitimately with the reason", file)
	}
	// A gate that finds nothing because it is looking in the wrong place reads
	// exactly like a clean tree.
	for file, why := range buildsAJobLegitimately {
		if !seen[file] {
			t.Errorf("%s is sanctioned to build a runner.Job (%s) but builds none — "+
				"this gate is reading the wrong tree, or the entry is stale", file, why)
		}
	}
}

// Each form is one a composite-literal-only gate would miss. Without these the
// gate above would report a clean tree while three of the four ways to make a
// Job walked past it.
func TestTheJobConstructionScanSeesEveryFormOfIt(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{"a composite literal", `package p
import "x/runner"
func f() { _ = runner.Job{} }`},
		{"new()", `package p
import "x/runner"
func f() { _ = new(runner.Job) }`},
		{"a typed var", `package p
import "x/runner"
func f() { var j runner.Job; _ = j }`},
		{"a helper returning one", `package p
import "x/runner"
func f() runner.Job { panic("") }`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := parser.ParseFile(token.NewFileSet(), "x.go", tc.src, 0)
			if err != nil {
				t.Fatalf("parsing the fixture: %v", err)
			}
			if len(jobConstructionForms(parsed)) == 0 {
				t.Errorf("%s was not seen as building a runner.Job, so the gate can be evaded by writing it that way", tc.name)
			}
		})
	}
	unrelated := `package p
import "x/runner"
func f() { _ = runner.Result{}; var q int; _ = q }`
	parsed, err := parser.ParseFile(token.NewFileSet(), "x.go", unrelated, 0)
	if err != nil {
		t.Fatalf("parsing the fixture: %v", err)
	}
	if got := len(jobConstructionForms(parsed)); got != 0 {
		t.Errorf("the scan reported %d Job constructions in a file that builds none", got)
	}
}
