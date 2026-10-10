// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func policiesOf(t *testing.T, paths string) ([]policy, []string) {
	t.Helper()
	var doc map[string]map[string]yaml.Node
	if err := yaml.Unmarshal([]byte(paths), &doc); err != nil {
		t.Fatalf("parsing the fixture paths: %v", err)
	}
	return derivePolicies(doc)
}

const slippingPaths = `
/deals/slipping:
  get:
    operationId: listSlippingDeals
    x-mcp-tool: { verb: whats_slipping_this_week, tier: auto_execute, scope: read, served_by: %s }
/deals:
  get:
    operationId: listDeals
    x-mcp-tool: { verb: list_records, tier: auto_execute, scope: read }
`

// served_by reaches the table on the operation that declares it and nowhere
// else, and the emitted row names it.
func TestServedByReachesTheTableOnlyWhereDeclared(t *testing.T) {
	policies, defects := policiesOf(t, strings.ReplaceAll(slippingPaths, "%s", "registry"))
	if len(defects) != 0 {
		t.Fatalf("defects %v", defects)
	}
	got := map[string]string{}
	for _, p := range policies {
		got[p.Op] = p.ServedBy
	}
	if got["listSlippingDeals"] != "registry" || got["listDeals"] != "" {
		t.Fatalf("served_by per operation = %v, want registry on listSlippingDeals alone", got)
	}
	table := renderTable(policies, vocabulary{servedBy: []string{"registry"}})
	if !strings.Contains(table, `Scope: "read", ServedBy: "registry"},`) {
		t.Errorf("the emitted table carries no ServedBy for the marked route:\n%s", table)
	}
}

// A served_by value the contract does not declare fails generation, like every
// other annotation value. A misspelling must not leave the gate admitting a
// call the registry admits again.
func TestAnUndeclaredServedByFailsGeneration(t *testing.T) {
	policies, _ := policiesOf(t, strings.ReplaceAll(slippingPaths, "%s", "registery"))
	vocab := vocabulary{
		access: []string{"tool"}, tier: []string{"auto_execute"}, scope: []string{"read"}, servedBy: []string{"registry"},
	}
	defects := vocab.violations(policies)
	if len(defects) != 1 || !strings.Contains(defects[0], `x-mcp-tool.served_by = "registery"`) {
		t.Errorf("defects = %v, want one naming the undeclared served_by", defects)
	}
}
