// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// A tool that archives, merges, sends, decides or disqualifies, and offers no
// restore to take it back, cannot be undone from the conversation. So its
// listing must tell the client to get the user's yes first. Derived from the
// contract's operations and the send scope, so a new verb of any kind is held
// by being declared.
func TestEveryIrreversibleToolAsksTheUserFirst(t *testing.T) {
	irreversible, restores := map[string]bool{}, map[string]bool{}
	for _, pol := range agentPolicies {
		if pol.Access != accessTool {
			continue
		}
		switch {
		case hasAnyPrefix(pol.Op, irreversibleOpPrefixes):
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
		if !spec.AskUserFirst {
			t.Errorf("%s cannot be undone from the conversation, but its spec leaves "+
				"AskUserFirst unset, so a client is never told to ask the user before it runs", tool)
		}
	}
	if checked < 5 {
		t.Fatalf("only %d tools were checked; the census is short and this gate proves nothing", checked)
	}
}

// irreversibleOpPrefixes are the contract verbs that act on a record or a
// queue in a way the conversation has no verb to take back.
var irreversibleOpPrefixes = []string{"archive", "merge", "disqualify", "approve", "reject", "executeBulkChange"}

func hasAnyPrefix(op string, prefixes []string) bool {
	return slices.ContainsFunc(prefixes, func(p string) bool { return strings.HasPrefix(op, p) })
}

// promisesApproval is written guidance that tells a caller a human will
// approve the call. A publish step a human takes later is not that promise.
var promisesApproval = regexp.MustCompile(`(?i)waits? for (?:a human|approval)|staged for (?:approval|confirmation)|needs? (?:a )?human'?s? (?:approval|decision)|a human approves`)

// publishStep is the one promise that stays true for an agent acting as its
// user. The draft is the agent's, and publishing is a later human act.
var publishStep = regexp.MustCompile(`(?i)waits for a human to publish`)

// A passport acting as its own user is admitted at auto-execute unless a tool
// sets WaitsEvenForItsUser (auth.TierFor). Written guidance promising such a
// caller a human approval is then false, and a model that trusts it skips
// asking the user. The tier belongs to the governance clause, which is read off
// the spec the caller is actually admitted on.
func TestNoToolPromisesAnApprovalItsUserSkips(t *testing.T) {
	registry := NewRegistry(nil, SendPath{})
	waits := map[string]bool{}
	for _, spec := range registry.Specs() {
		waits[spec.Name] = spec.WaitsEvenForItsUser
	}
	checked := 0
	for _, spec := range registry.Specs() {
		if spec.HumanOnly || spec.WaitsEvenForItsUser {
			continue
		}
		checked++
		// The input schema is client-facing guidance too.
		for _, sentence := range strings.SplitAfter(spec.Description+". "+string(spec.InputSchema), ". ") {
			if !promisesApproval.MatchString(publishStep.ReplaceAllString(sentence, "")) ||
				namesAToolThatWaits(sentence, spec, waits) {
				continue
			}
			t.Errorf("%s promises a human approval its user's own agent never meets: %q. "+
				"Say what the user should confirm instead; the governance clause states the tier.",
				spec.Name, strings.TrimSpace(sentence))
		}
	}
	if checked < 20 {
		t.Fatalf("only %d tools were checked; the census is short and this gate proves nothing", checked)
	}
}

// namesAToolThatWaits reports whether the sentence is about an action of this
// router whose target keeps waiting for a human even for its user's agent.
func namesAToolThatWaits(sentence string, spec mcp.ToolSpec, waits map[string]bool) bool {
	for _, action := range spec.Actions {
		if waits[action.Verb] && strings.Contains(sentence, action.Action) {
			return true
		}
	}
	return false
}
