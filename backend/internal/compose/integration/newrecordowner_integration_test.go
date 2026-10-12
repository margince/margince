// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// bornRecord is one record kind's create, named by the row it should not
// leave behind. Every kind that lets a caller name an owner is here: the gap
// this pins was four kinds wide precisely because each was read on its own.
type bornRecord struct {
	kind   string
	create func(t *testing.T, e *Env, owner ids.UserID) (ids.UUID, error)
	// resave is the edit form's ordinary save: the whole record back, owner
	// included and unchanged. Nothing is handed on, so nothing is assigned.
	resave func(t *testing.T, e *Env, id ids.UUID, owner ids.UserID) error
	// landed counts the rows the refused create must not have written, by the
	// name it was given — the row itself, not an audit or event row, because a
	// create that failed the gate never reached the write shape at all.
	landed func(t *testing.T, e *Env, name string) int
}

const bornRecordName = "Born For An Ineligible Seat"

func bornRecords(t *testing.T, e *Env) []bornRecord {
	t.Helper()
	pipeline, open, _ := DealFixture(t, e)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Anchor For A Born Project", nil))
	return []bornRecord{{
		kind: "contact",
		create: func(t *testing.T, e *Env, owner ids.UserID) (ids.UUID, error) {
			out, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
				FullName: bornRecordName, Source: "manual", OwnerID: &owner,
			})
			return ids.UUID(out.Id), err
		},
		resave: func(t *testing.T, e *Env, id ids.UUID, owner ids.UserID) error {
			name := bornRecordName
			_, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](id), contacts.UpdateContactInput{
				FullName: &name, OwnerID: &owner, Source: "manual",
			})
			return err
		},
		landed: func(t *testing.T, e *Env, name string) int {
			return e.WsCount(t, `SELECT count(*) FROM contact WHERE full_name = $1`, name)
		},
	}, {
		kind: "company",
		create: func(t *testing.T, e *Env, owner ids.UserID) (ids.UUID, error) {
			out, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{
				DisplayName: bornRecordName, OwnerID: &owner,
			})
			return ids.UUID(out.Id), err
		},
		resave: func(t *testing.T, e *Env, id ids.UUID, owner ids.UserID) error {
			name := bornRecordName
			_, err := e.Contacts.UpdateCompany(e.Admin(), ids.From[ids.CompanyKind](id), contacts.UpdateCompanyInput{
				DisplayName: &name, OwnerID: &owner,
			})
			return err
		},
		landed: func(t *testing.T, e *Env, name string) int {
			return e.WsCount(t, `SELECT count(*) FROM company WHERE display_name = $1`, name)
		},
	}, {
		kind: "deal",
		create: func(t *testing.T, e *Env, owner ids.UserID) (ids.UUID, error) {
			out, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
				Name: bornRecordName, PipelineID: pipeline, StageID: open, OwnerID: &owner,
			})
			return ids.UUID(out.Id), err
		},
		resave: func(t *testing.T, e *Env, id ids.UUID, owner ids.UserID) error {
			name := bornRecordName
			_, err := e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](id), deals.UpdateDealInput{
				Name: &name, OwnerID: &owner,
			})
			return err
		},
		landed: func(t *testing.T, e *Env, name string) int {
			return e.WsCount(t, `SELECT count(*) FROM deal WHERE name = $1`, name)
		},
	}, {
		kind: "project",
		create: func(t *testing.T, e *Env, owner ids.UserID) (ids.UUID, error) {
			out, err := e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{
				Name: bornRecordName, CompanyID: company, Source: "manual", OwnerID: &owner,
			})
			return ids.UUID(out.Id), err
		},
		resave: func(t *testing.T, e *Env, id ids.UUID, owner ids.UserID) error {
			name := bornRecordName
			_, err := e.Projects.UpdateProject(e.Admin(), ids.From[ids.ProjectKind](id), projects.UpdateProjectInput{
				Name: &name, OwnerID: &owner,
			})
			return err
		},
		landed: func(t *testing.T, e *Env, name string) int {
			return e.WsCount(t, `SELECT count(*) FROM project WHERE name = $1`, name)
		},
	}}
}

// Naming an owner on a create is an assignment, and it is refused for the same
// seats an ownership change is refused for.
//
// The foreign key proves only that the seat exists. Without this the owner
// picker's own rule stopped at the update path: a suspended colleague could not
// be handed a contact, and a contact could be created owned by them — which is
// the same record in the same state, reached by the other door.
func TestANewRecordCannotBeBornToAnIneligibleOwner(t *testing.T) {
	e := Setup(t)
	e.WsExec(t, `UPDATE app_user SET status = 'suspended' WHERE id = $1`, e.Rep3)
	defer e.WsExec(t, `UPDATE app_user SET status = 'active' WHERE id = $1`, e.Rep3)

	owner := ids.From[ids.UserKind](e.Rep3)
	for _, rec := range bornRecords(t, e) {
		t.Run(rec.kind, func(t *testing.T) {
			_, err := rec.create(t, e, owner)
			if !errors.As(err, new(*auth.AssigneeNotAllowedError)) {
				t.Fatalf("creating a %s owned by a suspended seat → %v, want AssigneeNotAllowedError", rec.kind, err)
			}
			if n := rec.landed(t, e, bornRecordName); n != 0 {
				t.Errorf("the refused %s create landed %d row(s), want 0", rec.kind, n)
			}
		})
	}
}

// An invited colleague may own a record created for them — the one way a new
// record's owner is judged more loosely than an existing record's handover
// (auth.EnsureNewRecordOwner). An import files a colleague's book before they
// have ever signed in, and refusing that would leave the rows on whoever ran
// the import.
func TestANewRecordMayBeBornToAnInvitedColleague(t *testing.T) {
	e := Setup(t)
	e.WsExec(t, `UPDATE app_user SET status = 'invited' WHERE id = $1`, e.Rep3)
	defer e.WsExec(t, `UPDATE app_user SET status = 'active' WHERE id = $1`, e.Rep3)

	owner := ids.From[ids.UserKind](e.Rep3)
	for _, rec := range bornRecords(t, e) {
		t.Run(rec.kind, func(t *testing.T) {
			if _, err := rec.create(t, e, owner); err != nil {
				t.Fatalf("creating a %s owned by an invited seat → %v, want it created", rec.kind, err)
			}
			if n := rec.landed(t, e, bornRecordName); n != 1 {
				t.Errorf("%d %s row(s) with the invited owner, want 1", n, rec.kind)
			}
		})
	}
}

// A record whose owner has since been suspended is still editable, because
// re-stating the owner an edit form already carries hands nothing on.
//
// Both halves are one rule (auth.EnsureOwnerHandOn), and holding only the
// refusing half turns a colleague's departure into a freeze: every project and
// deal they owned refuses the ordinary save that would have moved it on,
// because the form sends owner_id back whether or not the user touched it.
func TestAnEditThatRestatesTheOwnerHandsNothingOn(t *testing.T) {
	e := Setup(t)
	owner := ids.From[ids.UserKind](e.Rep3)
	born := map[string]ids.UUID{}
	records := bornRecords(t, e)
	for _, rec := range records {
		id, err := rec.create(t, e, owner)
		if err != nil {
			t.Fatalf("seeding a %s owned by an active seat: %v", rec.kind, err)
		}
		born[rec.kind] = id
	}

	e.WsExec(t, `UPDATE app_user SET status = 'suspended' WHERE id = $1`, e.Rep3)
	defer e.WsExec(t, `UPDATE app_user SET status = 'active' WHERE id = $1`, e.Rep3)

	for _, rec := range records {
		t.Run(rec.kind, func(t *testing.T) {
			if err := rec.resave(t, e, born[rec.kind], owner); err != nil {
				t.Fatalf("saving a %s whose owner was suspended → %v, want it saved", rec.kind, err)
			}
		})
	}
}
