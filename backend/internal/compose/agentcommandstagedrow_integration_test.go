// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The two doors, asked to archive one record neither caller can see: both refuse
// it and nothing is staged. The archive is floored confirm-first here, so the
// refusal is reached on the staging path of each door.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
	"github.com/margince/margince/backend/internal/shared/ports/authz/authztest"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// archiveContactPolicy is the operation both doors are asked to perform.
var archiveContactPolicy = agentPolicy{
	Op: "archiveContact", Access: accessTool, Tool: "archive_record",
	RecordType: recordTypeContact, Tier: tierConfirmationRequired, Scope: scopeWrite,
}

// A record the agent's own row scope hides is refused by BOTH doors, and
// neither stages anything.
//
// What this holds is the DOOR-TO-DOOR agreement over a real hidden row: one
// credential, one answer, through the real provider and the real store's
// row-scope clause. What it does not hold on its own is that Guards ran.
// Guards and Subject answer from one memoized read (archiveResolver.target),
// so a row the caller cannot read fails Subject too, and this test would pass
// against a door that resolved a subject and never asked the guard.
//
// Guards' own contribution — the external-system-of-record refusal, on a row
// the read SUCCEEDS for — is isolated per door instead, where a stub can hold
// the read open and vary only that one answer:
// TestAnArchiveOfAnExternallyHeldRecordStagesNothing for the REST door and
// TestTheToolDoorRefusesAnExternallyHeldRecordItCanRead for the tool door.
// Together with this test the claim is whole; neither half states it alone.
func TestBothDoorsRefuseOneRecordNeitherCallerCanSee(t *testing.T) {
	e := integration.Setup(t)
	native := NewProvider(e.Pool)
	agent := scopedArchiveAgent(t, e)

	// Capture-private to a rep in another team: a contact is otherwise readable
	// by every seat whoever owns it, so visibility='owner' is the one state that
	// still hides the row from the human this agent acts for.
	elsewhere := e.As(e.Rep3, []ids.UUID{e.Team2}, integration.AdminPerms)
	hidden, err := native.Create(elsewhere, datasource.CreateInput{
		EntityType: datasource.EntityContact,
		Fields:     json.RawMessage(`{"full_name":"Out Of Scope","owner_id":"` + e.Rep3.String() + `"}`),
		Source:     "test",
	})
	if err != nil {
		t.Fatalf("seeding the out-of-scope contact: %v", err)
	}
	e.MakeCapturePrivate(t, "contact", hidden.ID, e.Rep3)

	rec := httptest.NewRecorder()
	stageRefusal(rec, archiveRequestFor(agent, hidden.ID), approvalsAdapter{svc: approvals.NewService(e.DB())},
		restCommandDeps{records: native}, archiveContactPolicy, nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("the REST door answered %d for a row outside the agent's scope, want 404", rec.Code)
	}

	_, invokeErr := archiveRegistry(e).Invoke(agent, "archive_record",
		json.RawMessage(`{"record_type":"contact","id":"`+hidden.ID.String()+`"}`))
	if staged, ok := errors.AsType[*workflow.StagedApprovalError](invokeErr); ok {
		t.Errorf("the tool door staged approval %s for a row outside the agent's scope, where the REST door "+
			"refused — one credential, two answers", staged.ApprovalID)
	}

	if n := pendingApprovals(agent, t, e); n != 0 {
		t.Errorf("%d approval(s) were staged against a record neither door's caller can see", n)
	}
}

// archiveRequestFor is the request the router would hand the gate for
// DELETE /v1/contacts/{id}, carrying the agent's own context.
func archiveRequestFor(as context.Context, contact ids.UUID) *http.Request {
	req := httptest.NewRequest(http.MethodDelete, "/v1/contacts/"+contact.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", contact.String())
	return req.WithContext(context.WithValue(as, chi.RouteCtxKey, rctx))
}

// repSeat re-derives, for the tool door's admission gate, exactly the authority
// the REST door's own request context carries: a full seat with a rep's
// permissions and the rep's team. Both doors must be governed by ONE authority
// or the comparison below is about seats rather than about doors — and a gate
// handed admin permissions would read rows the REST side's row scope hides,
// which is a difference in the fixture, not in the surface.
//
// The harness stamps a rep's authority onto its contexts rather than seeding
// role grants, so the live identity resolver has nothing to answer with here;
// this stands in for it, with the same two answers.
type repSeat struct{ e *integration.Env }

func (s repSeat) EffectiveRBAC(context.Context, ids.UUID, ids.UUID) (authz.RBAC, error) {
	return authz.RBAC{Permissions: archiveRepPerms, TeamIDs: []ids.UUID{s.e.Team1}}, nil
}

func (repSeat) SeatType(context.Context, ids.UUID, ids.UUID) (principal.SeatType, error) {
	return principal.SeatFull, nil
}

// archiveRegistry is the tool door over the real provider and the real
// approvals engine — the same adapter the composed api server injects, so the
// row this door writes is written by production's own stager.
// archiveRegistry builds the tool door with a FLOOR on archive_record for a
// contact, which is how this operation reaches confirm-first now: the verb
// executes by default — a passport does what its holder could do unaided — and
// an installation that wants it confirmed declares the floor. The policy this
// test compares against (archiveContactPolicy) declares exactly that, so both
// doors are asked the same question.
func archiveRegistry(e *integration.Env) *agents.Registry {
	native := NewProvider(e.Pool)
	reg := agents.NewRegistry(approvalsAdapter{svc: approvals.NewService(e.DB())}, auth.NewGate(repSeat{e}),
		agents.WithTierFloor(func(tool, recordType string) (mcp.RiskTier, bool) {
			if tool == "archive_record" && recordType == string(recordTypeContact) {
				return mcp.TierConfirmationRequired, true
			}
			return mcp.TierAutoExecute, false
		}))
	// No consumer-mail seam: this registry exists to exercise the ARCHIVE tier
	// floor, and qualify_lead is not on its path. A nil seam is safe rather
	// than latent — the tool refuses on the terms an unreadable list refuses
	// on, so an accidental call here would fail loudly instead of deriving a
	// company from the compiled-in baseline.
	agents.RegisterCoreTools(reg, native, native, nil, nil, nil, nil)
	return reg
}

// AdmittedAuthority delegates to this fixture's own two reads; see
// authztest.AdmittedFromPair for why the body is not written out here.
func (s repSeat) AdmittedAuthority(ctx context.Context, ws, human, _ ids.UUID) (authz.RBAC, principal.SeatType, error) {
	return authztest.AdmittedFromPair(ctx, ws, human, s.EffectiveRBAC, s.SeatType)
}
