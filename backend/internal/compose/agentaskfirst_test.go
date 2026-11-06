// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
	"github.com/margince/margince/backend/internal/shared/ports/model"
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

// The loop agent runs with no one at the screen. The ask-first clause an MCP
// client reads must never reach the model it reasons with. The run's bounds
// are its passport, its allowlist and the tier. The run here is the production
// runner over the production registry, offered every tool that asks.
func TestTheLoopAgentIsNeverToldToAskItsUser(t *testing.T) {
	registry := NewRegistry(nil, SendPath{})
	ctx := principal.WithActor(context.Background(), loopAgentGrantedEveryScope(registry))

	asks := map[string]string{}
	var tools []string
	for _, spec := range registry.Runnable(ctx) {
		if clause := askFirstClause(spec); clause != "" {
			asks[spec.Name] = clause
			tools = append(tools, spec.Name)
		}
	}
	if len(tools) < 5 {
		t.Fatalf("only %d offered tools carry an ask-first clause; the census is short and this gate proves nothing", len(tools))
	}

	brain := &requestRecorder{}
	if _, err := runner.New(registry, brain).Run(ctx, runner.Job{Goal: "sweep", Tools: tools}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(brain.requests) == 0 {
		t.Fatal("the run made no model call, so nothing it was shown was checked")
	}
	var request strings.Builder
	request.WriteString(brain.requests[0].System + string(brain.requests[0].ResponseSchema))
	for _, m := range brain.requests[0].Messages {
		request.WriteString(m.Content)
	}
	shown := request.String()
	for _, tool := range tools {
		if !strings.Contains(shown, "- "+tool+" — ") {
			t.Errorf("%s is not in the run's listing, so its absent clause proves nothing", tool)
		}
		if strings.Contains(shown, asks[tool]) {
			t.Errorf("the loop agent is told to ask its user before %s: %q", tool, asks[tool])
		}
	}
}

// askFirstClause is what DescribeForClient adds to spec because it asks first:
// the text between the parts it shares with the same spec unmarked.
func askFirstClause(spec mcp.ToolSpec) string {
	unmarked := spec
	unmarked.AskUserFirst = false
	unmarked.Actions = slices.Clone(spec.Actions)
	for i := range unmarked.Actions {
		unmarked.Actions[i].AskUserFirst = false
	}
	with, without := agents.DescribeForClient(spec), agents.DescribeForClient(unmarked)
	prefix := 0
	for prefix < len(without) && with[prefix] == without[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(without)-prefix && with[len(with)-1-suffix] == without[len(without)-1-suffix] {
		suffix++
	}
	return with[prefix : len(with)-suffix]
}

// loopAgentGrantedEveryScope is a scheduled run's principal: an agent on a
// colleague's passport, holding every scope some registered tool requires.
func loopAgentGrantedEveryScope(registry *agents.Registry) principal.Principal {
	var scopes []principal.Scope
	for _, spec := range registry.Specs() {
		scopes = append(scopes, spec.RequiredScope)
	}
	user := ids.NewV7()
	return principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:loop", UserID: user, OnBehalfOf: user,
		PassportID: ids.NewV7(), SeatType: principal.SeatFull, Scopes: principal.NewScopeSet(scopes...),
	}
}

// requestRecorder is the model boundary: it keeps each request and ends the run.
type requestRecorder struct{ requests []model.Request }

func (r *requestRecorder) Complete(_ context.Context, req model.Request) (model.Response, runner.Meta, error) {
	r.requests = append(r.requests, req)
	return model.Response{Text: `{"final":{"summary":"nothing to do"}}`}, runner.Meta{}, nil
}

func (*requestRecorder) PromptWindow() int { return 0 }
