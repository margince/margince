// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package org360

import (
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The people section carries 25 of an account's contacts, and which 25 is a
// product decision the roster read does not make: it returns them ordered by
// person id, which is arbitrary as a reading order. Cutting that order keeps
// the first 25 ids rather than the 25 contacts worth looking at.
//
// The fixture puts the ONLY contact who has answered at the end of the id
// order, which is where the old cut dropped them. A reader opening the account
// then saw twenty-five untried strangers, no way in, and a has_more flag that
// truthfully reported more contacts while saying nothing about the one that
// mattered. Person ids are UUIDv7 and therefore ascend in creation order, so
// seeding the answered contact last is what places them outside the old cut.
func TestOrganization360RanksContactsBeforeTruncatingTheSection(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()

	owner := integration.OwnerConn(t)
	org := e.SeedOrg(t, "Brandt GmbH", nil)

	const contacts = 30
	var answered ids.UUID
	for i := range contacts {
		person := e.SeedPerson(t, fmt.Sprintf("Contact %02d", i), nil)
		employ(t, e, person, org, "Fleet")
		answered = person
	}

	// One inbound message: the single fact that separates a way in from a
	// roster. It lands inside the 90-day window ending at the pinned clock.
	mail := integration.AccountMailDirectedAt(t, owner, e.WS, "Re: your proposal",
		"inbound", org360Clock.AddDate(0, 0, -3))
	integration.LinkActivity(t, owner, mail, "person", answered)

	view, err := org360Service(e).Assemble(ctx, ids.OrganizationID{UUID: org})
	if err != nil {
		t.Fatalf("assembling the 360: %v", err)
	}
	if view.People == nil {
		t.Fatal("the people section is absent for a caller who may read it")
	}
	rows := view.People.Data
	if len(rows) != 25 {
		t.Fatalf("the section carried %d contacts, want the 25-row cut", len(rows))
	}
	if !view.People.Page.HasMore {
		t.Fatal("a 30-contact account must report has_more on a 25-row section")
	}

	// The one who answered leads, because they are the way in.
	if got := ids.UUID(rows[0].PersonId); got != answered {
		t.Fatalf("the section opens on %s; the contact who answered is %s and must lead",
			got, answered)
	}
}
