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
// other annotation value. The vocabulary refuses it, not the derivation: an
// unknown value read as unmarked would gate a route meant for the registry
// twice.
func TestAnUndeclaredServedByFailsGeneration(t *testing.T) {
	policies, derived := policiesOf(t, strings.ReplaceAll(slippingPaths, "%s", "handler"))
	if len(derived) != 0 {
		t.Fatalf("derivation refused the value itself: %v", derived)
	}
	vocab := vocabulary{
		access: []string{"tool"}, tier: []string{"auto_execute"}, scope: []string{"read"}, servedBy: []string{"registry"},
	}
	defects := vocab.violations(policies)
	if len(defects) != 1 || !strings.Contains(defects[0], `x-mcp-tool.served_by = "handler"`) {
		t.Errorf("defects = %v, want one naming the undeclared served_by", defects)
	}
}

// A read binds a passport as a mutation does, so an unannotated GET is a
// defect. Only an anonymous one, which no agent ever reaches, needs no row.
func TestAnUnannotatedReadFailsGenerationUnlessAnonymous(t *testing.T) {
	policies, defects := policiesOf(t, `
/companies/{id}/facts:
  get:
    operationId: listCompanyFacts
/public/availability:
  get:
    operationId: getPublicAvailability
    security: []
`)
	if len(policies) != 0 {
		t.Errorf("policies = %v, want none", policies)
	}
	if len(defects) != 1 || !strings.Contains(defects[0], "listCompanyFacts") {
		t.Errorf("defects = %v, want one naming listCompanyFacts", defects)
	}
}

// The parity catalog reads x-settings-audience as two words. A third is a defect
// here, since anything but member would put the operation in the admin group.
func TestAnUnknownSettingsAudienceFailsGeneration(t *testing.T) {
	const paths = `
/pipelines:
  get:
    operationId: listPipelines
    x-mcp-tool: { verb: list_pipelines, tier: auto_execute, scope: read }
    x-settings-audience: %s
`
	for audience, defects := range map[string]int{"member": 0, "admin": 0, "everyone": 1} {
		if _, got := policiesOf(t, strings.ReplaceAll(paths, "%s", audience)); len(got) != defects {
			t.Errorf("x-settings-audience: %s gave defects %v, want %d", audience, got, defects)
		}
	}
}

// x-agent-access and x-mcp-tool state opposite answers, so one operation may not
// declare both.
func TestAnOperationCannotBeBothHumanOnlyAndATool(t *testing.T) {
	const paths = `
/pipelines:
  get:
    operationId: listPipelines
    x-mcp-tool: { verb: list_pipelines, tier: auto_execute, scope: read }
    x-agent-access: human-only
    x-human-only-reason: pending
`
	if _, defects := policiesOf(t, paths); len(defects) != 1 || !strings.Contains(defects[0], "listPipelines") {
		t.Errorf("defects = %v, want one naming listPipelines", defects)
	}
}
