// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// A scheduled agent's run is offered the verbs its allowlist names, listed
// externally or not. A router is refused: the runner allows the name called,
// so a router would allow every action behind it.
func TestNoScheduledAgentIsGivenARouter(t *testing.T) {
	registry := servedSurface(t)
	for _, agent := range mustScheduledAgents() {
		for _, name := range agent.Tools {
			if _, isRouter := registry.RouteTargets(name); isRouter {
				t.Errorf("%s is allowed %s, which a scheduled run cannot hold to one action; "+
					"name the verbs behind it instead", agent.Name, name)
			}
		}
	}
}

// A scheduled run is offered every verb its allowlist names, listed or not. The
// allowlist is then all that stands between a nightly run and an outbound or
// paid act. No allowlist may name a verb that leaves the workspace, buys data
// or decides an approval: each needs a human there when it runs.
func TestNoScheduledAgentIsGivenAVerbThatNeedsAHumanThere(t *testing.T) {
	registry := servedSurface(t)
	checked := 0
	for _, agent := range mustScheduledAgents() {
		for _, name := range agent.Tools {
			spec, registered := registry.Spec(name)
			if !registered {
				t.Errorf("%s is allowed %s, which no tool is registered as", agent.Name, name)
				continue
			}
			checked++
			decides := strings.HasPrefix(name, "decide_approval")
			if spec.Egress || spec.RequiredScope == principal.ScopeSend || spec.RequiredScope == principal.ScopeEnrich || decides {
				t.Errorf("%s is allowed %s, which sends, buys data or decides an approval; a scheduled run "+
					"has nobody there to answer for it", agent.Name, name)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no scheduled agent names a tool, so this checked nothing")
	}
}

// A model can call only what it is shown. A description, or an argument's,
// that names an Unlisted tool sends the model to a name it was never given.
func TestNoOfferedDescriptionNamesAnUnlistedTool(t *testing.T) {
	defer namesAnApprovalKind.AssertAllMatched(t)
	unlisted := unlistedTools(t)
	registry := servedSurface(t)
	for _, spec := range registry.Specs() {
		if spec.Unlisted {
			continue
		}
		spec = registry.Listed(spec)
		text := spec.Description + " " + strings.Join(argumentDescriptions(t, spec.InputSchema), " ")
		for _, word := range strings.FieldsFunc(text, func(c rune) bool {
			return c != '_' && !unicode.IsLower(c) && !unicode.IsDigit(c)
		}) {
			if unlisted[word] && !namesAnApprovalKind.Waived(t, spec.Name+":"+word) {
				t.Errorf("%s's description names %s, which is not offered; name the tool that reaches it",
					spec.Name, word)
			}
		}
	}
}

// A served resource's description is read by the same model, so it may not send
// it to a name it was never given either.
func TestNoServedResourceNamesAnUnlistedTool(t *testing.T) {
	unlisted := unlistedTools(t)
	var listed struct {
		Resources []struct {
			URI         string `json:"uri"`
			Description string `json:"description"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(mcpResult(t, "resources/list", primedViews(t, everyDeclaredView())), &listed); err != nil {
		t.Fatalf("decoding resources/list: %v", err)
	}
	if len(listed.Resources) == 0 {
		t.Fatal("resources/list served nothing, so this checked nothing")
	}
	for _, resource := range listed.Resources {
		for _, word := range strings.FieldsFunc(resource.Description, func(c rune) bool {
			return c != '_' && !unicode.IsLower(c) && !unicode.IsDigit(c)
		}) {
			if unlisted[word] {
				t.Errorf("%s's description names %s, which is not offered; name the call that reaches it",
					resource.URI, word)
			}
		}
	}
}

// namesAnApprovalKind ratifies a word in a description that is spelled like an
// unlisted tool and names something else: a staged action's kind, or a verb.
var namesAnApprovalKind = gatekit.Waive(map[string]string{
	"list_approvals:advance_deal":     "the kind filter's example is an approval kind, and a deal move stages under advance_deal",
	"change_state:set_up_partner":     "the action's own name on change_state, spelled like the tool it routes to",
	"bulk_update_records:remove_tag":  "one of its own bulk verbs, spelled like the tool and not naming it",
	"bulk_update_records:create_task": "one of its own bulk verbs, spelled like the tool and not naming it",
})

// argumentDescriptions collects the descriptions in an input schema, at any depth.
func argumentDescriptions(t *testing.T, schema json.RawMessage) []string {
	t.Helper()
	var doc any
	if err := json.Unmarshal(schema, &doc); err != nil {
		t.Fatalf("reading an input schema: %v", err)
	}
	var out []string
	var walk func(any)
	walk = func(node any) {
		switch n := node.(type) {
		case map[string]any:
			if d, ok := n["description"].(string); ok {
				out = append(out, d)
			}
			for _, v := range n {
				walk(v)
			}
		case []any:
			for _, v := range n {
				walk(v)
			}
		}
	}
	walk(doc)
	return out
}

// offeredSpecs is the served surface a client is offered: every registered tool
// but the ones only a router reaches.
func offeredSpecs(t *testing.T) []mcp.ToolSpec {
	t.Helper()
	return slices.DeleteFunc(servedSurface(t).Specs(), func(spec mcp.ToolSpec) bool { return spec.Unlisted })
}

func unlistedTools(t *testing.T) map[string]bool {
	t.Helper()
	unlisted := map[string]bool{}
	for _, spec := range servedSurface(t).Specs() {
		if spec.Unlisted {
			unlisted[spec.Name] = true
		}
	}
	return unlisted
}
