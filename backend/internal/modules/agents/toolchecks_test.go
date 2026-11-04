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
	var file ToolChecksFile
	if err := json.Unmarshal(toolChecksJSON, &file); err != nil {
		t.Fatal(err)
	}
	for verb, row := range file.Verbs {
		for _, name := range row.Checks {
			if _, err := auth.ParseCheck(name); err != nil {
				t.Errorf("%s: %v", verb, err)
			}
		}
	}
	mustReadToolChecks(toolChecksJSON)
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
