// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
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

	registry := NewRegistry(nil, SendPath{})
	specs := registry.Specs()
	if len(specs) == 0 {
		t.Fatal("the registry has no tools — this sweep checked nothing")
	}
	extensionTools := composedToolNames()
	sort.Slice(specs, func(i, j int) bool { return specs[i].Name < specs[j].Name })
	for _, spec := range specs {
		if extensionTools[spec.Name] || spec.ProtocolOnly {
			continue
		}
		// A router runs as its targets, each held to its own route in this sweep.
		if _, isRouter := registry.RouteTargets(spec.Name); isRouter {
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
				verbOrNone(agentPolicies[op.method+" "+op.route].Tool)+", not "+tool+", so it "+
				"serves some other answer. Run the tool's engine on a route that declares it.")
		}
	}
	return problems
}

// A registry-served route answers the tool's own JSON, which the shape gate
// holds. A route that keeps its own shape (GET /attachments behind
// list_documents) can still drift from the tool's records. So some both-doors
// integration test calls that tool through twoDoors. The census sees the call;
// review holds that the test compares what the two doors return.
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
		for _, name := range toolsAskedOnBothDoors(string(src)) {
			called[name] = true
		}
	}

	extensionTools := composedToolNames()
	ownShape := 0
	for _, spec := range NewRegistry(nil, SendPath{}).Specs() {
		if extensionTools[spec.Name] || !keepsItsOwnShape(spec.OpenAPIOp, byID) {
			continue
		}
		ownShape++
		if !called[spec.Name] {
			t.Errorf("%s runs on %s, which answers in its own shape, and no integration test calls "+
				"it through twoDoors", spec.Name, spec.OpenAPIOp)
		}
	}
	if ownShape == 0 {
		t.Fatal("no tool runs on a route of its own shape — the census found nothing to hold")
	}
}

// bothDoorsToolCall finds a tool asked through the tool door of twoDoors: asked
// to run (`tool`) or asked where the door must refuse or stage (`toolRefused`).
// A seeding call is spelled `toolSeed`, which this does not match.
var bothDoorsToolCall = regexp.MustCompile(`\.tool(?:Refused)?\(t, "([a-z0-9_]+)"`)

// bothDoorsComparison finds a call that compares what two doors returned or
// left. A file that has none compares nothing, so its tool calls prove nothing.
var bothDoorsComparison = regexp.MustCompile(`\b(?:sameEffect|sameStaged|sameStored|sameIDs|sameRanking|sameDoorRecords|bothDoorsName)\(|sameRecords\[`)

// toolsAskedOnBothDoors names the tools a test file asks through the tool door,
// when the file also compares the two doors. A call in a comment is no call.
func toolsAskedOnBothDoors(src string) []string {
	var code []string
	for line := range strings.SplitSeq(src, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "//") {
			code = append(code, line)
		}
	}
	text := strings.Join(code, "\n")
	if !bothDoorsComparison.MatchString(text) {
		return nil
	}
	var names []string
	for _, m := range bothDoorsToolCall.FindAllStringSubmatch(text, -1) {
		names = append(names, m[1])
	}
	return names
}

func TestTheBothDoorsCensusCountsOnlyCallsInATestThatCompares(t *testing.T) {
	compared := "func TestX(t *testing.T) {\n\tsameRecords[T](t, \"x\", d.rest(), d.tool(t, \"read_a\", nil))\n}\n"
	cases := map[string]struct {
		src  string
		want []string
	}{
		"a call beside a comparison":             {compared, []string{"read_a"}},
		"a refusal beside a comparison":          {"d.toolRefused(t, \"merge_tags\", nil)\nd.sameStaged(t)\n", []string{"merge_tags"}},
		"a call in a comment":                    {"// d.tool(t, \"read_b\", nil)\nd.sameEffect(t)\n", nil},
		"a call in a file that compares nothing": {"d.tool(t, \"read_c\", nil)\n", nil},
		"a seeding call":                         {"d.toolSeed(t, \"create_record\", nil)\nd.sameEffect(t)\n", nil},
	}
	for name, c := range cases {
		if got := toolsAskedOnBothDoors(c.src); !slices.Equal(got, c.want) {
			t.Errorf("%s: the census reads %v, want %v", name, got, c.want)
		}
	}
}

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

func verbOrNone(verb string) string {
	if verb == "" {
		return "(none)"
	}
	return verb
}
