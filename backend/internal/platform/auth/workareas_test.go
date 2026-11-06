// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

func agentInAreas(areas principal.WorkAreas, scopes ...principal.Scope) context.Context {
	ctx := agentCtx(scopes...)
	p, _ := principal.Actor(ctx)
	p.Areas = areas
	return principal.WithActor(ctx, p)
}

var salesOnly = principal.WorkAreas{Limited: true, Chosen: []string{"business"}}

func TestAPassportIsRefusedACallInAnAreaItLeavesOff(t *testing.T) {
	authority := &stubAuthority{seat: principal.SeatFull}
	spec := mcp.ToolSpec{Name: "create_pipeline", RequiredScope: principal.ScopeWrite, Tier: mcp.TierAutoExecute, Areas: []string{"admin_settings"}}

	_, err := NewGate(authority).Admit(agentInAreas(salesOnly, principal.ScopeWrite), spec, noResolve)
	var off *AreaOffError
	if !errors.As(err, &off) || off.Area != "admin_settings" || !errors.Is(err, apperrors.ErrScopeExceeded) {
		t.Fatalf("a call in an area left off → %v, want an AreaOffError naming admin_settings on the 403 row", err)
	}
	_, message := off.MessageFault()
	if !strings.Contains(message, "“Admin settings”") || !strings.Contains(message, "Settings → AI access") {
		t.Errorf("the refusal reads %q; it should name the area as the user knows it and where to turn it on", message)
	}
}

func TestAPassportIsAdmittedInAnAreaItHasOn(t *testing.T) {
	spec := mcp.ToolSpec{Name: "read_record", RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute, Areas: []string{"business", "workspace_settings"}}
	if _, err := fullSeatGate().Admit(agentInAreas(salesOnly, principal.ScopeRead), spec, noResolve); err != nil {
		t.Fatalf("a call with one of its areas on → %v, want admitted", err)
	}
	if _, err := fullSeatGate().Admit(agentInAreas(principal.WorkAreas{}, principal.ScopeRead), spec, noResolve); err != nil {
		t.Fatalf("a passport set to all areas → %v, want admitted", err)
	}
}

// A tool placed in no area, such as an extension's, is reached only by a
// passport set to all areas. A later area starts off for a limited one.
func TestAToolInNoAreaRunsOnlyForAllAreas(t *testing.T) {
	spec := mcp.ToolSpec{Name: "ext_tool", RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute}
	if _, err := fullSeatGate().Admit(agentInAreas(salesOnly, principal.ScopeRead), spec, noResolve); !errors.Is(err, apperrors.ErrScopeExceeded) {
		t.Fatalf("an unplaced tool on a limited passport → %v, want refused", err)
	}
	if _, err := fullSeatGate().Admit(agentInAreas(principal.WorkAreas{}, principal.ScopeRead), spec, noResolve); err != nil {
		t.Fatalf("an unplaced tool on a passport set to all areas → %v, want admitted", err)
	}
}

func TestTheCallerItselfIsInEveryArea(t *testing.T) {
	spec := mcp.ToolSpec{Name: "whoami", RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute, SelfDescribing: true}
	if _, err := fullSeatGate().Admit(agentInAreas(principal.WorkAreas{Limited: true, Chosen: []string{"compliance"}}), spec, noResolve); err != nil {
		t.Fatalf("asking who the caller is → %v, want admitted whatever the areas", err)
	}
}

func TestAnAreaOutsideTheChoiceTellsTheUserToSetAllAreas(t *testing.T) {
	for area, want := range map[string]string{
		"operator": `“Operator and system” area, which only a passport set to all areas reaches`,
		"":         "this call is in none of them",
	} {
		_, message := (&AreaOffError{Area: area}).MessageFault()
		if !strings.Contains(message, want) || !strings.Contains(message, "all areas in Settings") {
			t.Errorf("area %q reads %q; want it to say %q and how to set all areas", area, message, want)
		}
	}
}

// A run authenticates once and keeps its principal, so the areas a passport
// holds now come from the gate's own read, not from that stamp.
func TestAPassportsAreasAreReadAgainAtEveryCall(t *testing.T) {
	authority := &stubAuthority{seat: principal.SeatFull, rbac: authz.RBAC{PassportAreas: salesOnly}}
	ctx := agentCtxWithPassport(ids.NewV7(), principal.ScopeRead)
	spec := mcp.ToolSpec{Name: "list_roles", RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute, Areas: []string{"admin_settings"}}

	if _, err := NewGate(authority).Admit(ctx, spec, noResolve); !errors.Is(err, apperrors.ErrScopeExceeded) {
		t.Fatalf("a run stamped at all areas, whose passport is now sales work only → %v, want refused", err)
	}
	authority.rbac.PassportAreas = principal.WorkAreas{}
	if _, err := NewGate(authority).Admit(ctx, spec, noResolve); err != nil {
		t.Fatalf("the same run once its passport is back at all areas → %v, want admitted", err)
	}
}

// Operator work is held as admin settings, and agent governance is admitted on
// every passport, so no limited passport loses its own approvals.
func TestAnAreaOutsideTheChoiceFollowsTheSwitchItIsHeldAs(t *testing.T) {
	ctx := agentInAreas(principal.WorkAreas{Limited: true, Chosen: []string{"admin_settings"}}, principal.ScopeRead)
	operator := mcp.ToolSpec{Name: "read_data_tasks", RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute, Areas: []string{"operator"}}
	if _, err := fullSeatGate().Admit(ctx, operator, noResolve); err != nil {
		t.Errorf("operator work with admin settings on → %v, want admitted", err)
	}
	approvals := mcp.ToolSpec{Name: "list_approvals", RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute, Areas: []string{"agent_governance"}}
	if _, err := fullSeatGate().Admit(agentInAreas(salesOnly, principal.ScopeRead), approvals, noResolve); err != nil {
		t.Errorf("the approval loop on a sales-only passport → %v, want admitted", err)
	}
	var off *AreaOffError
	if _, err := fullSeatGate().Admit(agentInAreas(salesOnly, principal.ScopeRead), operator, noResolve); !errors.As(err, &off) || off.Area != "admin_settings" {
		t.Errorf("operator work on a sales-only passport → %v, want refused naming admin settings", err)
	}
}

// A tool about the protocol itself sits in no area of work.
func TestAProtocolProbeIsInEveryArea(t *testing.T) {
	spec := mcp.ToolSpec{Name: "check_client", RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute, ProtocolOnly: true}
	if _, err := fullSeatGate().Admit(agentInAreas(salesOnly, principal.ScopeRead), spec, noResolve); err != nil {
		t.Fatalf("a protocol probe on a limited passport → %v, want admitted", err)
	}
}
