// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A tool that archives or merges, and offers no restore to take it back, cannot
// be undone from the conversation. So its listing must tell the client to get
// the user's yes first. Derived from the contract's operations, so a new archive
// or merge verb is held by being declared.
func TestEveryToolThatCannotBeUndoneAsksTheUserFirst(t *testing.T) {
	undoes, restores := map[string]bool{}, map[string]bool{}
	for _, pol := range agentPolicies {
		if pol.Access != accessTool {
			continue
		}
		switch {
		case strings.HasPrefix(pol.Op, "archive"), strings.HasPrefix(pol.Op, "merge"):
			undoes[pol.Tool] = true
		case strings.HasPrefix(pol.Op, "restore"):
			restores[pol.Tool] = true
		}
	}

	registry := NewRegistry(nil, SendPath{})
	for _, spec := range registry.Specs() {
		// A router carries the flag per action, read off its targets, which
		// this loop reaches by their own names.
		if _, isRouter := registry.RouteTargets(spec.Name); isRouter {
			continue
		}
		// A message, mail or invitation that has left cannot be called back.
		if spec.Egress && spec.RequiredScope == principal.ScopeSend {
			undoes[spec.Name] = true
		}
	}

	checked := 0
	tools := make([]string, 0, len(undoes))
	for tool := range undoes {
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
		if !spec.AskUserFirst {
			t.Errorf("%s archives, merges or sends and offers no restore, but its spec leaves "+
				"AskUserFirst unset, so a client is never told to ask the user before it runs", tool)
		}
	}
	if checked < 5 {
		t.Fatalf("only %d tools were checked; the census is short and this gate proves nothing", checked)
	}
}
