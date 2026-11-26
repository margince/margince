// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

func TestTheToolChecksTableReads(t *testing.T) {
	verbs, err := EmbeddedToolChecks()
	if err != nil {
		t.Fatal(err)
	}
	if len(verbs) == 0 {
		t.Fatal("the embedded checks folder holds no verb")
	}
	for verb, row := range verbs {
		for _, name := range row.Checks {
			if _, err := auth.ParseCheck(name); err != nil {
				t.Errorf("%s: %v", verb, err)
			}
		}
	}
	mustReadToolChecks()
}

// everyRecordedGrant holds every grant the measured table names, so a fixture
// agent is narrowed only where a test says so.
func everyRecordedGrant() principal.Permissions {
	all := principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
	perms := principal.Permissions{RoleKeys: []string{"admin"}, RowScope: principal.RowScopeAll, Objects: map[string]principal.ObjectGrant{}}
	for _, checks := range requiredChecks {
		for _, check := range checks {
			perms.Objects[check.Object] = all
		}
	}
	return perms
}

// grantedCtx is an agent on a full seat whose user holds only deal.read.
func grantedCtx(seat principal.SeatType) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:granted", OnBehalfOf: ids.NewV7(), PassportID: ids.NewV7(),
		SeatType: seat, Scopes: principal.NewScopeSet(principal.ScopeRead, principal.ScopeWrite),
		Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{"deal": {Read: true}}},
	})
}

func dealCheck(t *testing.T, name string) []auth.Check {
	t.Helper()
	check, err := auth.ParseCheck(name)
	if err != nil {
		t.Fatal(err)
	}
	return []auth.Check{check}
}

func offeredByName(ctx context.Context, r *Registry) map[string]mcp.ToolSpec {
	out := map[string]mcp.ToolSpec{}
	for _, spec := range r.Offered(ctx) {
		out[spec.Name] = spec
	}
	return out
}

func routeEnum(t *testing.T, spec mcp.ToolSpec) []string {
	t.Helper()
	var schema struct {
		Properties map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(spec.InputSchema, &schema); err != nil {
		t.Fatal(err)
	}
	return schema.Properties[spec.ActionField].Enum
}

// A router offers only the actions whose checks the caller holds, and its enum,
// tier and description all come from those actions.
func TestARouterOffersOnlyTheActionsTheCallerHolds(t *testing.T) {
	r := NewRegistry(&recordingStageApprovals{}, auth.NewGate(fullSeatAuthority{}))
	r.Register(&argsTool{spec: targetSpec("fake_a", principal.ScopeWrite, mcp.TierAutoExecute)})
	r.Register(unlistedStaging{&stagingTool{target: ids.NewV7()}})
	r.Register(fakeRouter{name: "mixed", targets: map[string]string{"a": "fake_a", "send": "send_it"}})
	r.required = map[string][]auth.Check{"fake_a": dealCheck(t, "deal.read"), "send_it": dealCheck(t, "deal.update")}

	mixed, ok := offeredByName(grantedCtx(principal.SeatFull), r)["mixed"]
	if !ok {
		t.Fatal("the router is not offered, and the caller holds one of its actions")
	}
	if len(mixed.Actions) != 1 || mixed.Actions[0].Action != "a" {
		t.Errorf("the router offers %v, want only a", mixed.Actions)
	}
	if got := routeEnum(t, mixed); !slices.Equal(got, []string{"a"}) {
		t.Errorf("the action enum is %v, want [a]", got)
	}
	if mixed.Tier != mcp.TierAutoExecute {
		t.Errorf("the router is offered at tier %v, want the tier of its one action", mixed.Tier)
	}
	if text := DescribeForClient(mixed); strings.Contains(text, "send") {
		t.Errorf("the description names the hidden action: %s", text)
	}
}

func TestARouterWithNoActionLeftIsNotOffered(t *testing.T) {
	r, _, _ := routedRegistry(t)
	r.required = map[string][]auth.Check{"fake_a": dealCheck(t, "deal.update"), "fake_b": dealCheck(t, "deal.delete")}
	if _, ok := offeredByName(grantedCtx(principal.SeatFull), r)["fake_ab"]; ok {
		t.Error("the router is offered, and the caller holds none of its actions")
	}
}

// A verb the sweep measured no row for stays offered, and one whose check the
// caller lacks does not.
func TestAToolIsHiddenOnlyByAMeasuredCheck(t *testing.T) {
	r := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}))
	measured := targetSpec("measured", principal.ScopeWrite, mcp.TierAutoExecute)
	unmeasured := targetSpec("unmeasured", principal.ScopeWrite, mcp.TierAutoExecute)
	measured.Unlisted, unmeasured.Unlisted = false, false
	r.Register(&argsTool{spec: measured})
	r.Register(&argsTool{spec: unmeasured})
	r.required = map[string][]auth.Check{"measured": dealCheck(t, "deal.update")}

	offered := offeredByName(grantedCtx(principal.SeatFull), r)
	if _, ok := offered["measured"]; ok {
		t.Error("measured is offered, and the caller lacks deal.update")
	}
	if _, ok := offered["unmeasured"]; !ok {
		t.Error("unmeasured is not offered, and nothing says what it needs")
	}
	human := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, ID: "user:h"})
	if _, ok := offeredByName(human, r)["measured"]; !ok {
		t.Error("a human is narrowed by the table, which only lists what an agent is offered")
	}
}

// A scheduled run chooses among what its caller may invoke. A check that hides
// a tool from the listing never stops a run before its first step.
func TestARunMayChooseAToolTheListingHides(t *testing.T) {
	r := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}))
	measured := targetSpec("measured", principal.ScopeWrite, mcp.TierAutoExecute)
	measured.Unlisted = false
	r.Register(&argsTool{spec: measured})
	r.required = map[string][]auth.Check{"measured": dealCheck(t, "deal.update")}

	ctx := grantedCtx(principal.SeatFull)
	if _, ok := offeredByName(ctx, r)["measured"]; ok {
		t.Fatal("measured is listed, and the caller lacks deal.update, so this proves nothing")
	}
	if !slices.ContainsFunc(r.Runnable(ctx), func(spec mcp.ToolSpec) bool { return spec.Name == "measured" }) {
		t.Error("a run may not choose measured, which the gate decides when it is called")
	}
}

func TestAReadSeatIsOfferedOnlyReadTools(t *testing.T) {
	r := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}))
	write := targetSpec("writes", principal.ScopeWrite, mcp.TierAutoExecute)
	read := targetSpec("reads", principal.ScopeRead, mcp.TierAutoExecute)
	write.Unlisted, read.Unlisted = false, false
	r.Register(&argsTool{spec: write})
	r.Register(&argsTool{spec: read})

	offered := offeredByName(grantedCtx(principal.SeatRead), r)
	if _, ok := offered["writes"]; ok {
		t.Error("a read seat is offered a write tool, which Admit refuses it outright")
	}
	if _, ok := offered["reads"]; !ok {
		t.Error("a read seat is not offered a read tool")
	}
}

// A tool answering who the caller is stays offered whatever checks its path
// asks, as the scope filter keeps it.
func TestASelfDescribingToolIsOfferedWithoutItsChecks(t *testing.T) {
	r := NewRegistry(&recordingStageApprovals{}, auth.NewGate(fullSeatAuthority{}))
	spec := targetSpec("who_am_i", principal.ScopeRead, mcp.TierAutoExecute)
	spec.SelfDescribing, spec.Unlisted = true, false
	r.Register(&argsTool{spec: spec})
	r.required = map[string][]auth.Check{"who_am_i": dealCheck(t, "installation_settings.read")}
	if _, ok := offeredByName(grantedCtx(principal.SeatFull), r)["who_am_i"]; !ok {
		t.Error("a self-describing tool is hidden from a caller who lacks a check its path asks")
	}
}

// A router narrowed to the caller's actions names no hidden one where it is
// listed: copy, route field wording, schema or result shape. The argument only
// a hidden action takes is gone. The kept action's argument and the retry key
// stay. No other tool's description points at the hidden action.
func TestANarrowedRouterSchemaNamesNoHiddenAction(t *testing.T) {
	r := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}))
	kept := targetSpec("draft_email", principal.ScopeWrite, mcp.TierAutoExecute)
	kept.InputSchema = json.RawMessage(`{"type":"object","properties":{"x":{"type":"integer"},"mode":{"type":"string","enum":["calm"]}}}`)
	kept.OutputSchema = json.RawMessage(`{"type":"object","properties":{"draft_activity_id":{"type":"string"}}}`)
	hidden := targetSpec("draft_follow_ups_for", principal.ScopeWrite, mcp.TierAutoExecute)
	hidden.InputSchema = json.RawMessage(`{"type":"object","properties":{"x":{"type":"integer","maximum":5},"segment":{"type":"string"}}}`)
	hidden.OutputSchema = json.RawMessage(`{"type":"object","properties":{"follow_ups":{"type":"array"}}}`)
	pointer := targetSpec("points_at_drafts", principal.ScopeRead, mcp.TierAutoExecute)
	pointer.Unlisted, pointer.Description = false, "Then call draft_follow_ups_for to draft each follow-up."
	r.Register(&argsTool{spec: kept})
	r.Register(&argsTool{spec: hidden})
	r.Register(&argsTool{spec: pointer})
	r.Register(withView{draftOutbound{}})
	r.required = map[string][]auth.Check{"draft_email": dealCheck(t, "deal.read"), "draft_follow_ups_for": dealCheck(t, "deal.update")}

	served, ok := r.Spec("draft_outbound")
	if !ok || !strings.Contains(served.Description+string(served.InputSchema)+string(served.OutputSchema), "follow_ups") ||
		!strings.Contains(served.Description+string(served.InputSchema), "slipping_deals") {
		t.Fatalf("the registered router does not name its hidden action, so this proves nothing: %+v", served)
	}
	offered := offeredByName(grantedCtx(principal.SeatFull), r)
	narrowed, ok := offered["draft_outbound"]
	if !ok {
		t.Fatal("the router is not offered, and the caller holds one of its actions")
	}
	for part, text := range map[string]string{
		"description": narrowed.Description, "instead": narrowed.Instead,
		"input schema": string(narrowed.InputSchema), "output schema": string(narrowed.OutputSchema),
	} {
		if strings.Contains(text, "slipping_deals") || strings.Contains(text, "segment") || strings.Contains(text, "follow_ups") || strings.Contains(text, "at most") {
			t.Errorf("the narrowed %s still speaks of the hidden action: %s", part, text)
		}
	}
	if !strings.Contains(narrowed.Description, "one_message") || !strings.Contains(string(narrowed.InputSchema), "one_message drafts one email") {
		t.Errorf("the narrowed router lost the kept action's own wording: %s %s", narrowed.Description, narrowed.InputSchema)
	}
	for _, want := range []string{`"calm"`, idempotencyKeyArg} {
		if !strings.Contains(string(narrowed.InputSchema), want) {
			t.Errorf("the narrowed schema lost %s: %s", want, narrowed.InputSchema)
		}
	}
	if text := offered["points_at_drafts"].Description; strings.Contains(text, "slipping_deals") {
		t.Errorf("another tool's description points at the hidden action: %s", text)
	}
}

// withView is a router served with a view, so it carries a result shape.
type withView struct{ draftOutbound }

func (v withView) Spec() mcp.ToolSpec {
	spec := v.draftOutbound.Spec()
	spec.UI = &mcp.ToolUI{ResourceURI: "ui://margince/drafts.html"}
	return spec
}

func TestARouteFieldKeepsItsWordingWhereNoHiddenActionIsNamed(t *testing.T) {
	names := namesAny([]string{"archive"})
	if got, removed := clausesNotNaming("create opens one, update changes it; read shows it", names); removed ||
		got != "create opens one, update changes it; read shows it" {
		t.Errorf("no hidden action named: got %q, removed %v", got, removed)
	}
	got, removed := clausesNotNaming("create opens one, archive retires it; read shows it", names)
	if want := "create opens one, read shows it"; !removed || got != want {
		t.Errorf("got %q, removed %v; want %q", got, removed, want)
	}
}
