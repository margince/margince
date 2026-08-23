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
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gradionhq/margince/backend/internal/shared/gatekit"
)

// buildsAJobLegitimately are the files that may construct a runner.Job.
// Adding one is a decision about the catalog boundary, so it is made here in
// review rather than discovered later in a sweep.
//
// gatekit.Waive rather than a bare map: it holds each reason to a floor, and
// AssertAllMatched reports an entry that no longer matches anything — so a file
// that stops building a Job cannot leave its permission behind for whatever is
// written there next.
var buildsAJobLegitimately = gatekit.Waive(map[string]string{
	"internal/compose/runnerservice.go":      "the two production paths, each carrying a spec's own allowlist",
	"internal/compose/certcase_agentloop.go": "the certification lane, whose fixture is the offered surface",
})

// jobConstructionForms are the ways a runner.Job comes into existence. A gate
// that knew only the composite literal would be evaded by three of these while
// reporting the tree clean, which is worse than not having it.
//
// The struct-field form is here because the certification lane already uses it
// (`certcase_agentloop.go` holds a `job runner.Job`), and a Job reached through
// a zero-valued field carries empty Tools exactly like one built with a literal.
//
// It matches on the `runner.Job` spelling, so an import alias (`import r
// ".../runner"` then `r.Job{}`) walks past it. That is a known limit, shared
// with the older gate in agentspectools_test.go, and it is left rather than
// papered over: closing it means resolving imports per file, and an alias for
// this package would itself be a strange thing to find in review.
func jobConstructionForms(file *ast.File) []token.Pos {
	// A struct type written INSIDE a signature — `func f(arg struct{ job
	// runner.Job })` — is a shape the function receives, not one it builds, so
	// those are collected first and skipped below. Rare, but a gate that
	// reports a file for a Job it was handed sends someone to read the wrong
	// line.
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
		case *ast.StructType: // a struct FIELD holding a runner.Job, consumed as its zero value
			// Walked from the StructType rather than matching *ast.Field,
			// because a FieldList is also how the AST spells parameters,
			// results, receivers and interface methods — so a bare Field case
			// would report `func f(job runner.Job)` as building one, and would
			// double-count every function that returns one.
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

// The walk covers backend/ AND THAT IS THE WHOLE REACH, which is worth saying
// because "the tree" would be an overclaim. runner lives under
// backend/internal/, so Go's internal rule puts it out of reach of anything
// outside backend/ — extensions/ and fixtures/ are separate modules that may
// import only the marker-allowlisted backend/pkg/** surface, and neither can
// name runner.Job at all. A file that could build one is a file inside this
// walk, by the language rather than by this gate's diligence.
func TestOnlySanctionedFilesBuildARunnerJob(t *testing.T) {
	root := filepath.Join("..", "..")
	var offenders []string
	seen := map[string]bool{}

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			rel := filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator)))
			// The runner's own package builds Jobs constantly and is the one
			// place that legitimately does: it OWNS the type, and its tests
			// exercise the empty case on purpose. Pinned to the full path
			// rather than the base name — a future modules/<x>/runner/ must not
			// inherit the exemption by being called the same thing.
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
		parsed, perr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if perr != nil {
			// A file this scan cannot read is a file it cannot clear, and
			// "unreadable" must not resolve to "clean". The build would also
			// fail on it, but not before this gate had already reported the
			// tree green.
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
		t.Errorf("%s builds a runner.Job — Job.Tools empty is read as NO narrowing, so a construction "+
			"nobody reviewed hands its run every verb the passport admits. Route it through "+
			"compose's scheduledAgents(), or add the file to buildsAJobLegitimately with the reason", file)
	}
	// A gate that finds nothing because it is looking in the wrong place reads
	// exactly like a clean tree, so a sanction that matched nothing is reported
	// rather than assumed dormant.
	buildsAJobLegitimately.AssertAllMatched(t)
	if len(seen) == 0 {
		t.Error("no file in the tree builds a runner.Job — this gate is reading the wrong root, " +
			"which is indistinguishable from a tree with nothing to find")
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
		// No function here returns or takes a runner.Job, so ONLY the struct
		// field can trigger detection: remove that case and this fails.
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
	// A parameter and a result are Field nodes too, and neither BUILDS a Job —
	// the caller supplies one. A scan that counted them would report files
	// that merely pass a Job through as construction sites.
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
		t.Errorf("the scan reported %d constructions in a file that only PASSES a Job — a parameter, "+
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
