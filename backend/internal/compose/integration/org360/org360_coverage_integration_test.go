// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package org360

import (
	"errors"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The counts describe the WHOLE account, not a page of it.
//
// A coverage figure taken from a page is a figure about the page, and the
// reader cannot tell the two apart — so an account with more contacts than any
// page carries must still count all of them.
func TestCoverageCountsTheWholeAccount(t *testing.T) {
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
	mail := integration.AccountMailDirectedAt(t, owner, e.WS, "Re: proposal",
		"inbound", org360Clock.AddDate(0, 0, -3))
	integration.LinkActivity(t, owner, mail, "person", answered)

	got, err := svc.Coverage(ctx, ids.OrganizationID{UUID: org})
	if err != nil {
		t.Fatalf("reading coverage: %v", err)
	}
	if got.Summary.ContactsTotal != contacts {
		t.Fatalf("counted %d contacts, want %d — the summary is reading a page",
			got.Summary.ContactsTotal, contacts)
	}
	if got.Summary.Answered != 1 || got.Summary.Untried != contacts-1 {
		t.Fatalf("answered=%d untried=%d, want 1 and %d",
			got.Summary.Answered, got.Summary.Untried, contacts-1)
	}
	// The way in is the contact who answered, by the same ranking the list uses.
	if got.BestWayIn == nil {
		t.Fatal("no way in named on an account where somebody answered")
	}
	if id := ids.UUID(got.BestWayIn.PersonId); id != answered {
		t.Fatalf("the way in is %s, want the contact who answered (%s)", id, answered)
	}
}

// An account where everyone was written to and nobody replied has no way IN to
// name. Following up again is a decision the reader makes, not a route the page
// recommends, and naming the least-cold contact would dress one up as the other.
func TestCoverageNamesNoWayInWhenNobodyHasAnswered(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	owner := integration.OwnerConn(t)
	svc := org360Service(e)

	org := e.SeedOrg(t, "Brandt GmbH", nil)
	person := e.SeedPerson(t, "Philipp Koenigs", nil)
	employ(t, e, person, org, "CFO")
	out := integration.AccountMailDirectedAt(t, owner, e.WS, "Introduction",
		"outbound", org360Clock.AddDate(0, 0, -10))
	integration.LinkActivity(t, owner, out, "person", person)

	got, err := svc.Coverage(ctx, ids.OrganizationID{UUID: org})
	if err != nil {
		t.Fatal(err)
	}
	if got.BestWayIn != nil {
		t.Fatalf("named %s as a way in on an account where nobody has answered",
			got.BestWayIn.FullName)
	}
	if got.Summary.NoReply != 1 {
		t.Fatalf("no_reply=%d, want 1", got.Summary.NoReply)
	}
}

// A missing champion is reported only when the committee could be read whole.
func TestCoverageReportsAChampionGapOnACompleteCommittee(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	svc := org360Service(e)

	org := e.SeedOrg(t, "Brandt GmbH", nil)
	pipeline, openStage, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Retrofit 2026", pipeline, openStage, nil)
	buyer := e.SeedPerson(t, "Ute Sommer", nil)
	employ(t, e, buyer, org, "Procurement")
	// The deal has to belong to this account for the coverage read to find it.
	e.WsExec(t, `UPDATE deal SET organization_id = $1 WHERE id = $2`, org, deal)
	e.WsExec(t, `INSERT INTO relationship (kind, person_id, deal_id, role, source, captured_by)
		VALUES ('deal_stakeholder', $1, $2, 'economic_buyer', 'manual', 'human:x')`, buyer, deal)

	got, err := svc.Coverage(ctx, ids.OrganizationID{UUID: org})
	if err != nil {
		t.Fatalf("reading coverage: %v", err)
	}
	if got.Committee == nil {
		t.Fatal("no committee for a caller who may read the deal")
	}
	if got.Committee.UnlistedSeats != 0 {
		t.Fatalf("%d seats unlisted for an admin who can see every person",
			got.Committee.UnlistedSeats)
	}
	// The economic buyer is held, the champion is not.
	if len(got.Committee.Gaps) != 1 || got.Committee.Gaps[0] != "champion" {
		t.Fatalf("gaps = %v, want exactly [champion]", got.Committee.Gaps)
	}
	if len(got.Committee.Seats) != 1 || got.Committee.Seats[0].Role != "economic_buyer" {
		t.Fatalf("seats = %+v, want the one economic buyer", got.Committee.Seats)
	}
	if !got.Completeness.CommitteeRead {
		t.Fatal("committee_read is false on a committee that was read")
	}
}

// An account the caller cannot see answers not-found, so this read does not
// become the way to learn that a company exists.
func TestCoverageHidesAnAccountItCannotRead(t *testing.T) {
	e := integration.Setup(t)
	svc := org360Service(e)

	_, err := svc.Coverage(e.Admin(), ids.OrganizationID{UUID: ids.NewV7()})
	if err == nil {
		t.Fatal("an account that does not exist answered rather than hiding")
	}
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("got %v, want not-found", err)
	}
}

// An account with no open deal has no committee to read, and says so as a
// COMPLETE answer rather than a refused one — there is no deal to hold a
// committee, which is different from being unable to look.
func TestCoverageSeparatesNoDealFromNoAccess(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	svc := org360Service(e)

	org := e.SeedOrg(t, "Brandt GmbH", nil)
	person := e.SeedPerson(t, "Jan Roth", nil)
	employ(t, e, person, org, "Workshop")

	got, err := svc.Coverage(ctx, ids.OrganizationID{UUID: org})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Completeness.CommitteeRead {
		t.Fatal("an account with no open deal must read as complete, not refused")
	}
	if got.Committee != nil {
		t.Fatal("a committee was reported for an account with no open deal")
	}
	if got.SelectedDealId != nil {
		t.Fatal("a deal was selected on an account with none open")
	}
}

// The unlisted-seat count is what keeps a gap honest.
//
// deals.Stakeholders applies the person row scope itself, so a seat whose
// holder the reader cannot see is absent from the slice rather than anonymous.
// If gaps were computed from that slice alone, a champion the reader may not
// see would read as NO champion — a hole that does not exist. seatCount reads
// the true total, and gaps stay empty whenever the two disagree.
//
// Seeded by archiving nothing and hiding nobody: this repo's access model lets
// every reader see every person, so an unlisted seat is reachable here only by
// removing the person row the seat points at. That is the shape the guard has
// to survive, and it is why the count is read separately rather than taken
// from len(seats).
func TestCoverageReportsNoGapWhenASeatIsUnlisted(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	svc := org360Service(e)

	org := e.SeedOrg(t, "Brandt GmbH", nil)
	pipeline, openStage, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Retrofit 2026", pipeline, openStage, nil)
	e.WsExec(t, `UPDATE deal SET organization_id = $1 WHERE id = $2`, org, deal)

	champion := e.SeedPerson(t, "Dietmar Rietsch", nil)
	employ(t, e, champion, org, "Managing Director")
	e.WsExec(t, `INSERT INTO relationship (kind, person_id, deal_id, role, source, captured_by)
		VALUES ('deal_stakeholder', $1, $2, 'champion', 'manual', 'human:x')`, champion, deal)
	// The seat outlives the person row it names: the stakeholder read joins
	// person and drops it, while the count still sees the relationship.
	e.WsExec(t, `UPDATE person SET archived_at = now() WHERE id = $1`, champion)

	got, err := svc.Coverage(ctx, ids.OrganizationID{UUID: org})
	if err != nil {
		t.Fatalf("reading coverage: %v", err)
	}
	if got.Committee == nil {
		t.Fatal("no committee for a caller who may read the deal")
	}
	if got.Committee.UnlistedSeats != 1 {
		t.Fatalf("unlisted_seats = %d, want 1 — the seat whose holder was dropped",
			got.Committee.UnlistedSeats)
	}
	if len(got.Committee.Gaps) != 0 {
		t.Fatalf("gaps = %v over a partial committee; the deal HAS a champion seat this read could not list",
			got.Committee.Gaps)
	}
}

// A reader without the deal grant learns nothing about the deals.
//
// Row scope answers WHICH deals, never WHETHER this caller may ask: a caller
// holding organization, person and relationship but not deal was being served
// deal names and the committee on them, because the read reached for
// scopeClause and never for the object grant behind it.
func TestCoverageWithholdsDealsFromAReaderWithoutTheGrant(t *testing.T) {
	e := integration.Setup(t)
	svc := org360Service(e)

	org := e.SeedOrg(t, "Brandt GmbH", nil)
	pipeline, openStage, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Retrofit 2026", pipeline, openStage, nil)
	e.WsExec(t, `UPDATE deal SET organization_id = $1 WHERE id = $2`, org, deal)
	buyer := e.SeedPerson(t, "Ute Sommer", nil)
	employ(t, e, buyer, org, "Procurement")
	e.WsExec(t, `INSERT INTO relationship (kind, person_id, deal_id, role, source, captured_by)
		VALUES ('deal_stakeholder', $1, $2, 'economic_buyer', 'manual', 'human:x')`, buyer, deal)

	got, err := svc.Coverage(e.As(e.Rep1, []ids.UUID{e.Team1}, org360NoDealPerms),
		ids.OrganizationID{UUID: org})
	if err != nil {
		t.Fatalf("the read failed rather than withholding: %v", err)
	}
	if len(got.Deals) != 0 {
		t.Fatalf("named %d deal(s) to a reader with no deal grant: %+v",
			len(got.Deals), got.Deals)
	}
	if got.Committee != nil {
		t.Fatal("served a committee to a reader who may not read the deal it sits on")
	}
	if got.Completeness.CommitteeRead {
		t.Fatal("claimed the committee was read when the caller may not read deals")
	}
	// The rest of the account still answers: a withheld section is not a
	// refused page.
	if got.Summary.ContactsTotal != 1 {
		t.Fatalf("contacts_total = %d, want the roster to survive the withholding",
			got.Summary.ContactsTotal)
	}
}

// A seat carries who on our side can reach it, from the same reader the 360's
// people section uses. A second route ranking would let the map and the roster
// disagree about which colleague to ask.
func TestCoverageSeatsCarryTheirRoutes(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	owner := integration.OwnerConn(t)
	svc := org360Service(e)

	org := e.SeedOrg(t, "Brandt GmbH", nil)
	pipeline, openStage, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Retrofit 2026", pipeline, openStage, nil)
	e.WsExec(t, `UPDATE deal SET organization_id = $1 WHERE id = $2`, org, deal)

	buyer := e.SeedPerson(t, "Ute Sommer", nil)
	employ(t, e, buyer, org, "Procurement")
	e.WsExec(t, `INSERT INTO relationship (kind, person_id, deal_id, role, source, captured_by)
		VALUES ('deal_stakeholder', $1, $2, 'economic_buyer', 'manual', 'human:x')`, buyer, deal)
	// One colleague who has actually exchanged mail with them.
	seedTouch(t, e, owner, "email", &e.Rep1, buyer)

	got, err := svc.Coverage(ctx, ids.OrganizationID{UUID: org})
	if err != nil {
		t.Fatalf("reading coverage: %v", err)
	}
	if got.Committee == nil || len(got.Committee.Seats) != 1 {
		t.Fatalf("expected one seat, got %+v", got.Committee)
	}
	routes := got.Committee.Seats[0].Routes
	if routes == nil {
		t.Fatal("the seat carries no routes for a caller who may read activity")
	}
	if routes.Untried {
		t.Fatal("a seat somebody has exchanged mail with reads as untried")
	}
	if len(routes.Top) == 0 {
		t.Fatal("no colleague named on a seat with a recorded exchange")
	}
}
