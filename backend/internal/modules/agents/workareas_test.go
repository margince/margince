// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// Two verbs the generated table places alone, one in each of two areas.
const (
	salesVerb = "accept_offer"
	adminVerb = "archive_automation"
)

var salesOnly = principal.WorkAreas{Limited: true, Chosen: []string{"business"}}

// inAreasCtx is grantedCtx with the passport's work areas set.
func inAreasCtx(areas principal.WorkAreas) context.Context {
	ctx := grantedCtx(principal.SeatFull)
	p, _ := principal.Actor(ctx)
	p.Areas = areas
	return principal.WithActor(ctx, p)
}

func areaRegistry(listed bool) (*Registry, *argsTool, *argsTool) {
	r := NewRegistry(&recordingStageApprovals{}, auth.NewGate(fullSeatAuthority{}))
	sales := &argsTool{spec: targetSpec(salesVerb, principal.ScopeWrite, mcp.TierAutoExecute)}
	admin := &argsTool{spec: targetSpec(adminVerb, principal.ScopeWrite, mcp.TierAutoExecute)}
	sales.spec.Unlisted, admin.spec.Unlisted = !listed, !listed
	r.Register(sales)
	r.Register(admin)
	// No measured check narrows these, so only the areas decide.
	r.required = nil
	return r, sales, admin
}

func TestAPassportIsOfferedOnlyTheToolsOfTheAreasItHasOn(t *testing.T) {
	r, _, _ := areaRegistry(true)

	offered := offeredByName(inAreasCtx(salesOnly), r)
	if _, ok := offered[salesVerb]; !ok {
		t.Errorf("%s is not offered, and its area is on", salesVerb)
	}
	if _, ok := offered[adminVerb]; ok {
		t.Errorf("%s is offered, and admin settings is off", adminVerb)
	}
	if _, ok := offeredByName(inAreasCtx(principal.WorkAreas{}), r)[adminVerb]; !ok {
		t.Errorf("%s is not offered to a passport set to all areas", adminVerb)
	}
}

func TestARouterKeepsOnlyTheActionsOfTheAreasOn(t *testing.T) {
	r, _, _ := areaRegistry(false)
	r.Register(fakeRouter{name: "mixed", targets: map[string]string{"offer": salesVerb, "automation": adminVerb}})

	mixed, ok := offeredByName(inAreasCtx(salesOnly), r)["mixed"]
	if !ok {
		t.Fatal("the router is not offered, and one of its actions is in an area that is on")
	}
	if got := routeEnum(t, mixed); !slices.Equal(got, []string{"offer"}) {
		t.Errorf("the router offers %v, want only the sales action", got)
	}
	if _, ok := offeredByName(inAreasCtx(principal.WorkAreas{Limited: true, Chosen: []string{"compliance"}}), r)["mixed"]; ok {
		t.Error("the router is offered, and none of its actions is in an area that is on")
	}
}

// The areas ride the principal each call authenticates, so turning an area
// back on admits the next call with no session to restart.
func TestTurningAnAreaBackOnAdmitsTheNextCall(t *testing.T) {
	r, _, admin := areaRegistry(false)
	r.Register(fakeRouter{name: "mixed", targets: map[string]string{"offer": salesVerb, "automation": adminVerb}})
	call := json.RawMessage(`{"action":"automation","x":1}`)

	_, err := r.Invoke(inAreasCtx(salesOnly), "mixed", call)
	var off *auth.AreaOffError
	if !errors.As(err, &off) || off.Area != "admin_settings" || admin.calls != 0 {
		t.Fatalf("a routed call into admin settings, left off → %v (ran %d times), want the area refusal", err, admin.calls)
	}
	if _, err := r.Invoke(inAreasCtx(principal.WorkAreas{Limited: true, Chosen: []string{"business", "admin_settings"}}), "mixed", call); err != nil {
		t.Fatalf("the same call once admin settings is on → %v, want admitted", err)
	}
	if admin.calls != 1 {
		t.Errorf("the admitted call ran %d times, want once", admin.calls)
	}
}

// recordTyped is a verb whose record type picks the operation, as create_record's does.
type recordTyped struct{ *argsTool }

func (r recordTyped) RecordTypeOf(args json.RawMessage) string { return recordTypeArg(args) }

func TestARecordTypeHoldsACallToItsOwnArea(t *testing.T) {
	r := NewRegistry(&recordingStageApprovals{}, auth.NewGate(fullSeatAuthority{}))
	create := recordTyped{&argsTool{spec: targetSpec("create_record", principal.ScopeWrite, mcp.TierAutoExecute)}}
	create.spec.InputSchema = json.RawMessage(`{"type":"object","properties":{"record_type":{"type":"string"}}}`)
	r.Register(create)
	r.required = nil

	ctx := inAreasCtx(salesOnly)
	if _, err := r.Invoke(ctx, "create_record", json.RawMessage(`{"record_type":"deal"}`)); err != nil {
		t.Fatalf("creating a deal with sales work on → %v, want admitted", err)
	}
	_, err := r.Invoke(ctx, "create_record", json.RawMessage(`{"record_type":"product"}`))
	var off *auth.AreaOffError
	if !errors.As(err, &off) || off.Area != "workspace_settings" {
		t.Fatalf("creating a product with only sales work on → %v, want refused in workspace settings", err)
	}
}

func TestTheAreasOfferedAreTheOnesTheGrantsReach(t *testing.T) {
	r, _, _ := areaRegistry(true)
	r.required = map[string][]auth.Check{salesVerb: dealCheck(t, "deal.read"), adminVerb: dealCheck(t, "deal.update")}
	p, _ := principal.Actor(grantedCtx(principal.SeatFull))

	if got := r.ReachableAreas(p); !slices.Equal(got, []string{"business"}) {
		t.Errorf("a user holding deal.read alone reaches %v, want only sales work", got)
	}
	p.Permissions = everyRecordedGrant()
	p.Permissions.Objects["deal"] = principal.ObjectGrant{Read: true, Update: true}
	if got := r.ReachableAreas(p); !slices.Equal(got, []string{"business", "admin_settings"}) {
		t.Errorf("a user holding both checks reaches %v, want sales work and admin settings in the table's order", got)
	}
}

// The agent is told the area and where its user turns it on. It is not told a
// scope is missing, since no scope would admit the call.
func TestTheAgentIsToldWhichAreaToAskFor(t *testing.T) {
	srv := NewDispatcher(nil, nil, "t", "0")
	got := srv.explain(adminVerb, &auth.AreaOffError{Tool: adminVerb, Area: "admin_settings"})
	if !strings.Contains(got, `"Admin settings"`) || !strings.Contains(got, "Settings, under Passports") || strings.Contains(got, "scope") {
		t.Fatalf("the agent reads %q", got)
	}
}
