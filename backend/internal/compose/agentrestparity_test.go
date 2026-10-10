// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// A passport reaches the same features over REST as over MCP, with the same
// answer. So every tool names, in OpenAPIOp, the operations it runs as. A
// passport can call each one, and each declares this tool as its x-mcp-tool
// verb. A route that only holds related data (listDeals behind a slipping-deals
// answer) declares some other verb, and fails here. No waiver map: a tool whose
// answer no route gives needs that route.
func TestEveryToolRunsOnARouteThatDeclaresIt(t *testing.T) {
	ops, defaultSecurity := passportOperations(t)
	if len(ops) == 0 {
		t.Fatal("the contract walk found no operations — this sweep checked nothing")
	}
	byID := map[string]passportOperation{}
	for _, op := range ops {
		if op.security == nil {
			op.security = defaultSecurity
		}
		byID[op.id] = op
	}

	specs := NewRegistry(nil, SendPath{}).Specs()
	if len(specs) == 0 {
		t.Fatal("the registry has no tools — this sweep checked nothing")
	}
	extensionTools := composedToolNames()
	sort.Slice(specs, func(i, j int) bool { return specs[i].Name < specs[j].Name })
	for _, spec := range specs {
		if extensionTools[spec.Name] {
			continue
		}
		for _, problem := range restParityProblems(spec.Name, spec.OpenAPIOp, byID) {
			t.Errorf("%s: %s", spec.Name, problem)
		}
	}
}

// restParityProblems reads OpenAPIOp as operationIds separated by "/", one per
// record type the tool serves.
func restParityProblems(tool, openAPIOp string, byID map[string]passportOperation) []string {
	if strings.TrimSpace(openAPIOp) == "" {
		return []string{"OpenAPIOp names no operation, so a passport on REST has no route to this " +
			"answer. Add the route to api/crm.yaml with x-mcp-tool verb " + tool + "."}
	}
	var problems []string
	for id := range strings.SplitSeq(openAPIOp, "/") {
		op, known := byID[id]
		switch {
		case !known:
			problems = append(problems, "OpenAPIOp "+openAPIOp+" names "+id+", which is not an "+
				"operationId in api/crm.yaml. List operationIds separated by \"/\", one route per answer.")
		case !passportCalls(op):
			problems = append(problems, id+" refuses a passport (human-only, or its security has "+
				"no bearerAuth), so REST cannot do what this tool does.")
		case agentPolicies[op.method+" "+op.route].Tool != tool:
			problems = append(problems, id+" declares x-mcp-tool verb "+
				quotedOrNone(agentPolicies[op.method+" "+op.route].Tool)+", not "+tool+", so it "+
				"serves some other answer. Run the tool's engine on a route that declares it.")
		}
	}
	return problems
}

// A registry-served route answers the tool's own JSON, which the shape gate
// holds. A route that keeps its own shape (GET /attachments behind
// list_documents) can still drift from the tool's records, so some both-doors
// integration test must call that tool through twoDoors and compare the two.
func TestEveryToolOnARouteOfItsOwnShapeHasABothDoorsTest(t *testing.T) {
	ops, _ := passportOperations(t)
	byID := map[string]passportOperation{}
	for _, op := range ops {
		byID[op.id] = op
	}
	sources, err := filepath.Glob("integration/*_test.go")
	if err != nil || len(sources) == 0 {
		t.Fatalf("no integration test sources found (%v) — this sweep checked nothing", err)
	}
	called := map[string]bool{}
	for _, path := range sources {
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("reading %s: %v", path, readErr)
		}
		for _, m := range bothDoorsToolCall.FindAllStringSubmatch(string(src), -1) {
			called[m[1]] = true
		}
	}

	untested := untestedDoorsBaseline(t)
	extensionTools := composedToolNames()
	ownShape := 0
	for _, spec := range NewRegistry(nil, SendPath{}).Specs() {
		if extensionTools[spec.Name] || !keepsItsOwnShape(spec.OpenAPIOp, byID) {
			continue
		}
		ownShape++
		listed := untested[spec.Name]
		delete(untested, spec.Name)
		switch {
		case !called[spec.Name] && !listed:
			t.Errorf("%s runs on %s, which answers in its own shape, and no integration test calls "+
				"it through twoDoors to compare the records both doors give", spec.Name, spec.OpenAPIOp)
		case called[spec.Name] && listed:
			t.Errorf("%s now has a both-doors test; remove it from %s", spec.Name, untestedDoorsPath)
		}
	}
	if ownShape == 0 {
		t.Fatal("no tool runs on a route of its own shape — the census found nothing to hold")
	}
	for name := range untested {
		t.Errorf("%s is in %s but is no tool on a route of its own shape; remove it", name, untestedDoorsPath)
	}
}

// untestedDoorsPath lists the tools that ran on a route of their own shape
// before this gate existed and still lack a both-doors test. It only shrinks.
const untestedDoorsPath = "testdata/untested_tool_doors.txt"

func untestedDoorsBaseline(t *testing.T) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(untestedDoorsPath)
	if err != nil {
		t.Fatalf("reading %s: %v", untestedDoorsPath, err)
	}
	names := map[string]bool{}
	for line := range strings.SplitSeq(string(src), "\n") {
		if name := strings.TrimSpace(line); name != "" && !strings.HasPrefix(name, "#") {
			names[name] = true
		}
	}
	return names
}

var bothDoorsToolCall = regexp.MustCompile(`\.tool\(t, "([a-z0-9_]+)"`)

// keepsItsOwnShape reports a tool any of whose operations is not served by the
// registry.
func keepsItsOwnShape(openAPIOp string, byID map[string]passportOperation) bool {
	for id := range strings.SplitSeq(openAPIOp, "/") {
		op, known := byID[id]
		if known && agentPolicies[op.method+" "+op.route].ServedBy != servedByRegistry {
			return true
		}
	}
	return false
}

func passportCalls(op passportOperation) bool {
	_, admitted := routeAdmitsAgents(op.method, op.route)
	return admitted && acceptsPassport(op.security)
}

func quotedOrNone(verb string) string {
	if verb == "" {
		return "(none)"
	}
	return verb
}
