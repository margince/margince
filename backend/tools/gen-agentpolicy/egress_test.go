// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"strings"
	"testing"
)

// `egress: false` has one reader. It lowers a send or enrich cap, and only on a
// route-backed verb, the one kind of tool whose egress the contract sets.
func TestTheEgressFlagIsReadOnlyWhereItHasAReader(t *testing.T) {
	for _, c := range []struct {
		name, scope, servedBy, egress string
		refused                       bool
	}{
		{name: "a route verb that sends nothing", scope: "send", servedBy: ", served_by: route", egress: "false"},
		{name: "a route verb repeating its scope", scope: "send", servedBy: ", served_by: route", egress: "true", refused: true},
		{name: "a route verb on a cap that never leaves", scope: "write", servedBy: ", served_by: route", egress: "false", refused: true},
		{name: "a registry tool, which states its own", scope: "enrich", servedBy: ", served_by: registry", egress: "false", refused: true},
		{name: "a tool the handler runs", scope: "send", egress: "false", refused: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			policies, defects := policiesOf(t, fmt.Sprintf(`
/brief:
  post:
    operationId: refreshBrief
    x-mcp-tool: { verb: refresh_brief, tier: auto_execute, scope: %s%s, egress: %s }
`, c.scope, c.servedBy, c.egress))
			refusedHere := len(defects) == 1 && strings.Contains(defects[0], "egress")
			if refusedHere != c.refused {
				t.Fatalf("defects %v, want refused=%t", defects, c.refused)
			}
			if !c.refused && (len(policies) != 1 || !policies[0].StaysInside) {
				t.Fatalf("policies %+v, want one that stays inside", policies)
			}
		})
	}
}
