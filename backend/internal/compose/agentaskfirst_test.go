// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// A tool that archives, merges or sends, and offers no restore to take it back,
// cannot be undone from the conversation. So its listing must tell the client
// to get the user's yes first. Derived from the contract's operations and the
// send scope, so a new verb of either kind is held by being declared.
func TestEveryArchiveMergeOrSendToolAsksTheUserFirst(t *testing.T) {
	irreversible, restores := map[string]bool{}, map[string]bool{}
	for _, pol := range agentPolicies {
		if pol.Access != accessTool {
			continue
		}
		switch {
		case strings.HasPrefix(pol.Op, "archive"), strings.HasPrefix(pol.Op, "merge"):
			irreversible[pol.Tool] = true
		case strings.HasPrefix(pol.Op, "restore"):
			restores[pol.Tool] = true
		}
	}

	registry := NewRegistry(nil, SendPath{})
	for _, spec := range registry.Specs() {
		// A router's own Egress is its targets', which this loop also visits.
		if _, isRouter := registry.RouteTargets(spec.Name); isRouter {
			continue
		}
		// A message, mail or invitation that has left cannot be called back.
		if spec.Egress && spec.RequiredScope == principal.ScopeSend {
			irreversible[spec.Name] = true
		}
	}

	checked := 0
	tools := make([]string, 0, len(irreversible))
	for tool := range irreversible {
		tools = append(tools, tool)
	}
	sort.Strings(tools)
	for _, tool := range tools {
		if restores[tool] {
			continue
		}
		spec, registered := registry.Spec(tool)
		if !registered {
			continue // TestEveryDeclaredToolVerbIsRegistered reports it.
		}
		checked++
		// A router's listing reads the flag off its actions, never its own.
		if _, isRouter := registry.RouteTargets(tool); isRouter {
			if !slices.ContainsFunc(spec.Actions, func(a mcp.ToolAction) bool { return a.AskUserFirst }) {
				t.Errorf("router %s is declared for an archive or merge and offers no restore, but no "+
					"target it fronts sets AskUserFirst. Set it on the target verb that archives or merges", tool)
			}
			continue
		}
		if !spec.AskUserFirst {
			t.Errorf("%s archives, merges or sends and offers no restore, but its spec leaves "+
				"AskUserFirst unset, so a client is never told to ask the user before it runs", tool)
		}
	}
	if checked < 5 {
		t.Fatalf("only %d tools were checked; the census is short and this gate proves nothing", checked)
	}
}
