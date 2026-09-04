// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A credential that does not fund the tools its agent declares buys a run that
// starts, discovers it cannot do its job, and stops.
//
// The failure is silent by construction. The runner DEGRADES a run whose
// declared tool is not admitted by its passport rather than refusing to start,
// which is right — one unfunded tool should not cost the whole night — but it
// means an under-scoped credential produces a nightly agent that wakes up,
// reads what it can, and never logs the note it exists to log. Nothing errors.
// The rep simply finds nothing was done, at 2am, with nobody watching.
//
// So the obligation is derived rather than restated: for every scheduled agent,
// the scopes the grant mints must cover the RequiredScope of every tool that
// agent's contract entry names. Both halves are read from source — the tool
// list from api/ai-tasks.yaml, each tool's scope from its own ToolSpec — so the
// map in handlers_agentgrants.go cannot drift from what the agents call.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestEveryGrantFundsTheToolsItsAgentDeclares(t *testing.T) {
	t.Parallel()
	granted := grantedScopes(t)
	required := toolScopes(t)
	declared := agentToolLists(t)

	if len(granted) == 0 || len(required) == 0 || len(declared) == 0 {
		t.Fatalf("this gate proves nothing unless it can read all three sources: "+
			"%d grant(s), %d tool scope(s), %d agent tool list(s) — one of the "+
			"scans has stopped matching and is reporting an empty world as agreement",
			len(granted), len(required), len(declared))
	}

	for agent, tools := range declared {
		scopes, offered := granted[agent]
		if !offered {
			// Not every scheduled agent has to be grantable, but one that is not
			// can never run: the fan-out seeds only against a live grant.
			t.Errorf("agent %q declares tools but no grant mints a credential for it — "+
				"nothing can authorize its nightly run", agent)
			continue
		}
		for _, tool := range tools {
			need, known := required[tool]
			if !known {
				// A tool named in the contract with no ToolSpec in the tree is a
				// separate defect, and the census that owns it will say so.
				continue
			}
			if !scopes[need] {
				t.Errorf("agent %q declares %s, which requires the %q scope, but its "+
					"grant mints %v — the run degrades that tool away and the agent "+
					"quietly does not do the work it exists for",
					agent, tool, need, sortedKeys(scopes))
			}
		}
	}
}

// grantedScopes reads the per-agent scope map the grant handler mints from.
func grantedScopes(t *testing.T) map[string]map[string]bool {
	t.Helper()
	src := readFile(t, filepath.Join("internal", "modules", "identity", "handlers_agentgrants.go"))
	out := map[string]map[string]bool{}
	// The map literal's entries: "agent": {"scope", "scope"}.
	entry := regexp.MustCompile(`"([a-z_]+)":\s*\{([^}]*)\}`)
	body := between(src, "var grantScopes = map[string][]string{", "\n}")
	for _, m := range entry.FindAllStringSubmatch(body, -1) {
		scopes := map[string]bool{}
		for _, raw := range strings.Split(m[2], ",") {
			if s := strings.Trim(strings.TrimSpace(raw), `"`); s != "" {
				scopes[s] = true
			}
		}
		out[m[1]] = scopes
	}
	return out
}

// agentToolLists reads each scheduled agent's declared tools from the contract
// that owns them.
func agentToolLists(t *testing.T) map[string][]string {
	t.Helper()
	src := readFile(t, filepath.Join("api", "ai-tasks.yaml"))
	body := between(src, "      morning_brief:", "    company_context:")
	out := map[string][]string{}
	var current string
	for _, line := range strings.Split("      morning_brief:"+body, "\n") {
		trimmed := strings.TrimSpace(line)
		if name := strings.TrimSuffix(trimmed, ":"); name != trimmed && !strings.Contains(trimmed, " ") {
			current = name
			continue
		}
		if current == "" {
			continue
		}
		// `tools: [a, b,` and its continuation lines both contribute names.
		if idx := strings.Index(trimmed, "tools:"); idx >= 0 {
			trimmed = trimmed[idx+len("tools:"):]
		}
		for _, raw := range strings.Split(strings.Trim(trimmed, "[]"), ",") {
			if name := strings.TrimSpace(raw); name != "" && !strings.Contains(name, ":") {
				out[current] = append(out[current], strings.Trim(name, "[]"))
			}
		}
	}
	return out
}

// toolScopes reads every ToolSpec's name and RequiredScope out of the agents
// package, so the requirement comes from the tool rather than from a list.
func toolScopes(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	dir := filepath.Join("internal", "modules", "agents")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the agents package: %v", err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", e.Name(), err)
		}
		ast.Inspect(parsed, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			name, scope := specNameAndScope(lit)
			if name != "" && scope != "" {
				out[name] = scope
			}
			return true
		})
	}
	return out
}

// specNameAndScope pulls Name and RequiredScope out of one composite literal,
// empty when it is not a ToolSpec.
func specNameAndScope(lit *ast.CompositeLit) (name, scope string) {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch key.Name {
		case "Name":
			if s, ok := kv.Value.(*ast.BasicLit); ok {
				name = strings.Trim(s.Value, `"`)
			}
		case "RequiredScope":
			// principal.ScopeWrite → "write".
			if sel, ok := kv.Value.(*ast.SelectorExpr); ok {
				scope = strings.ToLower(strings.TrimPrefix(sel.Sel.Name, "Scope"))
			}
		}
	}
	return name, scope
}

// between returns the text after the first `from` and before the next `to`,
// empty when either marker is gone — which the caller's emptiness check turns
// into a failure rather than a silent pass.
func between(src, from, to string) string {
	start := strings.Index(src, from)
	if start < 0 {
		return ""
	}
	rest := src[start+len(from):]
	if end := strings.Index(rest, to); end >= 0 {
		return rest[:end]
	}
	return rest
}

// sortedKeys is the set's keys in a stable order.
//
// It really does sort now. Both callers put the result in a FAILURE MESSAGE,
// and map iteration order is randomised per run — so the same finding read
// differently every time, which is how somebody comparing two runs concludes
// the tree moved when only the map did.
func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
