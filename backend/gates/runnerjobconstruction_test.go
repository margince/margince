// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

//go:build !integration

package gates

// Where a runner.Job may be built. Compose's allowlist census reads compose
// alone, so a construction anywhere else in backend/ fails here.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// buildsAJobLegitimately are the files that may construct a runner.Job. An
// entry that no longer matches fails, so no permission outlives its file.
var buildsAJobLegitimately = gatekit.Waive(map[string]string{
	"internal/compose/runnerservice.go":      "the scheduled path, carrying the spec's own allowlist",
	"internal/compose/runnerresume.go":       "the resumed path, carrying the current catalog entry's allowlist rather than the one staged with the call, so a run parked across a catalog change resumes under what the entry says now",
	"internal/compose/certcase_agentloop.go": "the certification lane, which builds its Job from ScheduledAgentSpecByName's own allowlist",
})

// jobConstructionForms are the ways a runner.Job comes into existence: a
// literal, new, a typed var, a helper's result and a struct field's zero value.
// It reads the `runner.Job` spelling, so an import alias walks past it.
func jobConstructionForms(file *ast.File) []token.Pos {
	// A struct type in a signature is received, not built, so it is skipped.
	received := map[ast.Node]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncType)
		if !ok {
			return true
		}
		for _, list := range []*ast.FieldList{fn.Params, fn.Results} {
			if list == nil {
				continue
			}
			ast.Inspect(list, func(inner ast.Node) bool {
				if st, isStruct := inner.(*ast.StructType); isStruct {
					received[st] = true
				}
				return true
			})
		}
		return true
	})

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
		case *ast.StructType: // a struct field holding a runner.Job, read as its zero value
			// Not *ast.Field: parameters and results are fields too, and pass a Job on.
			if node.Fields == nil || received[node] {
				return true
			}
			for _, field := range node.Fields.List {
				if isRunnerJob(field.Type) {
					at = append(at, field.Pos())
				}
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

// isRunnerJob reports whether a type expression is spelled runner.Job.
func isRunnerJob(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "runner" && sel.Sel.Name == "Job"
}

// The walk reads backend/ only. runner sits under backend/internal/, so Go's
// internal rule keeps every module outside backend/ from naming runner.Job.
func TestOnlySanctionedFilesBuildARunnerJob(t *testing.T) {
	t.Parallel()
	root := "."
	var offenders []string
	seen := map[string]bool{}

	err := gatekit.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			rel := filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator)))
			// The runner package owns the type. Its full path is pinned, so a
			// second package named runner does not inherit the exemption.
			switch {
			case d.Name() == ".git" || d.Name() == "node_modules",
				rel == "internal/modules/agents/runner":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, perr := gatekit.ParseFile(path, 0)
		if perr != nil {
			// A file the scan cannot read is one it cannot clear.
			return fmt.Errorf("parsing %s, so this gate cannot say whether it builds a runner.Job: %w", path, perr)
		}
		if len(jobConstructionForms(parsed)) == 0 {
			return nil
		}
		rel := filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator)))
		seen[rel] = true
		if !buildsAJobLegitimately.Waived(t, rel) {
			offenders = append(offenders, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the tree: %v", err)
	}

	sort.Strings(offenders)
	for _, file := range offenders {
		t.Errorf("%s builds a runner.Job outside the files the allowlist census reads, so nothing "+
			"checks that its Tools come from a catalog entry. Route it through compose's "+
			"scheduledAgents(), or add the file to buildsAJobLegitimately with the reason", file)
	}
	// A scan of the wrong root reads like a clean tree, so finding nothing fails.
	buildsAJobLegitimately.AssertAllMatched(t)
	if len(seen) == 0 {
		t.Error("no file in the tree builds a runner.Job, so this gate is reading the wrong root, " +
			"which is indistinguishable from a tree with nothing to find")
	}
}

// Each form is one a scan of composite literals alone would miss.
func TestTheJobConstructionScanSeesEveryFormOfIt(t *testing.T) {
	t.Parallel()
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
		// No function here returns or takes a runner.Job, so only the field is seen.
		{"a struct field consumed as its zero value", `package p
import "x/runner"
type holder struct{ job runner.Job }
func f() { _ = holder{}.job }`},
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
	// A parameter, a result and an interface method pass a Job on and build none.
	passesOneThrough := `package p
import "x/runner"
type iface interface{ take(j runner.Job) }
func f(job runner.Job) { _ = job }
func g(arg struct{ job runner.Job }) { _ = arg }`
	parsed, err := parser.ParseFile(token.NewFileSet(), "x.go", passesOneThrough, 0)
	if err != nil {
		t.Fatalf("parsing the fixture: %v", err)
	}
	if got := len(jobConstructionForms(parsed)); got != 0 {
		t.Errorf("the scan reported %d constructions in a file that only passes a Job on: a parameter, "+
			"a result and an interface method are all Field nodes, and none of them builds one", got)
	}

	unrelated := `package p
import "x/runner"
func f() { _ = runner.Result{}; var q int; _ = q }`
	parsed, err = parser.ParseFile(token.NewFileSet(), "x.go", unrelated, 0)
	if err != nil {
		t.Fatalf("parsing the fixture: %v", err)
	}
	if got := len(jobConstructionForms(parsed)); got != 0 {
		t.Errorf("the scan reported %d Job constructions in a file that builds none", got)
	}
}
