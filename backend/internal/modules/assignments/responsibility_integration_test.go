// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package assignments

// Who is responsible for a company, against a real database.
//
// The store states its authority beside each probe: out of scope reads as
// not-found, seeing a record is not leave to staff it, the actor comes from
// the session. A gate asserted only in prose looks exactly like one that
// passes, so each of those properties is held here by SQL that runs.

import (
	"context"
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// companyCRUD is the object grant of a seat that may do everything to a company.
var companyCRUD = principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}

// actingAs builds a human seat holding exactly one object grant, on company, at
// one row scope. The vocabulary grant rides along because the role picker
// reads under it; nothing here exercises it.
func (e *roleEnv) actingAs(user ids.UUID, grant principal.ObjectGrant, scope principal.RowScope) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			RowScope: scope,
			Objects: map[string]principal.ObjectGrant{
				"company":                  grant,
				assignmentVocabularyObject: {Read: true},
			},
		},
	})
}

func (e *roleEnv) seedColleague(t *testing.T, name string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, $3)`,
		id, name+"-"+id.String()+"@responsibility.test", name); err != nil {
		t.Fatal(err)
	}
	return id
}

func (e *roleEnv) seedCompany(t *testing.T, owner ids.UUID, visibility string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO company (id, display_name, source, captured_by, owner_id, visibility)
		 VALUES ($1, 'Account under test', 'manual', 'human:seed', $2, $3)`,
		id, owner, visibility); err != nil {
		t.Fatal(err)
	}
	return id
}

// accountManager is the seeded role every company admits for a colleague.
func (e *roleEnv) accountManager(t *testing.T) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := e.owner.QueryRow(context.Background(),
		`SELECT id FROM record_role WHERE key = 'account_manager'`).Scan(&id); err != nil {
		t.Fatalf("the seeded account_manager role: %v", err)
	}
	return id
}

func staffing(company, colleague, role ids.UUID) CreateAssignmentInput {
	return CreateAssignmentInput{
		RecordType:  crmcontracts.AssignmentRecordTypeCompany,
		RecordID:    company,
		SubjectKind: crmcontracts.AssignmentSubjectKindUser,
		SubjectID:   colleague,
		RoleID:      role,
	}
}

func (e *roleEnv) listCompany(ctx context.Context, company ids.UUID) ([]crmcontracts.RecordAssignment, error) {
	return e.store.ListForRecord(ctx, crmcontracts.AssignmentRecordTypeCompany, company)
}

// TestStaffingACompanyIsRecordedAsTheCaller holds the write shape: the row,
// its audit entry and its captured_by all name the session's principal, and
// the read-back joins the role and the colleague so the card needs no second
// lookup.
func TestStaffingACompanyIsRecordedAsTheCaller(t *testing.T) {
	e := setupRoleEnv(t)
	ctx := e.actingAs(e.admin, companyCRUD, principal.RowScopeAll)
	company := e.seedCompany(t, e.admin, "workspace")
	colleague := e.seedColleague(t, "Dana")

	created, err := e.store.CreateAssignment(ctx, staffing(company, colleague, e.accountManager(t)))
	if err != nil {
		t.Fatalf("staffing a company: %v", err)
	}
	if created.RoleKey == nil || *created.RoleKey != "account_manager" {
		t.Fatalf("role key on the read-back = %v, want account_manager", created.RoleKey)
	}
	if created.SubjectName == nil || *created.SubjectName != "Dana" {
		t.Fatalf("subject name on the read-back = %v, want Dana", created.SubjectName)
	}

	listed, err := e.listCompany(ctx, company)
	if err != nil {
		t.Fatalf("listing the team: %v", err)
	}
	if len(listed) != 1 || listed[0].Id != created.Id {
		t.Fatalf("the list after one staffing = %+v, want exactly the row just created", listed)
	}

	me := "human:" + e.admin.String()
	var capturedBy, action, actorID string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT a.captured_by, l.action, l.actor_id
		   FROM record_assignment a
		   JOIN audit_log l ON l.entity_type = 'record_assignment' AND l.entity_id = a.id
		  WHERE a.id = $1`, ids.UUID(created.Id)).Scan(&capturedBy, &action, &actorID); err != nil {
		t.Fatalf("the row and its audit entry: %v", err)
	}
	if capturedBy != me || actorID != me || action != "create" {
		t.Fatalf("captured_by=%q audit actor=%q action=%q, want both %q and create", capturedBy, actorID, action, me)
	}
}

// TestOneColleagueHoldsARoleOncePerCompany holds live uniqueness and the end of
// a responsibility: the same staffing twice is a conflict, an ended one leaves
// the list, and ending it again is a conflict rather than a second timestamp.
func TestOneColleagueHoldsARoleOncePerCompany(t *testing.T) {
	e := setupRoleEnv(t)
	ctx := e.actingAs(e.admin, companyCRUD, principal.RowScopeAll)
	company := e.seedCompany(t, e.admin, "workspace")
	in := staffing(company, e.seedColleague(t, "Dana"), e.accountManager(t))

	created, err := e.store.CreateAssignment(ctx, in)
	if err != nil {
		t.Fatalf("staffing a company: %v", err)
	}
	if _, err := e.store.CreateAssignment(ctx, in); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("the same staffing twice: err=%v, want ErrConflict", err)
	}

	if err := e.store.ArchiveAssignment(ctx, ids.UUID(created.Id)); err != nil {
		t.Fatalf("ending the responsibility: %v", err)
	}
	listed, err := e.listCompany(ctx, company)
	if err != nil {
		t.Fatalf("listing after the end: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("an ended responsibility is still listed: %+v", listed)
	}
	if err := e.store.ArchiveAssignment(ctx, ids.UUID(created.Id)); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("ending it twice: err=%v, want ErrConflict", err)
	}
	// Ended is not gone: the row is history, and the same colleague can be
	// staffed again without the old row standing in the way.
	if _, err := e.store.CreateAssignment(ctx, in); err != nil {
		t.Fatalf("staffing again after the end: %v", err)
	}
}

// TestAnAccountTheCallerCannotOpenHasNoTeamToShow holds existence-hiding on
// every verb. The account is owner-private, which not even an all-scope admin
// reads, so the reader is exactly that admin: the strongest seat that must
// still learn nothing.
func TestAnAccountTheCallerCannotOpenHasNoTeamToShow(t *testing.T) {
	e := setupRoleEnv(t)
	owner := e.seedColleague(t, "Owner")
	company := e.seedCompany(t, owner, "owner")
	colleague := e.seedColleague(t, "Dana")
	role := e.accountManager(t)

	theirs, err := e.store.CreateAssignment(
		e.actingAs(owner, companyCRUD, principal.RowScopeOwn), staffing(company, colleague, role))
	if err != nil {
		t.Fatalf("the owner staffing their own private account: %v", err)
	}

	ctx := e.actingAs(e.admin, companyCRUD, principal.RowScopeAll)
	if _, err := e.listCompany(ctx, company); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("listing a team on an account the caller cannot open: err=%v, want ErrNotFound", err)
	}
	if _, err := e.store.CreateAssignment(ctx, staffing(company, e.admin, role)); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("staffing an account the caller cannot open: err=%v, want ErrNotFound", err)
	}
	// The assignment id alone must not be a way in: the parent is read off the
	// stored row, so naming a row on a hidden account answers as if it were absent.
	_, err = e.store.UpdateAssignment(ctx, ids.UUID(theirs.Id), UpdateAssignmentInput{RoleID: &role})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("reassigning through the row id on a hidden account: err=%v, want ErrNotFound", err)
	}
	if err := e.store.ArchiveAssignment(ctx, ids.UUID(theirs.Id)); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("ending through the row id on a hidden account: err=%v, want ErrNotFound", err)
	}
}

// TestSeeingAnAccountIsNotLeaveToStaffIt holds the read/write split. A company
// is workspace-readable, so a rep with own scope lists the team on a
// colleague's account and gets the honest empty answer — and is refused, not
// hidden from, when they try to change it. Both halves of the write gate
// refuse the same way: the row arm for a seat that may update companies but
// not this one, the object arm for a seat holding no update grant at all.
func TestSeeingAnAccountIsNotLeaveToStaffIt(t *testing.T) {
	e := setupRoleEnv(t)
	owner := e.seedColleague(t, "Owner")
	company := e.seedCompany(t, owner, "workspace")
	rep := e.seedColleague(t, "Rep")
	in := staffing(company, rep, e.accountManager(t))

	ctx := e.actingAs(rep, companyCRUD, principal.RowScopeOwn)
	listed, err := e.listCompany(ctx, company)
	if err != nil {
		t.Fatalf("a rep listing the team on a colleague's account: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("an unstaffed account listed %d rows", len(listed))
	}
	if _, err := e.store.CreateAssignment(ctx, in); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("staffing a colleague's account under own scope: err=%v, want ErrPermissionDenied", err)
	}

	readOnly := e.actingAs(e.admin, principal.ObjectGrant{Read: true}, principal.RowScopeAll)
	if _, err := e.store.CreateAssignment(readOnly, in); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("staffing without company.update: err=%v, want ErrPermissionDenied", err)
	}
}

// TestAnArchivedAccountTakesNoNewResponsibility holds the live half of the
// write probe: archived is frozen, and the refusal is not-found because the
// live read paths have already stopped serving the record.
func TestAnArchivedAccountTakesNoNewResponsibility(t *testing.T) {
	e := setupRoleEnv(t)
	ctx := e.actingAs(e.admin, companyCRUD, principal.RowScopeAll)
	company := e.seedCompany(t, e.admin, "workspace")
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE company SET archived_at = now() WHERE id = $1`, company); err != nil {
		t.Fatal(err)
	}
	_, err := e.store.CreateAssignment(ctx, staffing(company, e.seedColleague(t, "Dana"), e.accountManager(t)))
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("staffing an archived account: err=%v, want ErrNotFound", err)
	}
}
