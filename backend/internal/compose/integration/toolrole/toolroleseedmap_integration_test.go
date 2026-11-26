// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package toolrole

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/shared/toolroles"
)

const seedMapFile = "seedmap.md"

// verbTakes is, per verb, the kinds of record its call is handed: what a
// counting pool is asked for when the call's arguments are filled.
func verbTakes(t *testing.T) map[string][]string {
	t.Helper()
	takes, err := takesOf(toolRoleCalls(compose.NewRegistry(nil, compose.SendPath{}).Specs(), routeShapes()), func() *recordPool { return countingPool(t) })
	if err != nil {
		t.Fatal(err)
	}
	return takes
}

func takesOf(calls []toolRoleCall, counting func() *recordPool) (map[string][]string, error) {
	out, by := map[string][]string{}, map[string]string{}
	for _, call := range calls {
		if first, taken := by[call.verb]; taken {
			return nil, sharedVerb(call.verb, first, call.row)
		}
		by[call.verb] = call.row
		pool := counting()
		call.args(pool)
		out[call.verb] = slices.Sorted(maps.Keys(pool.wanted))
	}
	return out, nil
}

// Two calls of one verb would share a file, so both writers refuse them. The
// refusal says the verb is shared, not that the sweep is stale.
func TestAVerbTwoCallsServeIsRefused(t *testing.T) {
	calls := []toolRoleCall{{row: "deals.get", tool: "deals", action: "get", verb: "get_deal"}, {row: "records.get", tool: "records", action: "get", verb: "get_deal"}}
	if _, err := takesOf(calls, func() *recordPool { return countingPool(t) }); err == nil || !strings.Contains(err.Error(), "shared by deals.get and records.get") {
		t.Errorf("takes of a shared verb: %v", err)
	}
	doc := toolRolesFileShape{Rows: []toolroles.Row{{Tool: "deals", Action: "get", Verb: "get_deal"}, {Tool: "records", Action: "get", Verb: "get_deal"}}}
	if _, err := doc.published(); err == nil || !strings.Contains(err.Error(), "shared by deals.get and records.get") {
		t.Errorf("rows of a shared verb: %v", err)
	}
}

// seedEntry is one kind's seed as this package's source writes it.
type seedEntry struct {
	kind, at string
	routes   []string
	uses     []string
	sql      bool
}

// seedEntries reads every seed this package declares. A seed is a create[kind]
// assignment, a kind keyed in a map of seed functions, or a kind a batch seed
// makes. It reads the source, so no seed can be left off by hand.
func seedEntries(t *testing.T) map[string][]seedEntry {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]seedEntry{}
	fset := token.NewFileSet()
	for _, name := range files {
		file, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			at := name + " " + fn.Name.Name
			for kind, body := range seedsIn(fn) {
				entry := describeSeed(kind, at, body)
				out[kind] = append(out[kind], entry)
			}
		}
	}
	return out
}

// seedsIn is each kind fn seeds and the code that seeds it.
func seedsIn(fn *ast.FuncDecl) map[string]ast.Node {
	out := map[string]ast.Node{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			for i, lhs := range node.Lhs {
				if kind, ok := createIndex(lhs); ok && i < len(node.Rhs) {
					out[kind] = node.Rhs[i]
				}
			}
		case *ast.CompositeLit:
			if isSeedMap(node.Type) {
				for _, elt := range node.Elts {
					if kv, ok := elt.(*ast.KeyValueExpr); ok {
						if kind, ok := stringLit(kv.Key); ok {
							out[kind] = kv.Value
						}
					}
				}
			}
			for _, kind := range batchKinds(node) {
				out[kind] = fn.Body
			}
		}
		return true
	})
	return out
}

func createIndex(expr ast.Expr) (string, bool) {
	index, ok := expr.(*ast.IndexExpr)
	if !ok {
		return "", false
	}
	if ident, ok := index.X.(*ast.Ident); !ok || ident.Name != "create" {
		return "", false
	}
	return stringLit(index.Index)
}

func isSeedMap(expr ast.Expr) bool {
	m, ok := expr.(*ast.MapType)
	if !ok {
		return false
	}
	fn, ok := m.Value.(*ast.FuncType)
	return ok && (fn.Params == nil || len(fn.Params.List) == 0) && fn.Results != nil && len(fn.Results.List) == 1
}

func batchKinds(lit *ast.CompositeLit) []string {
	var out []string
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if key, isIdent := kv.Key.(*ast.Ident); !isIdent || key.Name != "kinds" {
			continue
		}
		if list, ok := kv.Value.(*ast.CompositeLit); ok {
			for _, item := range list.Elts {
				if kind, ok := stringLit(item); ok {
					out = append(out, kind)
				}
			}
		}
	}
	return out
}

func stringLit(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	text, err := strconv.Unquote(lit.Value)
	return text, err == nil
}

// describeSeed is what one seed's code reaches. That is the routes it names,
// the other kinds it asks for, and whether it writes SQL past the routes.
func describeSeed(kind, at string, body ast.Node) seedEntry {
	entry := seedEntry{kind: kind, at: at}
	routes, uses := map[string]bool{}, map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.BasicLit:
			if text, ok := stringLit(node); ok && strings.HasPrefix(text, "/v1/") {
				routes[text] = true
			}
		case *ast.IndexExpr:
			if other, ok := createIndex(node); ok && other != kind {
				uses[other] = true
			}
		case *ast.SelectorExpr:
			if node.Sel.Name == "Exec" || node.Sel.Name == "ownerExec" {
				entry.sql = true
			}
		}
		return true
	})
	entry.routes, entry.uses = slices.Sorted(maps.Keys(routes)), slices.Sorted(maps.Keys(uses))
	return entry
}

func renderSeedMap(entries map[string][]seedEntry) []byte {
	var b bytes.Buffer
	b.WriteString("# Role sweep seeds\n\n<!-- Generated by TestTheSeedMapIsCurrent; do not edit by hand. -->\n\n" +
		"Each kind of record the role sweep hands a call, the function that seeds it, the routes that code names, the " +
		"other kinds it asks for, and whether it also writes SQL past the routes. A route change that breaks a seed is " +
		"found here by its path. A verb's kinds are in its `takes` in `backend/internal/modules/agents/toolchecks/`.\n\n" +
		"| Kind | Seeded in | Routes | Uses kinds | SQL |\n|---|---|---|---|---|\n")
	for _, kind := range slices.Sorted(maps.Keys(entries)) {
		for _, entry := range entries[kind] {
			fmt.Fprintf(&b, "| `%s` | `%s` | %s | %s | %s |\n", kind, entry.at, codeList(entry.routes), codeList(entry.uses),
				map[bool]string{true: "yes", false: ""}[entry.sql])
		}
	}
	return b.Bytes()
}

func codeList(items []string) string {
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = "`" + item + "`"
	}
	return strings.Join(quoted, " ")
}

// TestTheSeedMapIsCurrent holds seedmap.md to the seeds this package declares,
// so the author of a route change can find the seed it breaks.
func TestTheSeedMapIsCurrent(t *testing.T) {
	entries := seedEntries(t)
	if len(entries) == 0 {
		t.Fatal("no seed was found in this package's source, so the map is empty")
	}
	rendered := renderSeedMap(entries)
	if os.Getenv("UPDATE_MCP_TOOL_ROLES") != "" {
		if err := os.WriteFile(seedMapFile, rendered, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	committed, err := os.ReadFile(seedMapFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(committed, rendered) {
		t.Errorf("%s is not what this package seeds. Run: UPDATE_MCP_TOOL_ROLES=1 make test-it DIR=backend/internal/compose/integration/toolrole RUN=TestTheSeedMapIsCurrent", seedMapFile)
	}
}

// TestEveryVerbTakesWhatTheSeedsMake holds each verb's takes to what its call
// is handed, and every kind a call takes to a seed that makes it.
func TestEveryVerbTakesWhatTheSeedsMake(t *testing.T) {
	takes, seeded := verbTakes(t), seedEntries(t)
	head, checks, err := toolroles.Read[toolroles.Checks](os.DirFS(toolChecksDir))
	if err != nil {
		t.Fatal(err)
	}
	for _, verb := range slices.Sorted(maps.Keys(takes)) {
		for _, kind := range takes[verb] {
			if _, ok := seeded[kind]; !ok {
				t.Errorf("%s takes a %s, and no seed in this package makes one", verb, kind)
			}
		}
	}
	if os.Getenv("UPDATE_MCP_TOOL_ROLES") != "" {
		for verb, row := range checks {
			row.Takes = takes[verb]
			checks[verb] = row
		}
		files, err := toolroles.Render(head, checks)
		if err != nil {
			t.Fatal(err)
		}
		if err := toolroles.Write(toolChecksDir, files); err != nil {
			t.Fatal(err)
		}
		return
	}
	for _, verb := range slices.Sorted(maps.Keys(checks)) {
		if !slices.Equal(checks[verb].Takes, takes[verb]) {
			t.Errorf("%s takes %v and its checks file says %v. Run: UPDATE_MCP_TOOL_ROLES=1 make test-it DIR=backend/internal/compose/integration/toolrole RUN=TestEveryVerbTakesWhatTheSeedsMake",
				verb, takes[verb], checks[verb].Takes)
		}
	}
}
