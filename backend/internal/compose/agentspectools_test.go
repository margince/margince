// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// A catalog entry's allowlist is a list of NAMES, and a name is only as good as
// the registry it resolves against. Two ways it goes wrong, and neither is
// visible at the call site:
//
//   - a misspelt verb silently drops the one tool a goal depends on. The run
//     still starts, reads what it can, and reports a thin answer.
//   - an EMPTY set, or one naming every served tool, is a run offered the whole
//     catalog: every verb paid for on every step, and callable. The runner
//     refuses the first; nothing but this gate refuses the second.
//
// Derived from the live registry rather than from a list kept beside it, so a
// tool that is renamed or retired fails here instead of at 02:00 in a sweep.
func TestEveryAgentSpecNamesRegisteredTools(t *testing.T) {
	registered := map[string]bool{}
	for _, spec := range NewRegistry(nil, SendPath{}).Specs() {
		registered[spec.Name] = true
	}
	for _, spec := range mustScheduledAgents() {
		if len(spec.Tools) == 0 {
			t.Errorf("agent %q names no tools, so the runner refuses its every job — a run attaches "+
				"the tools its goal needs", spec.Name)
			continue
		}
		if len(spec.Tools) >= len(registered) {
			t.Errorf("agent %q attaches %d tools against the %d this build serves — a run attaches what "+
				"its goal needs and is never offered the whole catalog", spec.Name, len(spec.Tools), len(registered))
		}
		seen := map[string]bool{}
		for _, name := range spec.Tools {
			switch {
			case !registered[name]:
				t.Errorf("agent %q names tool %q, which no registered tool answers to — "+
					"the agent silently loses it, and its goal with it", spec.Name, name)
			case seen[name]:
				t.Errorf("agent %q names tool %q twice; the allowlist is a set", spec.Name, name)
			}
			seen[name] = true
		}
	}
}

// The allowlist only binds a run if the job CARRIES it, and nothing in the type
// system says it must: Job.Tools is an ordinary field, and a call site that
// forgets it builds a job the runner refuses — or, set by hand, a job offered
// whatever that call site typed.
//
// So the obligation is derived from the source: every runner.Job built in this
// package's production files sets Tools from a runner.AgentSpec's own Tools
// field. Every file, not a named one: the run, the resume and the certification
// case each build a Job, and a census that read one file would pass while a
// second builder drifted. Both halves are resolved by TYPE, so an import alias
// cannot hide a literal and a look-alike `fixture.Tools` cannot stand in for
// the allowlist.
func TestEveryRunnerJobBuiltHereCarriesAnAllowlist(t *testing.T) {
	checked := typeCheckedComposeSources(t)
	found := 0
	for _, file := range checked.files {
		builds, findings := runnerJobAllowlistFindings(checked, file)
		found += builds
		for _, finding := range findings {
			t.Error(finding)
		}
	}
	// The run, the resume and the certification case are the three builders;
	// fewer means the scan is reading the wrong files.
	if found < 3 {
		t.Fatalf("found %d runner.Job literals in this package — this gate is reading the wrong files, "+
			"which is worse than not having it", found)
	}
}

// The census above only means something if it can say no, including to the two
// shapes a name-matching scan waves through: another struct's Tools field, and
// a Job spelled through an aliased import.
func TestTheAllowlistCensusNamesAJobThatCarriesNone(t *testing.T) {
	for _, tc := range []struct {
		name, alias, body string
		named             bool
	}{
		{"no Tools", "runner", `func f(spec runner.AgentSpec) runner.Job { return runner.Job{Goal: spec.Goal} }`, true},
		{"a hand-typed list", "runner", `func f() runner.Job { return runner.Job{Tools: []string{"read_record"}} }`, true},
		{"another struct's Tools", "runner", `type fixture struct{ Tools []string }
func f(spec fixture) runner.Job { return runner.Job{Tools: spec.Tools} }`, true},
		{"another Job's Tools", "runner", `func f(prior runner.Job) runner.Job { return runner.Job{Tools: prior.Tools} }`, true},
		{"an aliased import", "agents", `func f() agents.Job { return agents.Job{Goal: "x"} }`, true},
		{"the entry's allowlist", "runner", `func f(spec runner.AgentSpec) runner.Job { return runner.Job{Tools: spec.Tools} }`, false},
		{"the allowlist through a pointer", "agents", `func f(spec *agents.AgentSpec) *agents.Job { return &agents.Job{Tools: (spec.Tools)} }`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := fmt.Sprintf("package planted\nimport %s %q\n%s\n", tc.alias, runnerPackagePath, tc.body)
			planted := typeCheckPlanted(t, src)
			builds, findings := runnerJobAllowlistFindings(planted, planted.files[0])
			if builds != 1 {
				t.Fatalf("the census saw %d runner.Job literals in the planted file, want 1", builds)
			}
			if named := len(findings) == 1; named != tc.named {
				t.Errorf("findings %q, want the literal named=%v", findings, tc.named)
			}
		})
	}
}

// runnerJobAllowlistFindings counts the runner.Job literals in one checked file
// and names each that does not take its Tools from a runner.AgentSpec's own
// Tools field.
func runnerJobAllowlistFindings(checked *typeCheckedSources, file *ast.File) (builds int, findings []string) {
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok || !isRunnerNamed(checked.info.TypeOf(lit), "Job") {
			return true
		}
		builds++
		if finding := jobToolsFinding(checked, lit); finding != "" {
			findings = append(findings, finding)
		}
		return true
	})
	return builds, findings
}

// jobToolsFinding names a Job literal whose Tools is absent or is not read off
// an AgentSpec. PRESENT IS NOT ENOUGH: `Tools: nil` satisfies "the field is
// set", and a hand-typed list is a second allowlist nobody declared.
func jobToolsFinding(checked *typeCheckedSources, lit *ast.CompositeLit) string {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if ident, ok := kv.Key.(*ast.Ident); !ok || ident.Name != "Tools" {
			continue
		}
		if !readsAgentSpecTools(checked.info, kv.Value) {
			return fmt.Sprintf("the runner.Job at %s sets Tools to something that is not a "+
				"runner.AgentSpec's own allowlist", checked.fset.Position(kv.Pos()))
		}
		return ""
	}
	return fmt.Sprintf("the runner.Job built at %s sets no Tools, so the runner refuses it and the "+
		"agent's declared allowlist binds nothing", checked.fset.Position(lit.Pos()))
}

// readsAgentSpecTools reports whether an expression is a selection of the Tools
// field declared on runner.AgentSpec. The receiver's NAME is deliberately not
// pinned — an honest rename of `spec` must not fail — but the field it resolves
// to is, so any other struct's Tools is refused however it is spelled.
func readsAgentSpecTools(info *types.Info, expr ast.Expr) bool {
	sel, ok := ast.Unparen(expr).(*ast.SelectorExpr)
	if !ok {
		return false
	}
	selection, ok := info.Selections[sel]
	if !ok || selection.Kind() != types.FieldVal {
		return false
	}
	field := selection.Obj()
	return field.Name() == "Tools" && field.Pkg() != nil && field.Pkg().Path() == runnerPackagePath &&
		declaresField(field.Pkg().Scope().Lookup("AgentSpec"), field)
}

// declaresField reports whether obj is a struct type declaring field directly.
func declaresField(obj types.Object, field types.Object) bool {
	if obj == nil {
		return false
	}
	st, ok := obj.Type().Underlying().(*types.Struct)
	if !ok {
		return false
	}
	for each := range st.Fields() {
		if each == field {
			return true
		}
	}
	return false
}

// isRunnerNamed reports whether t is the runner package's named type called
// name, through any alias or pointer.
func isRunnerNamed(t types.Type, name string) bool {
	if t == nil {
		return false
	}
	if ptr, ok := types.Unalias(t).(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := types.Unalias(t).(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == runnerPackagePath &&
		named.Obj().Name() == name
}

// runnerPackagePath is derived from the type rather than typed, so a move of the
// runner package moves the census with it.
var runnerPackagePath = reflect.TypeFor[runner.Job]().PkgPath()

// typeCheckedSources is parsed source with the type information the census
// resolves both halves through.
type typeCheckedSources struct {
	fset  *token.FileSet
	files []*ast.File
	info  *types.Info
}

// composeLoad is what type-checking this package needs from the go command: its
// production file list, honouring build tags, and an importer reading the export
// data its imports already compiled to for this test binary.
type composeLoad struct {
	fset     *token.FileSet
	goFiles  []string
	importer types.Importer
}

// loadCompose runs once per test binary; the importer caches what it reads, so
// the planted fixtures resolve runner to the same package the real files do.
var loadCompose = sync.OnceValues(func() (*composeLoad, error) {
	out, err := goList("-json=GoFiles,Imports", ".")
	if err != nil {
		return nil, err
	}
	var listed struct{ GoFiles, Imports []string }
	if err := json.Unmarshal(out, &listed); err != nil {
		return nil, fmt.Errorf("decoding go list for this package: %w", err)
	}
	out, err = goList(append([]string{"-export", "-deps", "-f", "{{if .Export}}{{.ImportPath}}\t{{.Export}}{{end}}"},
		listed.Imports...)...)
	if err != nil {
		return nil, err
	}
	exports := map[string]string{}
	for line := range strings.Lines(string(out)) {
		if path, file, ok := strings.Cut(strings.TrimSpace(line), "\t"); ok {
			exports[path] = file
		}
	}
	fset := token.NewFileSet()
	imp := importer.ForCompiler(fset, "gc", func(path string) (io.ReadCloser, error) {
		file, ok := exports[path]
		if !ok {
			return nil, fmt.Errorf("go list reported no export data for %s", path)
		}
		return os.Open(file)
	})
	return &composeLoad{fset: fset, goFiles: listed.GoFiles, importer: imp}, nil
})

func goList(args ...string) ([]byte, error) {
	cmd := exec.Command("go", append([]string{"list"}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list %s: %w\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return out, nil
}

// typeCheckedComposeSources type-checks this package's production files.
func typeCheckedComposeSources(t *testing.T) *typeCheckedSources {
	t.Helper()
	load, err := loadCompose()
	if err != nil {
		t.Fatalf("loading this package for type-checking: %v", err)
	}
	files := make([]*ast.File, 0, len(load.goFiles))
	for _, name := range load.goFiles {
		parsed, err := parser.ParseFile(load.fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		files = append(files, parsed)
	}
	return typeCheck(t, load, "compose", files)
}

// typeCheckPlanted type-checks one planted file against the same imports.
func typeCheckPlanted(t *testing.T, src string) *typeCheckedSources {
	t.Helper()
	load, err := loadCompose()
	if err != nil {
		t.Fatalf("loading this package for type-checking: %v", err)
	}
	parsed, err := parser.ParseFile(load.fset, "planted.go", src, 0)
	if err != nil {
		t.Fatalf("parse the planted file: %v", err)
	}
	return typeCheck(t, load, "planted", []*ast.File{parsed})
}

func typeCheck(t *testing.T, load *composeLoad, path string, files []*ast.File) *typeCheckedSources {
	t.Helper()
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Selections: map[*ast.SelectorExpr]*types.Selection{}}
	if _, err := (&types.Config{Importer: load.importer}).Check(path, load.fset, files, info); err != nil {
		// A file the checker cannot resolve is a file the census cannot clear.
		t.Fatalf("type-checking %s: %v", path, err)
	}
	return &typeCheckedSources{fset: load.fset, files: files, info: info}
}

// The two shipped agents are the reason the allowlist exists, so the property
// that motivated it is asserted rather than left to the reader: what each goal
// needs is a strict subset of what its scopes admit, and the gap is the verbs
// nothing but the entry can withhold.
func TestTheShippedAgentsAreNarrowerThanTheirScopesAllow(t *testing.T) {
	specs := NewRegistry(nil, SendPath{}).Specs()
	byName := map[string]string{}
	for _, spec := range specs {
		byName[spec.Name] = string(spec.RequiredScope)
	}
	for _, spec := range mustScheduledAgents() {
		needed := map[string]bool{}
		for _, name := range spec.Tools {
			needed[byName[name]] = true
		}
		var admitted, withheld []string
		for _, tool := range specs {
			if !needed[string(tool.RequiredScope)] {
				continue
			}
			admitted = append(admitted, tool.Name)
			if !containsName(spec.Tools, tool.Name) {
				withheld = append(withheld, tool.Name)
			}
		}
		if len(withheld) == 0 {
			t.Errorf("agent %q withholds nothing its scopes admit (%d tools) — either the entry is "+
				"redundant or it has drifted into naming everything", spec.Name, len(admitted))
			continue
		}
		t.Logf("agent %-24s names %d of the %d tools its scopes admit; withholds %s",
			spec.Name, len(spec.Tools), len(admitted), strings.Join(withheld, ", "))
	}
}

func containsName(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}

// The wire declares an `awaiting_approval` state, the store puts it in the
// running list, and agent_run's CHECK admits it — but no locale carries a word
// for it, on the argument that the shipped catalog cannot stage a confirmation.
// That argument is a property of the catalog, not of the reader, and the day an
// entry gains a 🟡 tool the orb turns "working" with nothing said anywhere. So
// the property is asserted rather than argued: every tool every spec names
// resolves to the auto-execute tier.
//
// TierDynamic fails too, and deliberately: a resolver may only ever RAISE to
// confirmation_required, so a dynamic tool is a tool that can suspend a run.
func TestEveryScheduledAgentsToolsAreAutoExecute(t *testing.T) {
	tiers := map[string]mcp.RiskTier{}
	for _, spec := range NewRegistry(nil, SendPath{}).Specs() {
		tiers[spec.Name] = spec.Tier
	}
	for _, spec := range mustScheduledAgents() {
		for _, name := range spec.Tools {
			tier, registered := tiers[name]
			if !registered {
				// Named-but-unregistered is TestEveryAgentSpecNamesRegisteredTools'
				// finding; reporting it twice buries the tier failure.
				continue
			}
			if tier != mcp.TierAutoExecute {
				t.Errorf("agent %q may call %q, which is not auto-execute — the run can suspend on a "+
					"staged approval, and the AI-activity projection reports a suspended run as "+
					"`running`. The rail would then tell the reader the AI is working while it is "+
					"waiting for THEM, and the approvals inbox is the only surface that says otherwise. "+
					"Either drop the tool from the entry, or give a suspended run its own state on the "+
					"projection and ship the copy in en/de/vi first",
					spec.Name, name)
			}
		}
	}
}
