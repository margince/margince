// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents/apps"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// A choice card that no tool offers is a card nobody is shown, and the tests
// that draw it stay green. Both directions are held here: every view the build
// declares is named by a tool, and every tool a view may ask its host to run is a
// registered, agent-reachable tool the model can also see.

func TestEveryDeclaredCardIsOfferedByATool(t *testing.T) {
	offered := map[string]bool{}
	for _, spec := range NewRegistry(nil, SendPath{}).Specs() {
		if spec.UI != nil {
			offered[spec.UI.ResourceURI] = true
		}
	}
	declared := apps.DeclaredViews()
	if len(declared) == 0 {
		t.Fatal("the build declares no card, so this census checked nothing")
	}
	for uri := range declared {
		if !offered[uri] {
			t.Errorf("%s is declared and published but no tool names it, so no host is ever shown the card", uri)
		}
	}
}

func TestEveryToolACardActsThroughIsRegisteredAndVisibleToTheModel(t *testing.T) {
	actions := apps.ActionTools()
	if len(actions) == 0 {
		t.Fatal("no card acts through any tool, so this census checked nothing")
	}
	reachable := map[string]bool{}
	for _, pol := range agentPolicies {
		if pol.Access == accessTool {
			reachable[pol.Tool] = true
		}
	}
	registered := map[string]mcp.ToolSpec{}
	for _, spec := range NewRegistry(nil, SendPath{}).Specs() {
		registered[spec.Name] = spec
	}
	for name := range actions {
		spec, ok := registered[name]
		if !ok {
			t.Errorf("a card acts through %q, which no tool registers", name)
			continue
		}
		if !reachable[name] {
			t.Errorf("a card acts through %q, which the contract does not declare for agents: a view would hold a door the model cannot", name)
		}
		if spec.UI != nil && !slices.Contains(visibilityOf(spec), mcp.VisibilityModel) {
			t.Errorf("%q is app-only, so a surface without cards would lose the capability", name)
		}
	}
}

// visibilityOf is the audiences a tool's own view declaration names; empty
// means both, the extension's default.
func visibilityOf(spec mcp.ToolSpec) []string {
	if len(spec.UI.Visibility) == 0 {
		return []string{mcp.VisibilityModel, mcp.VisibilityApp}
	}
	return spec.UI.Visibility
}
