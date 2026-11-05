// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

func TestAMemberMergeStagesReviewsApprovesAndRedeemsThroughTheToolDoor(t *testing.T) {
	e := integration.Setup(t)
	manager := memberManagerFor(t, e)
	human := e.As(e.AdminUser, nil, integration.AdminWithSignals)
	actor := identity.Identity{UserID: ids.From[ids.UserKind](e.AdminUser), Roles: []string{"admin"}, SeatType: "full", Permissions: integration.AdminWithSignals}
	passport, err := manager.service.IssuePassport(human, actor, identity.IssuePassportInput{Scopes: []string{"read", "write"}})
	if err != nil {
		t.Fatal(err)
	}
	agent := principal.WithActor(human, principal.Principal{Type: principal.PrincipalAgent, ID: "agent:member-management", OnBehalfOf: e.AdminUser, UserID: e.AdminUser, PassportID: passport.ID.UUID, SeatType: principal.SeatFull, Scopes: principal.NewScopeSet(principal.ScopeRead, principal.ScopeWrite), Permissions: integration.AdminWithSignals})
	registry := registryWithGate(e.DB(), auth.NewGate(manager.service), nil, SendPath{}, companyEnricher{}, nil, nil, nil, nil, slog.Default(), registryFeatures{})
	previewArgs, err := json.Marshal(map[string]string{"source_id": e.Rep1.String(), "to_user_id": e.Rep2.String(), "mode": "merge"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := registry.Invoke(agent, "preview_member_retirement", previewArgs)
	if err != nil {
		t.Fatal(err)
	}
	assertMemberResultSchema(t, registry, "preview_member_retirement", raw)
	var envelope agents.Envelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	var preview crmcontracts.MemberRetirementPreview
	if err := json.Unmarshal(envelope.Data, &preview); err != nil {
		t.Fatal(err)
	}
	args := map[string]string{"source_id": e.Rep1.String(), "to_user_id": e.Rep2.String(), "preview_token": preview.Token}
	request, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	_, err = registry.Invoke(agent, "merge_members", request)
	var staged *workflow.StagedApprovalError
	if !errors.As(err, &staged) {
		t.Fatalf("staging = %v", err)
	}
	approvals := NewApprovalsService(e.DB())
	proposal, err := approvals.GetWire(human, staged.ApprovalID)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Preview == nil || proposal.Preview.State != "ready" || proposal.Preview.Effects == nil {
		t.Fatalf("review = %+v", proposal.Preview)
	}
	if _, err := approvals.Decide(human, staged.ApprovalID, true, nil); err != nil {
		t.Fatal(err)
	}
	args["approval_id"] = staged.ApprovalID.String()
	request, err = json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	raw, err = registry.Invoke(agent, "merge_members", request)
	if err != nil {
		t.Fatal(err)
	}
	assertMemberResultSchema(t, registry, "merge_members", raw)
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	if string(envelope.Data) != `{"merged":true}` {
		t.Fatalf("merge response = %s", raw)
	}
	if e.WsScalar(t, `SELECT merged_into_user_id::text FROM app_user WHERE id=$1`, e.Rep1) != e.Rep2.String() {
		t.Fatal("approved effect did not merge the member")
	}
}

func assertMemberResultSchema(t *testing.T, registry *agents.Registry, tool string, result json.RawMessage) {
	t.Helper()
	spec, ok := registry.Spec(tool)
	if !ok {
		t.Fatalf("%s was not registered", tool)
	}
	if defect := agents.ResultDefect(spec.OutputSchema, result); defect != "" {
		t.Fatalf("%s result violates its served schema: %s", tool, defect)
	}
}
