// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/assignments"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestRetirementEndsResponsibilitiesWhoseParentWasArchived(t *testing.T) {
	e := integration.Setup(t)
	manager := memberManagerFor(t, e)
	company := e.SeedCompany(t, "Closed account", &e.AdminUser)
	role, err := ids.Parse(e.WsScalar(t, `SELECT id::text FROM record_role WHERE key='account_manager'`))
	if err != nil {
		t.Fatal(err)
	}
	row, err := assignments.NewStore(e.DB()).CreateAssignment(e.Admin(), assignments.CreateAssignmentInput{
		RecordType: crmcontracts.AssignmentRecordTypeCompany, RecordID: company,
		SubjectKind: crmcontracts.AssignmentSubjectKindUser, SubjectID: e.Rep1, RoleID: role,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Contacts.ArchiveCompany(e.Admin(), ids.From[ids.CompanyKind](company), nil); err != nil {
		t.Fatal(err)
	}
	deactivate(t, e, e.Rep1)
	ctx := e.As(e.AdminUser, nil, integration.AdminWithSignals)
	preview, err := manager.Preview(ctx, e.Rep1, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Owned.RecordAssignment != 0 || len(preview.Blockers) != 0 {
		t.Fatalf("archived parent blocks retirement: %+v", preview)
	}
	actor, err := manager.service.MemberAdministrator(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.service.RetireMember(ctx, actor, ids.From[ids.UserKind](e.Rep1), nil, preview.Token, false); err != nil {
		t.Fatal(err)
	}
	if got := e.WsScalar(t, `SELECT (archived_at IS NOT NULL)::text FROM record_assignment WHERE id=$1`, row.Id); got != "true" {
		t.Fatal("retirement left an active responsibility on the archived parent")
	}
	if got := e.WsScalar(t, `SELECT (removed_at IS NOT NULL)::text FROM app_user WHERE id=$1`, e.Rep1); got != "true" {
		t.Fatal("source was not retired")
	}
}

func TestRetirementRejectsAReceiverWhoCannotWorkTheHandoff(t *testing.T) {
	e := integration.Setup(t)
	manager := memberManagerFor(t, e)
	lead := ids.UUID(seedBulkLeads(t, e, e.AdminUser, 1)[0].Id)
	handoff, err := e.Contacts.SubmitHandoff(e.Admin(), contacts.NewSDRHandoff{LeadID: &lead, AssignedTo: &e.Rep1})
	if err != nil {
		t.Fatal(err)
	}
	deactivate(t, e, e.Rep1)
	ctx := e.As(e.AdminUser, nil, integration.AdminWithSignals)
	preview, err := manager.Preview(ctx, e.Rep1, &e.Rep2, false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(preview.Blockers, crmcontracts.MemberRetirementBlocker("work_not_transferable")) {
		t.Fatalf("ineligible receiver was not identified: %+v", preview)
	}
	actor, err := manager.service.MemberAdministrator(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.service.RetireMember(ctx, actor, ids.From[ids.UserKind](e.Rep1), &e.Rep2, preview.Token, false); err == nil {
		t.Fatal("retired a member while their handoff could not move")
	}
	if got := e.WsScalar(t, `SELECT assigned_to::text FROM sdr_handoff WHERE id=$1`, handoff); got != e.Rep1.String() {
		t.Fatal("refused retirement changed the assignment")
	}
	if got := e.WsScalar(t, `SELECT (removed_at IS NULL)::text FROM app_user WHERE id=$1`, e.Rep1); got != "true" {
		t.Fatal("refused retirement removed the member")
	}
}
