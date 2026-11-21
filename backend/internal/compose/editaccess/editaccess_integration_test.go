// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package editaccess_test

// A colleague who can see a record but not change it asks its owner for edit
// access. The owner answers in their inbox, and approving shares the record
// for writing while the owner stays the owner.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/editaccess"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

const editorRole = `{"objects":{` +
	`"contact":{"create":true,"read":true,"update":true},` +
	`"company":{"create":true,"read":true,"update":true},` +
	`"deal":{"create":true,"read":true,"update":true},` +
	`"lead":{"create":true,"read":true,"update":true},` +
	`"project":{"create":true,"read":true,"update":true},` +
	`"activity":{"create":true,"read":true,"update":true},` +
	`"relationship":{"create":true,"read":true,"update":true},` +
	`"pipeline":{"read":true}},"row_scope":"own"}`

// editorPerms may change every shareable record type, on its own rows only.
func editorPerms() principal.Permissions {
	grant := principal.ObjectGrant{Create: true, Read: true, Update: true}
	return principal.Permissions{
		Objects: map[string]principal.ObjectGrant{
			"contact": grant, "company": grant, "deal": grant, "lead": grant, "project": grant,
			"activity": grant, "relationship": grant, "pipeline": {Read: true},
		},
		RowScope: principal.RowScopeOwn,
	}
}

func seated(e *integration.Env, user ids.UUID, seat principal.SeatType) context.Context {
	ctx := e.As(user, nil, editorPerms())
	p, _ := principal.Actor(ctx)
	p.SeatType = seat
	return principal.WithActor(ctx, p)
}

func grantEditorRole(t *testing.T, e *integration.Env, members ...ids.UUID) {
	t.Helper()
	e.WsExec(t, `INSERT INTO role (key, name, permissions) VALUES ('edit-access-rep', 'Edit access rep', $1::jsonb)
		ON CONFLICT (key) DO NOTHING`, editorRole)
	for _, member := range members {
		e.WsExec(t, `INSERT INTO role_assignment (role_id, user_id) SELECT id, $1 FROM role WHERE key = 'edit-access-rep'`, member)
	}
}

func engine(e *integration.Env) editaccess.Engine {
	return editaccess.New(e.DB(), compose.NewApprovalsService(e.DB()))
}

// seedRecord writes one record of kind owned by owner through its real writer.
func seedRecord(t *testing.T, e *integration.Env, kind string, owner ids.UUID) ids.UUID {
	t.Helper()
	user := ids.From[ids.UserKind](owner)
	switch kind {
	case "contact":
		return e.SeedContact(t, "Ines Brandt", &owner)
	case "company":
		return e.SeedCompany(t, "Brandt Logistik", &owner)
	case "lead":
		return seedLead(t, e, &user)
	case "deal":
		pipeline, open, _ := integration.DealFixture(t, e)
		return e.SeedDeal(t, "Keller renewal", pipeline, open, &owner)
	case "project":
		company := e.SeedCompany(t, "Keller Bau", &owner)
		project, err := e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{
			Name: "Keller rollout", CompanyID: ids.From[ids.CompanyKind](company), OwnerID: &user,
			Source: provenance.RecordSourceManual,
		})
		if err != nil {
			t.Fatalf("seeding a project: %v", err)
		}
		return ids.UUID(project.Id)
	}
	t.Fatalf("no writer for a %s", kind)
	return ids.Nil
}

// seedLead writes a lead through its real writer; a nil owner leaves it unowned.
func seedLead(t *testing.T, e *integration.Env, owner *ids.UserID) ids.UUID {
	t.Helper()
	name := "Jonas Keller"
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{
		FullName: &name, OwnerID: owner, Source: provenance.RecordSourceManual,
	})
	if err != nil {
		t.Fatalf("seeding a lead: %v", err)
	}
	return ids.UUID(lead.Id)
}

func refusalCode(err error) string {
	var refusal *contacts.TransferRefusal
	if errors.As(err, &refusal) {
		return refusal.Code
	}
	return ""
}

func decide(t *testing.T, e *integration.Env, approval ids.ApprovalID, approve bool) {
	t.Helper()
	owner := seated(e, e.Rep1, principal.SeatFull)
	if _, err := compose.NewApprovalsServiceWithEffects(e.Pool).Decide(owner, approval, approve, nil); err != nil {
		t.Fatalf("the owner deciding: %v", err)
	}
}

// Each shareable kind goes from the refusal to a granted write. A second press
// answers the open request. After the owner approves, the colleague may change
// the record, and its owner stays the same.
func TestEditAccessIsAskedAndGivenOnEveryShareableKind(t *testing.T) {
	for _, kind := range []string{"contact", "company", "lead", "deal", "project"} {
		t.Run(kind, func(t *testing.T) {
			e := integration.Setup(t)
			grantEditorRole(t, e, e.Rep1, e.Rep3)
			record := seedRecord(t, e, kind, e.Rep1)
			asker := seated(e, e.Rep3, principal.SeatFull)

			request, created, err := engine(e).Ask(asker, kind, record)
			if err != nil || !created || request.Ask.OwnerID != e.Rep1 || request.Ask.RequesterID != e.Rep3 {
				t.Fatalf("asking: %+v (created %v, err %v), want a new request put to the owner", request, created, err)
			}
			again, created, err := engine(e).Ask(asker, kind, record)
			if err != nil || created || again.ApprovalID != request.ApprovalID {
				t.Fatalf("asking again: %+v (created %v, err %v), want the open request answered", again, created, err)
			}
			decide(t, e, request.ApprovalID, true)

			if n := e.WsCount(t, `SELECT count(*) FROM record_grant WHERE record_type = $1 AND record_id = $2
				AND subject_type = 'user' AND subject_id = $3 AND access = 'write'`, kind, record, e.Rep3); n != 1 {
				t.Errorf("write shares to the colleague who asked = %d, want 1", n)
			}
			if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE action = 'record_share' AND entity_type = $1 AND entity_id = $2`, kind, record); n != 1 {
				t.Errorf("record_share audit rows = %d, want 1", n)
			}
			if owner := e.WsScalar(t, `SELECT owner_id::text FROM `+kind+` WHERE id = $1`, record); owner != e.Rep1.String() {
				t.Errorf("the %s is owned by %s, want its owner unchanged", kind, owner)
			}
			if n := e.WsCount(t, `SELECT count(*) FROM notice WHERE recipient_user_id = $1 AND kind = 'edit_access_accepted'`, e.Rep3); n != 1 {
				t.Errorf("acceptance notices to the colleague who asked = %d, want 1", n)
			}
			if _, _, err := engine(e).Ask(asker, kind, record); refusalCode(err) != "already_yours_to_change" {
				t.Errorf("asking once the share stands: %v, want already_yours_to_change", err)
			}
		})
	}
}

// Declining shares nothing and tells the colleague who asked, who may ask again.
func TestADeclinedEditAccessRequestTellsTheColleagueWhoAsked(t *testing.T) {
	e := integration.Setup(t)
	grantEditorRole(t, e, e.Rep1, e.Rep3)
	lead := seedRecord(t, e, "lead", e.Rep1)
	asker := seated(e, e.Rep3, principal.SeatFull)
	request, _, err := engine(e).Ask(asker, "lead", lead)
	if err != nil {
		t.Fatalf("asking: %v", err)
	}
	decide(t, e, request.ApprovalID, false)

	if n := e.WsCount(t, `SELECT count(*) FROM record_grant WHERE record_id = $1`, lead); n != 0 {
		t.Errorf("shares after a decline = %d, want none", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM notice WHERE recipient_user_id = $1 AND kind = 'edit_access_declined'`, e.Rep3); n != 1 {
		t.Errorf("decline notices to the colleague who asked = %d, want 1", n)
	}
	if _, created, err := engine(e).Ask(asker, "lead", lead); err != nil || !created {
		t.Errorf("asking after a decline: created %v, err %v, want a new request", created, err)
	}
}

// The colleague who asked takes the request back, and it is no longer theirs to read.
func TestAWithdrawnEditAccessRequestLeavesTheInbox(t *testing.T) {
	e := integration.Setup(t)
	grantEditorRole(t, e, e.Rep1, e.Rep3)
	deal := seedRecord(t, e, "deal", e.Rep1)
	asker := seated(e, e.Rep3, principal.SeatFull)
	request, _, err := engine(e).Ask(asker, "deal", deal)
	if err != nil {
		t.Fatalf("asking: %v", err)
	}
	if err := engine(e).Withdraw(asker, "deal", deal); err != nil {
		t.Fatalf("withdrawing: %v", err)
	}
	if _, err := engine(e).Own(asker, "deal", deal); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("reading a withdrawn request: %v, want not found", err)
	}
	if status := e.WsScalar(t, `SELECT status FROM approval WHERE id = $1`, request.ApprovalID); status == "pending" {
		t.Errorf("the withdrawn request is still pending")
	}
}

// Nothing is asked of a record that is the asker's own, that nobody owns, or
// from a read seat.
func TestEditAccessIsNotAskedWhereThereIsNothingToAsk(t *testing.T) {
	e := integration.Setup(t)
	grantEditorRole(t, e, e.Rep1, e.Rep3)
	own := seedRecord(t, e, "lead", e.Rep3)
	colleagues := seedRecord(t, e, "lead", e.Rep1)
	unowned := seedLead(t, e, nil)

	if _, _, err := engine(e).Ask(seated(e, e.Rep3, principal.SeatFull), "lead", own); refusalCode(err) != "already_yours_to_change" {
		t.Errorf("asking about one's own lead: %v, want already_yours_to_change", err)
	}
	if _, _, err := engine(e).Ask(seated(e, e.Rep3, principal.SeatFull), "lead", unowned); refusalCode(err) != "unowned" {
		t.Errorf("asking about an unowned lead: %v, want unowned", err)
	}
	if _, _, err := engine(e).Ask(seated(e, e.Rep3, principal.SeatRead), "lead", colleagues); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("asking from a read seat: %v, want permission denied", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE kind = 'edit_access'`); n != 0 {
		t.Errorf("requests filed = %d, want none", n)
	}
}

// A request cannot be approved once the record has moved to another owner. A
// former owner who may still change it is refused too. The card stays pending,
// so it can still be declined.
func TestEditAccessIsNotGivenOnceTheRecordChangedOwner(t *testing.T) {
	e := integration.Setup(t)
	grantEditorRole(t, e, e.Rep1, e.Rep3)
	lead := seedRecord(t, e, "lead", e.Rep1)
	request, _, err := engine(e).Ask(seated(e, e.Rep3, principal.SeatFull), "lead", lead)
	if err != nil {
		t.Fatalf("asking: %v", err)
	}
	newOwner := ids.From[ids.UserKind](e.Rep2)
	if _, err := e.Contacts.UpdateLead(e.Admin(), ids.From[ids.LeadKind](lead), contacts.UpdateLeadInput{OwnerID: &newOwner}); err != nil {
		t.Fatalf("moving the lead: %v", err)
	}
	perms := editorPerms()
	perms.RowScope = principal.RowScopeAll
	formerOwner := e.As(e.Rep1, nil, perms)
	if _, err := compose.NewApprovalsServiceWithEffects(e.Pool).Decide(formerOwner, request.ApprovalID, true, nil); refusalCode(err) != "owner_changed" {
		t.Errorf("approving after the lead moved: %v, want owner_changed", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM record_grant WHERE record_id = $1`, lead); n != 0 {
		t.Errorf("shares after a refused approval = %d, want none", n)
	}
	if status := e.WsScalar(t, `SELECT status FROM approval WHERE id = $1`, request.ApprovalID); status != "pending" {
		t.Errorf("the refused approval is %s, want it still pending", status)
	}
}
