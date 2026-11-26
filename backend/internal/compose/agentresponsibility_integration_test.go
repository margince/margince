// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/assignments"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAnActiveAgentCanHoldAResponsibilityWithoutGainingAccess(t *testing.T) {
	e := integration.Setup(t)
	memberManagerFor(t, e)
	agent := declareSignatureServiceMember(t, e)
	company := e.SeedCompany(t, "Private company", &e.AdminUser)
	e.WsExec(t, `UPDATE company SET visibility='owner' WHERE id=$1`, company)
	actor := principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:" + agent.String(), UserID: agent.UUID, SeatType: principal.SeatFull,
		Permissions: principal.Permissions{RowScope: principal.RowScopeOwn, Objects: map[string]principal.ObjectGrant{"company": {Read: true}, "custom_field": {Read: true}}},
	}
	agentContext := principal.WithActor(e.Admin(), actor)
	companies := contacts.NewStore(e.DB())
	if _, err := companies.GetCompany(agentContext, ids.From[ids.CompanyKind](company), storekit.LiveOnly); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("private company before assignment: %v", err)
	}
	store := assignments.NewStore(e.DB())
	roles, err := store.ListRecordRoles(e.As(e.AdminUser, nil, integration.CustomFieldAdminPerms))
	if err != nil {
		t.Fatal(err)
	}
	var role ids.UUID
	for _, r := range roles {
		if r.Key == "account_manager" {
			role = ids.UUID(r.Id)
		}
	}
	if role.IsZero() {
		t.Fatal("account manager role missing")
	}
	created, err := store.CreateAssignment(e.Admin(), assignments.CreateAssignmentInput{RecordType: crmcontracts.AssignmentRecordTypeCompany, RecordID: company, SubjectKind: crmcontracts.AssignmentSubjectKindUser, SubjectID: agent.UUID, RoleID: role})
	if err != nil {
		t.Fatal(err)
	}
	if ids.UUID(created.SubjectId) != agent.UUID || created.SubjectName == nil || *created.SubjectName != "Signature service" {
		t.Fatalf("assigned another colleague: %+v", created)
	}
	if created.SubjectInactive == nil || *created.SubjectInactive {
		t.Fatal("active agent reported inactive")
	}
	if _, err := companies.GetCompany(agentContext, ids.From[ids.CompanyKind](company), storekit.LiveOnly); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("responsibility granted private company access: %v", err)
	}
}
