// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package org360

import (
	"errors"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	org360svc "github.com/margince/margince/backend/internal/compose/org360"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The whole account pages, and the ranking survives the page boundary.
//
// This is the read the 25-row section could not give: an account with more
// contacts than a summary carries, walked end to end without losing anybody or
// naming anybody twice. The contact who answered is seeded LAST so that a read
// which paged in id order would bury them on the final page.
func TestContactPageWalksTheWholeAccountInRankedOrder(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	owner := integration.OwnerConn(t)
	svc := org360Service(e)

	org := e.SeedOrg(t, "Brandt GmbH", nil)
	const contacts = 30
	var answered ids.UUID
	for i := range contacts {
		person := e.SeedPerson(t, fmt.Sprintf("Contact %02d", i), nil)
		employ(t, e, person, org, "Fleet")
		answered = person
	}
	mail := integration.AccountMailDirectedAt(t, owner, e.WS, "Re: your proposal",
		"inbound", org360Clock.AddDate(0, 0, -3))
	integration.LinkActivity(t, owner, mail, "person", answered)

	limit := 10
	seen := map[ids.UUID]bool{}
	var cursor *string
	var first ids.UUID
	for page := 0; page < 5; page++ {
		got, err := svc.ContactPage(ctx, ids.OrganizationID{UUID: org},
			org360svc.ContactListQuery{Limit: &limit, Cursor: cursor})
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		if page == 0 {
			first = ids.UUID(got.Data[0].PersonId)
		}
		for _, row := range got.Data {
			id := ids.UUID(row.PersonId)
			if seen[id] {
				t.Fatalf("page %d repeats contact %s — a reader would write to them twice", page, id)
			}
			seen[id] = true
		}
		if !got.Page.HasMore {
			break
		}
		if got.Page.NextCursor == nil {
			t.Fatal("has_more is set with no cursor to follow — the rest of the account is unreachable")
		}
		cursor = got.Page.NextCursor
	}
	if len(seen) != contacts {
		t.Fatalf("walked %d contacts of %d — the pages do not cover the account", len(seen), contacts)
	}
	if first != answered {
		t.Fatalf("the first page opens on %s; the contact who answered is %s and the ranking must lead with them",
			first, answered)
	}
}

// Each engagement state is its own filter, and the three partition the account.
func TestContactPageNarrowsByEngagement(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	owner := integration.OwnerConn(t)
	svc := org360Service(e)

	org := e.SeedOrg(t, "Brandt GmbH", nil)
	answered := e.SeedPerson(t, "Dietmar Rietsch", nil)
	silent := e.SeedPerson(t, "Philipp Koenigs", nil)
	untried := e.SeedPerson(t, "Ute Sommer", nil)
	for _, p := range []ids.UUID{answered, silent, untried} {
		employ(t, e, p, org, "Fleet")
	}
	in := integration.AccountMailDirectedAt(t, owner, e.WS, "Re: proposal", "inbound",
		org360Clock.AddDate(0, 0, -3))
	integration.LinkActivity(t, owner, in, "person", answered)
	out := integration.AccountMailDirectedAt(t, owner, e.WS, "Introduction", "outbound",
		org360Clock.AddDate(0, 0, -10))
	integration.LinkActivity(t, owner, out, "person", silent)

	for _, tc := range []struct {
		state people.Engagement
		want  ids.UUID
	}{
		{people.EngagementAnswered, answered},
		{people.EngagementNoReply, silent},
		{people.EngagementUntried, untried},
	} {
		state := tc.state
		got, err := svc.ContactPage(ctx, ids.OrganizationID{UUID: org},
			org360svc.ContactListQuery{Status: &state})
		if err != nil {
			t.Fatalf("filtering by %s: %v", state, err)
		}
		if len(got.Data) != 1 {
			t.Fatalf("%s returned %d contacts, want exactly the one seeded in that state",
				state, len(got.Data))
		}
		if id := ids.UUID(got.Data[0].PersonId); id != tc.want {
			t.Fatalf("%s returned %s, want %s", state, id, tc.want)
		}
		if got.Data[0].Engagement != crmcontracts.ContactEngagement(state) {
			t.Fatalf("%s returned a row labelled %s", state, got.Data[0].Engagement)
		}
	}
}

// A search matches the name or the title, and drops everyone else.
func TestContactPageSearchesNameAndTitle(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	svc := org360Service(e)

	org := e.SeedOrg(t, "Brandt GmbH", nil)
	byName := e.SeedPerson(t, "Dietmar Rietsch", nil)
	byTitle := e.SeedPerson(t, "Ute Sommer", nil)
	neither := e.SeedPerson(t, "Jan Roth", nil)
	employ(t, e, byName, org, "Fleet")
	employ(t, e, byTitle, org, "Chief Financial Officer")
	employ(t, e, neither, org, "Workshop")

	// The title lives on the person, not the employment edge, so set it there.
	e.WsExec(t, `UPDATE person SET title = 'Chief Financial Officer' WHERE id = $1`, byTitle)

	for _, tc := range []struct {
		needle string
		want   ids.UUID
	}{
		{"rietsch", byName},
		{"financial", byTitle},
	} {
		needle := tc.needle
		got, err := svc.ContactPage(ctx, ids.OrganizationID{UUID: org},
			org360svc.ContactListQuery{Query: &needle})
		if err != nil {
			t.Fatalf("searching %q: %v", needle, err)
		}
		if len(got.Data) != 1 {
			t.Fatalf("%q matched %d contacts, want 1", needle, len(got.Data))
		}
		if id := ids.UUID(got.Data[0].PersonId); id != tc.want {
			t.Fatalf("%q matched %s, want %s", needle, id, tc.want)
		}
	}
}

// A cursor minted under one order is refused by another.
//
// Every order here is derived in Go, so a token replayed against a different one
// names a position that order never had: the page would resume from the wrong
// place and skip contacts silently. Refusing is the only honest answer.
func TestContactPageRefusesACursorFromAnotherOrder(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	svc := org360Service(e)

	org := e.SeedOrg(t, "Brandt GmbH", nil)
	for i := range 5 {
		person := e.SeedPerson(t, fmt.Sprintf("Contact %02d", i), nil)
		employ(t, e, person, org, "Fleet")
	}
	limit := 2
	first, err := svc.ContactPage(ctx, ids.OrganizationID{UUID: org},
		org360svc.ContactListQuery{Limit: &limit})
	if err != nil {
		t.Fatal(err)
	}
	if first.Page.NextCursor == nil {
		t.Fatal("the fixture did not produce a second page")
	}
	_, err = svc.ContactPage(ctx, ids.OrganizationID{UUID: org}, org360svc.ContactListQuery{
		Limit: &limit, Cursor: first.Page.NextCursor, Sort: "name",
	})
	if err == nil {
		t.Fatal("a cursor minted under the recommended order was accepted by the name order")
	}
}

// An account this caller cannot see is not found, not forbidden: the list must
// not become the way to learn that a company exists.
//
// The gate is GetOrganizationTx, the same call the 360 opens with — so this
// asserts the list is behind it rather than re-proving the row scope that read
// already owns.
func TestContactPageHidesAnAccountItCannotRead(t *testing.T) {
	e := integration.Setup(t)
	svc := org360Service(e)

	_, err := svc.ContactPage(e.Admin(), ids.OrganizationID{UUID: ids.NewV7()},
		org360svc.ContactListQuery{})
	if err == nil {
		t.Fatal("an account that does not exist answered rather than hiding")
	}
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("got %v, want not-found so existence stays hidden", err)
	}
}
