// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/assignments"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/migrations"
)

func TestMemberHandoverMovesPendingStaffingAndRetainsAcceptedCredit(t *testing.T) {
	e := integration.Setup(t)
	manager := memberManagerFor(t, e)
	e.WsExec(t, `INSERT INTO role_assignment(user_id,role_id) SELECT @user::uuid,id FROM role WHERE key='admin'`, pgx.StrictNamedArgs{"user": e.Rep2})
	company := e.SeedCompany(t, "Example account", &e.Rep1)
	lead := ids.UUID(seedBulkLeads(t, e, e.Rep1, 1)[0].Id)
	pending, err := e.Contacts.SubmitHandoff(e.Admin(), contacts.NewSDRHandoff{LeadID: &lead, AssignedTo: &e.Rep1})
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := e.Contacts.SubmitHandoff(e.Admin(), contacts.NewSDRHandoff{LeadID: &lead, AssignedTo: &e.Rep1})
	if err != nil {
		t.Fatal(err)
	}
	pipeline, stage, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Accepted opportunity", pipeline, stage, &e.Rep1)
	if err := e.Contacts.DecideHandoff(e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms), accepted, contacts.HandoffDecision{Status: contacts.HandoffAccepted, DealID: &deal}); err != nil {
		t.Fatal(err)
	}
	role, err := ids.Parse(e.WsScalar(t, `SELECT id::text FROM record_role WHERE key='account_manager'`))
	if err != nil {
		t.Fatal(err)
	}
	responsibility, err := assignments.NewStore(e.DB()).CreateAssignment(e.Admin(), assignments.CreateAssignmentInput{
		RecordType: crmcontracts.AssignmentRecordTypeCompany, RecordID: company,
		SubjectKind: crmcontracts.AssignmentSubjectKindUser, SubjectID: e.Rep1, RoleID: role,
	})
	if err != nil {
		t.Fatal(err)
	}
	deactivate(t, e, e.Rep1)
	seam := handoverSeamFor(e)
	before, err := seam.Owned(e.Admin(), e.Rep1)
	if err != nil {
		t.Fatal(err)
	}
	if before.SdrHandoff != 1 || before.RecordAssignment != 1 {
		t.Fatalf("owned staffing = %+v", before)
	}
	result, err := seam.HandOver(integration.OnSeat(e.Admin(), principal.SeatFull), e.Rep1, e.Rep2)
	if err != nil {
		t.Fatal(err)
	}
	if result.Moved.SdrHandoff != 1 || result.Moved.RecordAssignment != 1 || result.Left.SdrHandoff != 0 || result.Left.RecordAssignment != 0 {
		t.Fatalf("handover staffing = %+v", result)
	}
	if got := e.WsScalar(t, `SELECT assigned_to::text FROM sdr_handoff WHERE id=$1`, pending); got != e.Rep2.String() {
		t.Fatalf("waiting handoff went to %s", got)
	}
	if got := e.WsScalar(t, `SELECT assigned_to::text FROM sdr_handoff WHERE id=$1`, accepted); got != e.Rep1.String() {
		t.Fatalf("accepted credit changed to %s", got)
	}
	for _, handoff := range []ids.UUID{pending, accepted} {
		if got := e.WsScalar(t, `SELECT submitted_by::text FROM sdr_handoff WHERE id=$1`, handoff); got != e.AdminUser.String() {
			t.Fatalf("submission history changed to %s", got)
		}
	}
	if got := e.WsScalar(t, `SELECT user_id::text FROM record_assignment WHERE id=$1`, responsibility.Id); got != e.Rep2.String() {
		t.Fatalf("responsibility went to %s", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM sdr_handoff_event WHERE handoff_id=$1`, pending); got != 1 {
		t.Fatalf("reassignment invented %d decision journal rows", got)
	}
	ctx := integration.OnSeat(e.As(e.AdminUser, nil, integration.AdminWithSignals), principal.SeatFull)
	preview, err := manager.Preview(ctx, e.Rep1, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	actor, err := manager.service.MemberAdministrator(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.service.RetireMember(ctx, actor, ids.From[ids.UserKind](e.Rep1), nil, preview.Token, false); err != nil {
		t.Fatal(err)
	}
	if got := e.WsScalar(t, `SELECT assigned_to::text FROM sdr_handoff WHERE id=$1`, accepted); got != e.Rep1.String() {
		t.Fatal("retirement rewrote accepted credit")
	}
	core, err := migrations.Core()
	if err != nil {
		t.Fatal(err)
	}
	owner := integration.FreshOwnerConn(t)
	_, err = dbmigrate.Down(t.Context(), owner, core, stepsDownTo(t, core, "accepted_handoffs_retain_their_receiver"))
	// The refusal comes after every newer migration was reverted; later tests need them back.
	if _, upErr := dbmigrate.Up(t.Context(), owner, core); upErr != nil {
		t.Fatal(upErr)
	}
	fault, ok := errors.AsType[*pgconn.PgError](err)
	if !ok || fault.Code != "23514" {
		t.Fatalf("unsafe downgrade was not refused: %v", err)
	}
	e.WsExec(t, `UPDATE sdr_handoff SET note='Historical correction' WHERE id=$1`, accepted)
}

// stepsDownTo counts the migrations a downgrade reverts to reach the named one,
// itself included, so a migration shipped after it does not take its place.
func stepsDownTo(t *testing.T, core dbmigrate.Namespace, name string) int {
	t.Helper()
	for i, m := range core.Migrations {
		if m.Name == name {
			return len(core.Migrations) - i
		}
	}
	t.Fatalf("no core migration is named %q", name)
	return 0
}
