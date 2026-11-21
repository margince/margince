// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration_test

// Who a contact case merge accepts as the owner of the result. The pair merge
// keeps the same set. An invited colleague who has not signed in counts. A
// deactivated seat or an agent does not.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ownerCase is a duplicate case whose first member belongs to the colleague
// and whose other members belong to the rep who reviews it.
type ownerCase struct {
	e       *integration.Env
	store   *contacts.Store
	rep     context.Context
	members []ids.UUID
}

func seedOwnerCase(t *testing.T, size int, number string) ownerCase {
	t.Helper()
	e := integration.Setup(t)
	e.GrantRoleToEach(t, "rep", e.Rep1, e.Rep2)
	store := caseMergeStore(t, e)
	if err := store.SetSharedMergeReviews(e.Admin(), true); err != nil {
		t.Fatalf("switching shared merge reviews: %v", err)
	}
	rep := e.As(e.Rep1, nil, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects: map[string]principal.ObjectGrant{
			"contact":               {Create: true, Read: true, Update: true},
			"relationship":          {Create: true, Read: true, Update: true},
			"installation_settings": {Read: true},
		},
		RowScope: principal.RowScopeOwn,
	})
	actor, _ := principal.Actor(rep)
	actor.SeatType = principal.SeatFull
	c := ownerCase{e: e, store: store, rep: principal.WithActor(rep, actor)}
	for i := range size {
		owner := ids.From[ids.UserKind](e.Rep1)
		if i == 0 {
			owner = ids.From[ids.UserKind](e.Rep2)
		}
		created, err := store.CreateContact(e.Admin(), contacts.CreateContactInput{
			FullName: "Ada Lovelace", Source: "manual", OwnerID: &owner,
			Phones: []contacts.ContactPhoneInput{{Phone: number, PhoneType: "work", IsPrimary: true, Position: 1}},
		})
		if err != nil {
			t.Fatalf("creating a duplicate: %v", err)
		}
		c.members = append(c.members, ids.UUID(created.Id))
	}
	return c
}

func (c ownerCase) seat(t *testing.T, set string) {
	t.Helper()
	if _, err := integration.OwnerConn(t).Exec(context.Background(), `UPDATE app_user SET `+set+` WHERE id = $1`, c.e.Rep2); err != nil {
		t.Fatalf("setting the colleague's seat to %s: %v", set, err)
	}
}

// merge asks for the case merge into the colleague's contact, owned by them.
func (c ownerCase) merge(t *testing.T) (ids.UUID, error) {
	t.Helper()
	read, err := c.store.GetContactCase(c.rep, c.members[0])
	if err != nil {
		t.Fatalf("reading the case: %v", err)
	}
	out, err := c.store.MergeContactCase(c.rep, c.members[0], contacts.ContactCaseDecision{
		Revision: read.Revision, MemberIDs: c.members, SurvivorID: c.members[0], OwnerID: c.e.Rep2,
	})
	return ids.UUID(out.OwnerId), err
}

func TestACaseMergeAcceptsAnInvitedColleagueAsOwnerLikeThePairMerge(t *testing.T) {
	for name, size := range map[string]int{"a case of three": 3, "a case of two": 2} {
		t.Run(name, func(t *testing.T) {
			c := seedOwnerCase(t, size, "+493055509101")
			c.seat(t, "status = 'invited'")
			owner, err := c.merge(t)
			if err != nil {
				t.Fatalf("a case merge into the invited colleague's contact = %v, want it accepted", err)
			}
			if owner != c.e.Rep2 {
				t.Errorf("the case merge kept owner %s, want the invited colleague %s", owner, c.e.Rep2)
			}
		})
	}

	pair := seedOwnerCase(t, 2, "+493055509102")
	pair.seat(t, "status = 'invited'")
	survivor, err := pair.store.MergeContact(pair.rep,
		ids.From[ids.ContactKind](pair.members[1]), ids.From[ids.ContactKind](pair.members[0]), nil)
	if err != nil {
		t.Fatalf("the pair merge into the invited colleague's contact: %v", err)
	}
	if survivor.OwnerId == nil || ids.UUID(*survivor.OwnerId) != pair.e.Rep2 {
		t.Errorf("the pair merge kept owner %v, want the invited colleague", survivor.OwnerId)
	}
}

func TestACaseMergeRefusesADeactivatedOrAgentOwner(t *testing.T) {
	for name, set := range map[string]string{
		"a deactivated seat": "status = 'deactivated'",
		"an agent seat":      "is_agent = true",
	} {
		t.Run(name, func(t *testing.T) {
			c := seedOwnerCase(t, 3, "+493055509103")
			c.seat(t, set)
			_, err := c.merge(t)
			var refusal *contacts.CaseSelectionError
			if !errors.As(err, &refusal) || refusal.Code != "owner_not_eligible" {
				t.Errorf("a case merge owned by %s = %v, want owner_not_eligible", name, err)
			}
		})
	}
}
