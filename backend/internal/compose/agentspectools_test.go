// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

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
// package's production files sets Tools from an entry's own allowlist. Every
// file, not a named one: the run, the resume and the certification case each
// build a Job, and a census that read one file would pass while a second
// builder drifted. It is a source read because the property belongs to the
// construction site, and there is no runtime seam to observe it through.
func TestEveryRunnerJobBuiltHereCarriesAnAllowlist(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("listing this package: %v", err)
	}
	fset := token.NewFileSet()
	found := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		builds, findings := runnerJobAllowlistFindings(fset, parsed)
		found += builds
		for _, finding := range findings {
			t.Error(finding)
		}
	}
	// The run and the resume are the two production builders; fewer means the
	// scan is reading the wrong directory.
	if found < 2 {
		t.Fatalf("found %d runner.Job literals in this package — this gate is reading the wrong files, "+
			"which is worse than not having it", found)
	}
}

// The census above only means something if it can say no. A Job with no Tools
// and a Job whose Tools were typed at the call site must both be named.
func TestTheAllowlistCensusNamesAJobThatCarriesNone(t *testing.T) {
	const planted = `package compose
func build(spec runner.AgentSpec) {
	_ = runner.Job{Goal: spec.Goal}
	_ = runner.Job{Goal: spec.Goal, Tools: []string{"read_record"}}
	_ = runner.Job{Goal: spec.Goal, Tools: spec.Tools}
}`
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, "planted.go", planted, 0)
	if err != nil {
		t.Fatalf("parse the planted file: %v", err)
	}
	builds, findings := runnerJobAllowlistFindings(fset, parsed)
	if builds != 3 || len(findings) != 2 {
		t.Errorf("the census saw %d jobs and named %d, want 3 jobs and the 2 without an entry's allowlist: %q",
			builds, len(findings), findings)
	}
}

// runnerJobAllowlistFindings counts the runner.Job literals in one file and names
// each that does not take its Tools from an entry's own allowlist.
func runnerJobAllowlistFindings(fset *token.FileSet, parsed *ast.File) (builds int, findings []string) {
	ast.Inspect(parsed, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok || !isRunnerJob(lit.Type) {
			return true
		}
		builds++
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			if ident, ok := kv.Key.(*ast.Ident); !ok || ident.Name != "Tools" {
				continue
			}
			// PRESENT IS NOT ENOUGH: `Tools: nil` satisfies "the field is set",
			// and a hand-typed list is a second allowlist nobody declared.
			if !isAllowlistFromASpec(kv.Value) {
				findings = append(findings, fmt.Sprintf("the runner.Job at %s sets Tools to something that is "+
					"not an entry's own allowlist", fset.Position(kv.Pos())))
			}
			return true
		}
		findings = append(findings, fmt.Sprintf("the runner.Job built at %s sets no Tools, so the runner "+
			"refuses it and the agent's declared allowlist binds nothing", fset.Position(lit.Pos())))
		return true
	})
	return builds, findings
}

// isAllowlistFromASpec reports whether an expression reads a spec's own Tools —
// `spec.Tools`, or any future receiver's.
//
// It deliberately does NOT pin the receiver's name: what matters is that the
// value comes from a catalog entry rather than being written at the call site. A
// gate that hardcoded `spec` would fail on an honest rename while still passing
// an inline []string{"read_record"} typed out by hand.
func isAllowlistFromASpec(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Tools"
}

// isRunnerJob reports whether a composite literal's type is runner.Job.
func isRunnerJob(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "runner" && sel.Sel.Name == "Job"
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
