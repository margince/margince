// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
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
