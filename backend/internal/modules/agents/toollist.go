// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// What a caller is SHOWN: the tool list tools/list answers, filtered to what
// this principal could invoke at all, and the description each entry carries.
//
// It sits beside the dispatcher rather than inside it because advertising and
// dispatching answer different questions. This decides what a client is told
// exists; the gate decides what it may do. The two must agree without being the
// same code — a surface that advertises what the gate will refuse is a surface
// that lies, and the client's only way to learn the truth is to call and be
// denied.

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/modules/agents/apps"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// invocableByCaller reports whether the calling principal's passport scopes
// would let it invoke spec at all. It mirrors the scope arm of auth.Gate.Admit
// deliberately — a surface that advertises what the gate will refuse is a
// surface that lies, and the client's only way to discover the truth is to
// call and be denied.
//
// Registry.Offered is its only caller, and every narrowing of the TOOL catalog
// goes through there — the external tools/list and the tool listing a Surface-B
// run is shown alike.
//
// The resources catalogue is the one sibling that does not: readableByCaller
// spells the same three branches over mcp.Resource. One rule, two spellings,
// because neither type carries the other's fields — which is a real cost, not a
// tidy separation. A third surface needing this rule should lift the predicate
// rather than copy it a third time.
//
// It answers the SCOPE axis only, which is what §5.7 promises. The seat
// ceiling and object RBAC are re-derived per call through the authority seam
// and are a named follow-up (§10.2); this filter must not pretend to enforce
// them, and Registry.Invoke remains the authority for every one of them.
//
// A ctx with no principal shows nothing rather than everything: the caller of
// a tools/list that never authenticated has no scopes, and an empty surface is
// the honest answer.
func invocableByCaller(ctx context.Context, spec mcp.ToolSpec) bool {
	p, ok := principal.Actor(ctx)
	if !ok {
		return false
	}
	// A Deal Room participant holds no tool authority at all — auth.Gate.Admit
	// refuses every tool for a Buyer outright, not on the scope axis but by
	// kind (admit.go: "a Deal Room participant holds no tool authority").
	// Checked before the "not an Agent" fast path below, or a Buyer would be
	// offered the whole surface and refused every one of them.
	if p.Type == principal.PrincipalBuyer {
		return false
	}
	// Humans and the system principal do not ride the scope model — their
	// authority is their RBAC, enforced at the store — so filtering them by a
	// passport scope they never carry would hide the whole surface.
	if p.Type != principal.PrincipalAgent {
		return true
	}
	// A HumanOnly tool requests no agent authority and RequireHuman refuses
	// this same principal at Invoke — offering it here would advertise a call
	// the gate can only ever deny, exactly the lie this function exists to
	// prevent. Ordered first rather than relying on it never co-occurring
	// with SelfDescribing, so the rule reads plainly.
	if spec.HumanOnly {
		return false
	}
	// A tool answering who the caller is stays offered whatever the passport
	// is scoped to do; mcp.ToolSpec.SelfDescribing says why, and the
	// admission gate reads the same flag so the listing cannot offer what the
	// gate would then refuse.
	if spec.SelfDescribing {
		return true
	}
	return p.Scopes.Has(spec.RequiredScope)
}

// DescribeForClient is one tool's advertised description: what the tool is for,
// then how this server governs the call. tools/list and GET /v1/agent-tools both
// serve it, so a client and the operator console read the same text.
//
// The written text comes first, because it answers which tool serves a goal.
// Running at once under the read scope is the default the surface instructions
// state, so such a tool says nothing more. Every other tier and every scope past
// read is named, read off the spec the admission gate enforces.
func DescribeForClient(spec mcp.ToolSpec) string {
	scope := fmt.Sprintf("requires passport scope %q", spec.RequiredScope)
	// Every arm is named, and the fallthrough is the CONSERVATIVE reading, not
	// the convenient one: the admission gate treats anything that is not
	// TierAutoExecute as confirm-first, so a tier added without updating this
	// switch must not be advertised as running unattended. The same posture
	// tierWire takes on the REST side, for the same reason.
	tier := "a human approves every call before it runs"
	switch spec.Tier {
	case mcp.TierAutoExecute:
		if spec.RequiredScope == principal.ScopeRead {
			return spec.Description
		}
		return fmt.Sprintf("%s (Runs immediately%s; %s.)", spec.Description, askUserFirst(spec), scope)
	case mcp.TierConfirmationRequired:
		tier = "a human approves every call before it runs"
	case mcp.TierDynamic:
		tier = "some calls run immediately and others a human approves first, decided per call from its arguments"
		if byAction := actionGovernance(spec.Actions); byAction != "" {
			tier = byAction
		}
	}
	return fmt.Sprintf("%s (Governance: %s%s; %s.)", spec.Description, tier, askUserFirst(spec), scope)
}

// askUserFirst tells the client to get the user's yes before a call that runs
// with no human approval and cannot be undone. A call a human approves already
// gets that yes, so it is never named here.
func askUserFirst(spec mcp.ToolSpec) string {
	const ask = "; %s cannot be undone, so tell the user exactly what the call will change and " +
		"get their yes before calling, unless they already asked for exactly this"
	if len(spec.Actions) == 0 {
		if !spec.AskUserFirst || spec.Tier == mcp.TierConfirmationRequired {
			return ""
		}
		return fmt.Sprintf(ask, "it")
	}
	var names []string
	for _, a := range spec.Actions {
		if a.AskUserFirst && a.Tier != mcp.TierConfirmationRequired {
			names = append(names, a.Action)
		}
	}
	switch len(names) {
	case 0:
		return ""
	case 1:
		return fmt.Sprintf(ask, "the action "+names[0])
	}
	slices.Sort(names)
	return fmt.Sprintf(ask, "the actions "+strings.Join(names, ", "))
}

// actionGovernance says which of a router's actions run at once, which wait for
// a human and which decide per call. Empty for a tool with no actions.
func actionGovernance(actions []mcp.ToolAction) string {
	groups := map[mcp.RiskTier][]string{}
	for _, a := range actions {
		groups[a.Tier] = append(groups[a.Tier], a.Action)
	}
	var parts []string
	for _, g := range []struct {
		tier  mcp.RiskTier
		words string
	}{
		{mcp.TierAutoExecute, "runs immediately"},
		{mcp.TierConfirmationRequired, "a human approves first"},
		{mcp.TierDynamic, "decided per call from its arguments"},
	} {
		if names := groups[g.tier]; len(names) > 0 {
			slices.Sort(names)
			parts = append(parts, fmt.Sprintf("%s for %s", g.words, strings.Join(names, ", ")))
		}
	}
	return strings.Join(parts, "; ")
}

// toolList reads Registry.Offered rather than filtering Specs itself, so the
// external catalog and the one a Surface-B run is shown are the same function
// and not two that agree today.
//
// It takes the FRAMING because one member of a tool's entry is negotiated:
// `_meta.ui` is offered only to a request that declared the App extension, and
// only where this server actually serves views. That is the rule the Tasks
// extension already rides, for the same reason — advertising an extension to a
// client that cannot enter its negotiation offers a capability the client has no
// way to use, and here one whose whole point is that the HOST prefetch and
// sandbox a document it was told about.
func (s *Dispatcher) toolList(ctx context.Context, fr framing) []map[string]any {
	specs := s.registry.Offered(ctx)
	tools := make([]map[string]any, 0, len(specs))
	actionTools := apps.ActionTools()
	for _, spec := range specs {
		tool := map[string]any{
			fieldName: spec.Name,
			// Top-level title outranks annotations.title for display, and both
			// outrank the name. Registry.Register refuses a title-less tool, so
			// neither is ever the empty string here.
			fieldTitle:    spec.Title,
			"description": DescribeForClient(spec),
			"inputSchema": spec.InputSchema,
			// The two hints this server can state as FACTS, both read off the
			// spec the admission gate itself enforces rather than restated by
			// hand: what a tool may change is its scope, and whether it leaves
			// the workspace is its egress flag.
			//
			// destructiveHint and idempotentHint are deliberately absent: their
			// protocol defaults (destructive, non-idempotent) are already the
			// conservative reading, and only the looser value would need a
			// per-tool judgement, with nothing to hold it true.
			"annotations": map[string]any{
				fieldTitle:      spec.Title,
				"readOnlyHint":  spec.ReadOnly(),
				"openWorldHint": spec.Egress,
			},
		}
		if spec.OutputSchema != nil {
			tool["outputSchema"] = spec.OutputSchema
		}
		// The view, offered only where BOTH halves are real: this request
		// declared the extension, and this server serves the documents. Either
		// missing and the member is absent rather than empty — a client reads
		// `_meta.ui` as "there is a view to fetch", so an entry pointing at a
		// document this deployment does not publish is worse than none.
		if s.appsOffered(fr) {
			if ui := s.toolMeta(spec, actionTools); ui != nil {
				tool[fieldMeta] = map[string]any{metaUIKey: ui}
			}
		}
		tools = append(tools, tool)
	}
	return tools
}
