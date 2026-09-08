// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// leadObjectGrant is what every seat below needs before row scope is even
// asked: the object grant to read and change a lead. The seats differ only in
// RowScope, which is the axis these tests are about.
func leadObjectGrant() map[string]principal.ObjectGrant {
	return map[string]principal.ObjectGrant{
		"lead":   {Create: true, Read: true, Update: true},
		"person": {Create: true, Read: true, Update: true},
	}
}

func leadRepPerms() principal.Permissions {
	return principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects:  leadObjectGrant(),
		RowScope: principal.RowScopeOwn,
	}
}

func leadManagerPerms() principal.Permissions {
	return principal.Permissions{
		RoleKeys: []string{"manager"},
		Objects:  leadObjectGrant(),
		RowScope: principal.RowScopeTeam,
	}
}

// seedOwnerlessLead mints a lead through the real writer and then clears its
// owner, which is the state an inbound capture or a departed rep leaves
// behind. Seeded through CreateLead rather than an INSERT so the row carries
// everything the production writer puts on it.
func seedOwnerlessLead(t *testing.T, e *Env, name string) ids.LeadID {
	t.Helper()
	lead, _, err := e.People.CreateLead(e.Admin(), people.CreateLeadInput{
		FullName: &name,
		Source:   "manual",
	})
	if err != nil {
		t.Fatalf("seeding lead %q: %v", name, err)
	}
	id := ids.From[ids.LeadKind](ids.UUID(lead.Id))
	e.WsExec(t, `UPDATE lead SET owner_id = NULL WHERE id = $1`, ids.UUID(lead.Id))
	return id
}

func assignLead(actor context.Context, e *Env, id ids.LeadID, dest ids.UUID) error {
	owner := ids.From[ids.UserKind](dest)
	_, err := e.People.UpdateLead(actor, id, people.UpdateLeadInput{OwnerID: &owner})
	return err
}

// A lead nobody owns is the queue every inbound lead lands in, and the whole
// point of the queue is that somebody can take work out of it. Three seats,
// three different answers, and the shape of each is what this asserts: a rep
// may take an ownerless lead for THEMSELVES and hand it to nobody else; a
// Team Lead may route one to their own team and no further; an admin may
// place it anywhere a live human seat exists.
func TestAnOwnerlessLeadIsAssignableWithinTheAssignersOwnReach(t *testing.T) {
	e := Setup(t)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, leadRepPerms())
	manager := e.As(e.Rep2, []ids.UUID{e.Team1}, leadManagerPerms())

	t.Run("a rep takes an ownerless lead for themselves", func(t *testing.T) {
		id := seedOwnerlessLead(t, e, "Rep Self Pickup")
		if err := assignLead(rep, e, id, e.Rep1); err != nil {
			t.Fatalf("a rep assigning an ownerless lead to themselves → %v, want allowed", err)
		}
		got, err := e.People.GetLead(rep, id, 0)
		if err != nil {
			t.Fatalf("reading the lead back: %v", err)
		}
		if got.OwnerId == nil || ids.UUID(*got.OwnerId) != e.Rep1 {
			t.Errorf("owner after self-assignment = %v, want Rep1", got.OwnerId)
		}
	})

	t.Run("a rep may not hand an ownerless lead to a teammate", func(t *testing.T) {
		id := seedOwnerlessLead(t, e, "Rep Hands Off")
		err := assignLead(rep, e, id, e.Rep2)
		if !errors.As(err, new(*auth.AssigneeNotAllowedError)) {
			t.Fatalf("a rep assigning an ownerless lead to a teammate → %v, want AssigneeNotAllowedError", err)
		}
		mustStayUnowned(t, e, id)
	})

	t.Run("a Team Lead routes an ownerless lead to a teammate", func(t *testing.T) {
		id := seedOwnerlessLead(t, e, "Manager Routes")
		if err := assignLead(manager, e, id, e.Rep1); err != nil {
			t.Fatalf("a manager assigning an ownerless lead to a live teammate → %v, want allowed", err)
		}
		got, err := e.People.GetLead(manager, id, 0)
		if err != nil {
			t.Fatalf("reading the lead back: %v", err)
		}
		if got.OwnerId == nil || ids.UUID(*got.OwnerId) != e.Rep1 {
			t.Errorf("owner after the manager's assignment = %v, want Rep1", got.OwnerId)
		}
	})

	t.Run("a Team Lead may not route outside their own team", func(t *testing.T) {
		id := seedOwnerlessLead(t, e, "Manager Overreaches")
		err := assignLead(manager, e, id, e.Rep3)
		if !errors.As(err, new(*auth.AssigneeNotAllowedError)) {
			t.Fatalf("a manager assigning to a user on another team → %v, want AssigneeNotAllowedError", err)
		}
		mustStayUnowned(t, e, id)
	})

	t.Run("an admin places an ownerless lead anywhere", func(t *testing.T) {
		id := seedOwnerlessLead(t, e, "Admin Places")
		if err := assignLead(e.Admin(), e, id, e.Rep3); err != nil {
			t.Fatalf("an admin assigning an ownerless lead across teams → %v, want allowed", err)
		}
	})
}

// The destination is a seat, not a row in app_user. A suspended colleague, an
// archived one and an agent are all present in the table and none of them can
// do lead work, so handing them a lead files it where nobody will answer it.
func TestALeadIsNeverAssignedToASeatThatCannotWorkIt(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()

	cases := []struct {
		name    string
		prepare func(t *testing.T, target ids.UUID)
	}{
		{"suspended", func(t *testing.T, target ids.UUID) {
			e.WsExec(t, `UPDATE app_user SET status = 'suspended' WHERE id = $1`, target)
		}},
		{"archived", func(t *testing.T, target ids.UUID) {
			e.WsExec(t, `UPDATE app_user SET archived_at = now() WHERE id = $1`, target)
		}},
		{"an agent", func(t *testing.T, target ids.UUID) {
			e.WsExec(t, `UPDATE app_user SET is_agent = true WHERE id = $1`, target)
		}},
		{"a read seat", func(t *testing.T, target ids.UUID) {
			e.WsExec(t, `UPDATE app_user SET seat_type = 'read' WHERE id = $1`, target)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name+" is refused as a destination", func(t *testing.T) {
			id := seedOwnerlessLead(t, e, "Dest "+tc.name)
			// Restored after the case so the seats stay shared fixtures.
			before := e.WsCount(t, `SELECT count(*) FROM app_user WHERE id = $1`, e.Rep3)
			if before != 1 {
				t.Fatalf("Rep3 is not a seeded seat, so this case proves nothing")
			}
			tc.prepare(t, e.Rep3)
			defer e.WsExec(t, `UPDATE app_user SET status = 'active', archived_at = NULL,
				is_agent = false, seat_type = 'full' WHERE id = $1`, e.Rep3)

			err := assignLead(admin, e, id, e.Rep3)
			if !errors.As(err, new(*auth.AssigneeNotAllowedError)) {
				t.Fatalf("assigning to a %s seat → %v, want AssigneeNotAllowedError", tc.name, err)
			}
			mustStayUnowned(t, e, id)
		})
	}
}

// The ownerless door is for ASSIGNMENT and nothing else. Without this, a seat
// that may not touch the lead could ride a score or a status change in beside
// the owner and rewrite a record it has no authority over.
func TestTheOwnerlessDoorCarriesOwnershipAndNoOtherField(t *testing.T) {
	e := Setup(t)
	manager := e.As(e.Rep2, []ids.UUID{e.Team1}, leadManagerPerms())
	id := seedOwnerlessLead(t, e, "Mixed Patch")

	owner := ids.From[ids.UserKind](e.Rep1)
	title := "VP Engineering"
	_, err := e.People.UpdateLead(manager, id, people.UpdateLeadInput{
		OwnerID: &owner,
		Title:   &title,
	})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("an owner change carrying a title on an ownerless lead → %v, want ErrPermissionDenied", err)
	}
	mustStayUnowned(t, e, id)
	if n := e.WsCount(t, `SELECT count(*) FROM lead WHERE id = $1 AND title = $2`, id.UUID, title); n != 0 {
		t.Errorf("the refused patch wrote the title anyway")
	}
}

// A lead that is already somebody's stays theirs. The ownerless arm widens the
// door for a record nobody owns, and this is the assertion that it widened
// nothing else: a stranger's lead answers the same refusal it always did.
func TestAnotherSeatsLeadIsStillNotAssignableByAStranger(t *testing.T) {
	e := Setup(t)
	stranger := e.As(e.Rep3, []ids.UUID{e.Team2}, leadRepPerms())

	name := "Owned By Rep1"
	ownedBy := ids.From[ids.UserKind](e.Rep1)
	lead, _, err := e.People.CreateLead(e.Admin(), people.CreateLeadInput{
		FullName: &name, Source: "manual", OwnerID: &ownedBy,
	})
	if err != nil {
		t.Fatalf("seeding an owned lead: %v", err)
	}
	id := ids.From[ids.LeadKind](ids.UUID(lead.Id))

	if err := assignLead(stranger, e, id, e.Rep3); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a stranger taking a colleague's lead → %v, want ErrPermissionDenied", err)
	}
	got, err := e.People.GetLead(e.Admin(), id, 0)
	if err != nil {
		t.Fatalf("reading the lead back: %v", err)
	}
	if got.OwnerId == nil || ids.UUID(*got.OwnerId) != e.Rep1 {
		t.Errorf("owner after the refused assignment = %v, want Rep1 untouched", got.OwnerId)
	}
}

// The create path is the second way an owner reaches a lead, and it is the one
// a plan that only guarded the update would have missed. CSV and mirror
// imports reach the same body through CreateLeadTx.
func TestACreatedLeadCannotNameAnIneligibleOwner(t *testing.T) {
	e := Setup(t)
	e.WsExec(t, `UPDATE app_user SET status = 'suspended' WHERE id = $1`, e.Rep3)
	defer e.WsExec(t, `UPDATE app_user SET status = 'active' WHERE id = $1`, e.Rep3)

	name := "Created For A Suspended Seat"
	owner := ids.From[ids.UserKind](e.Rep3)
	_, _, err := e.People.CreateLead(e.Admin(), people.CreateLeadInput{
		FullName: &name, Source: "manual", OwnerID: &owner,
	})
	if !errors.As(err, new(*auth.AssigneeNotAllowedError)) {
		t.Fatalf("creating a lead owned by a suspended seat → %v, want AssigneeNotAllowedError", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM lead WHERE full_name = $1`, name); n != 0 {
		t.Errorf("the refused create landed %d lead rows, want 0", n)
	}
}

func mustStayUnowned(t *testing.T, e *Env, id ids.LeadID) {
	t.Helper()
	if n := e.WsCount(t, `SELECT count(*) FROM lead WHERE id = $1 AND owner_id IS NULL`, id.UUID); n != 1 {
		t.Errorf("the lead did not stay unowned after a refused assignment")
	}
}

// Two seats reaching for the same ownerless lead: one gets it, and the other
// does not quietly take it away again.
//
// The gate turns on whether the lead is ownerless, so that fact has to be read
// under the row lock. Read unlocked, both callers see "nobody owns this", both
// pass, and the second write lands on a lead the first had just taken — a
// record the loser could not otherwise have touched at all.
func TestTwoSeatsRacingForOneOwnerlessLeadDoNotBothWin(t *testing.T) {
	e := Setup(t)
	id := seedOwnerlessLead(t, e, "Contested")
	first := e.As(e.Rep1, []ids.UUID{e.Team1}, leadRepPerms())
	second := e.As(e.Rep3, []ids.UUID{e.Team2}, leadRepPerms())

	var running sync.WaitGroup
	errs := make([]error, 2)
	start := make(chan struct{})
	for slot, actor := range []context.Context{first, second} {
		running.Add(1)
		go func(slot int, actor context.Context, dest ids.UUID) {
			defer running.Done()
			<-start
			errs[slot] = assignLead(actor, e, id, dest)
		}(slot, actor, []ids.UUID{e.Rep1, e.Rep3}[slot])
	}
	close(start)
	running.Wait()

	// Whoever the row ended up with, it is one of the two and the OTHER one's
	// call must not have reported success: a refused assignment that answers
	// nil is the shape that loses a lead silently.
	if e.WsCount(t, `SELECT count(*) FROM lead WHERE id = $1 AND owner_id IS NULL`, id.UUID) == 1 {
		t.Fatal("the contested lead ended ownerless: neither racer took it")
	}
	winner := 0
	if e.WsCount(t, `SELECT count(*) FROM lead WHERE id = $1 AND owner_id = $2`, id.UUID, e.Rep3) == 1 {
		winner = 1
	}
	if errs[winner] != nil {
		t.Errorf("the seat that owns the lead reported %v, want success", errs[winner])
	}
	if errs[1-winner] == nil {
		t.Errorf("the seat that did NOT get the lead reported success: " +
			"the other call wrote nothing and said it had")
	}
}
