// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// modelVerbsOffEnrich names the agent verbs marked always that spend another scope,
// and the source that shows why. The two reads call ai.ReadLane, so an agent without
// enrich gets the deterministic floor and no model call. draft_email has its own
// scope, which a passport holds to draft and nothing more.
var modelVerbsOffEnrich = map[string]struct{ scope, file, proof string }{
	"getMeetingBrief":  {"read", "internal/compose/meetingbrief/write.go", "ai.ReadLane("},
	"getDealRoomBrief": {"read", "internal/compose/roombrief/service.go", "ai.ReadLane("},
	"draftEmail":       {"draft", "internal/modules/agents/tools_comms.go", `Name: "draft_email"`},
}

// AI work an agent starts spends enrich. The census is the contract's own marker.
// A verb that always waits on a model on another scope spends budget nobody lent it.
func TestAnAgentVerbThatAlwaysCallsAModelSpendsEnrich(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(modelRouteContract)
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	var doc struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	spending := 0
	seen := map[string]bool{}
	for path, item := range doc.Paths {
		for method, entry := range item {
			operation, isOperation := entry.(map[string]any)
			if !isOperation || operation[modelRouteMarker] != "always" {
				continue
			}
			tool, offered := operation["x-mcp-tool"].(map[string]any)
			if !offered {
				continue
			}
			route := strings.ToUpper(method) + " " + path
			op, _ := operation["operationId"].(string)
			if tool["scope"] == "enrich" {
				spending++
				continue
			}
			seen[op] = true
			named, waived := modelVerbsOffEnrich[op]
			if !waived {
				t.Errorf("%s (%s) always calls a model and an agent reaches it on scope %v, not enrich; "+
					"move it to scope enrich, on a router that spends enrich", route, op, tool["scope"])
				continue
			}
			if tool["scope"] != named.scope {
				t.Errorf("%s spends %v, and its reason here holds only for scope %s", op, tool["scope"], named.scope)
				continue
			}
			source, err := os.ReadFile(named.file)
			if err != nil {
				t.Errorf("%s: reading why it spends no enrich: %v", op, err)
				continue
			}
			if !strings.Contains(string(source), named.proof) {
				t.Errorf("%s spends no enrich because %s holds %q, and it no longer does", op, named.file, named.proof)
			}
		}
	}
	if spending == 0 {
		t.Error("no verb spends enrich, so this census read too little of the contract")
	}
	for op := range modelVerbsOffEnrich {
		if !seen[op] {
			t.Errorf("%s is named here and the contract no longer offers it as a verb that always calls a model; remove it", op)
		}
	}
}
