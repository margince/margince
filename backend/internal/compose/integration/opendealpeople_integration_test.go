// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Which lapsed contacts still have money resting on them, and who is allowed to
// learn that.
//
// The decay lane ranks a silence higher when a deal sits behind it, so this read
// decides what a rep is shown first. It carries TWO admissions that answer
// different questions — the edge grant asks whether this caller may read
// stakeholder pairs at all, the deal row scope asks which deals count — and
// neither implies the other. A unit test cannot tell them apart: both are SQL,
// and both fail open in the same direction, by naming a contact whose deal the
// caller was never entitled to know about.
//
// Seeded through the module stores the HTTP writers use, so the seat and the
// deal are rows production writes.

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// fundedReaderPerms is everything the read asks for except the edge, which
// withEdge adds — so two callers can differ in exactly one grant.
func fundedReaderPerms(withEdge bool, scope principal.RowScope) principal.Permissions {
	perms := fundedReaderPermsWithout("", scope)
	if withEdge {
		perms.Objects["relationship"] = principal.ObjectGrant{Read: true}
	}
	return perms
}

// fundedReaderPermsWithout builds the same reader MINUS one named object grant,
// so a test can remove exactly one admission and attribute what changes to it.
//
// Passing "" removes nothing and yields the reader without the edge, which is
// what fundedReaderPerms then adds back.
func fundedReaderPermsWithout(dropped string, scope principal.RowScope) principal.Permissions {
	objects := map[string]principal.ObjectGrant{
		"deal":         {Read: true},
		"person":       {Read: true},
		"organization": {Read: true},
	}
	delete(objects, dropped)
	return principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects:  objects,
		RowScope: scope,
	}
}

func openDealPeople(
	t *testing.T, e *Env, perms principal.Permissions, candidates []ids.PersonID,
) (map[ids.UUID]bool, error) {
	t.Helper()
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)
	var out map[ids.UUID]bool
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		funded, readErr := deals.OpenDealPeople(ctx, tx, candidates)
		out = funded
		return readErr
	})
	return out, err
}

// fundedOrFail is the ordinary path, where a refusal is a test failure rather
// than a case under test.
func fundedOrFail(
	t *testing.T, e *Env, perms principal.Permissions, candidates []ids.PersonID,
) map[ids.UUID]bool {
	t.Helper()
	funded, err := openDealPeople(t, e, perms, candidates)
	if err != nil {
		t.Fatalf("reading which contacts carry an open deal: %v", err)
	}
	return funded
}

func TestOnlyContactsOnAnOpenDealTheReaderMaySeeAreReportedAsFunded(t *testing.T) {
	e := Setup(t)
	org := e.SeedOrg(t, "Kessler Systems", nil)
	orgID := ids.From[ids.OrganizationKind](org)

	seated := ids.From[ids.PersonKind](e.SeedPerson(t, "Seated Contact", nil))
	// A contact on NO deal, so the answer has something to be wrong about. A
	// read that reported every candidate would pass a fixture holding only the
	// funded one.
	unseated := ids.From[ids.PersonKind](e.SeedPerson(t, "Unseated Contact", nil))

	pipeline, open := pipelineFixtureFor(e.Admin(), t, e.Deals)
	// OWNED by somebody other than the reader. An ownerless deal is shared for
	// reads by design — a row nobody owns is the workspace's to see — so a
	// fixture that left the owner unset would put every caller inside the row
	// scope and the narrowing case below would pass without the clause.
	owner := ids.From[ids.UserKind](e.Rep2)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Retrofit", PipelineID: pipeline, StageID: open,
		OrganizationID: &orgID, OwnerID: &owner, Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the deal: %v", err)
	}
	dealID := ids.From[ids.DealKind](ids.UUID(deal.Id))
	champion := "champion"
	if _, err := e.People.CreateRelationship(e.Admin(), people.CreateRelationshipInput{
		Kind: "deal_stakeholder", PersonID: &seated, DealID: &dealID,
		Role: &champion, Source: "manual",
	}); err != nil {
		t.Fatalf("seating the champion: %v", err)
	}

	candidates := []ids.PersonID{seated, unseated}

	granted := fundedOrFail(t, e, fundedReaderPerms(true, principal.RowScopeAll), candidates)
	if !granted[seated.UUID] {
		t.Fatalf("the seated contact is not reported as carrying an open deal — the fixture "+
			"then proves nothing about the refusals below: %v", granted)
	}
	if granted[unseated.UUID] {
		t.Errorf("a contact on no deal at all is reported as carrying one")
	}

	// The edge grant, removed alone. A seat is a PAIR, and knowing a deal does
	// not license learning who sits on it — so a caller who may read both the
	// person and the deal still learns nothing about the seat joining them.
	//
	// It REFUSES rather than answering an empty set, which is what lets the
	// decay lane above it choose: that lane narrows, because a deal behind a
	// silence is one fact on its answer rather than the answer. A read that
	// returned "nobody is funded" would have made that choice here, silently,
	// for every future caller.
	withheld, err := openDealPeople(t, e, fundedReaderPerms(false, principal.RowScopeAll), candidates)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a caller with no edge grant got %v, want the edge refusal", err)
	}
	if withheld[seated.UUID] {
		t.Errorf("a caller with no edge grant is told which contact carries the deal")
	}

	// The DEAL row scope, narrowed alone while the edge grant stays — and the
	// answer does NOT change, because `deal` is an identity table: customer
	// identity is workspace-readable, so the own/team arm is TRUE for every
	// seated actor and only capture privacy or a grant narrows it
	// (platform/auth/tableclass.go).
	//
	// Asserted rather than left out. The read composes the deal's own scope
	// clause, so what that clause does here is part of what this read means,
	// and writing the expectation down is what stops the next reader assuming
	// the narrowing they would get on an owner-scoped table. The deal above is
	// owned by somebody other than this reader, so the case is real: if deal
	// ever leaves the identity set, this line fails and says so.
	bounded := fundedOrFail(t, e, fundedReaderPerms(true, principal.RowScopeOwn), candidates)
	if !bounded[seated.UUID] {
		t.Errorf("a reader bounded to their own rows lost a deal that customer identity is " +
			"workspace-readable for — the row-scope clause narrowed an identity table")
	}

	// The DEAL OBJECT grant, removed alone while the edge grant stays. The
	// third admission, and the one this read originally omitted: a caller who
	// may read stakeholder pairs but not deals was told which contacts carry an
	// open one, so the deal's existence leaked through a person they were
	// entitled to read.
	//
	// Row scope does not cover this and cannot: `deal` is an identity table, so
	// the own/team arm is TRUE for every seated actor. That is exactly why the
	// case above passes and this one has to be asserted separately — a suite
	// that granted deal.read to every principal would have called the leak
	// green.
	noDeals := fundedReaderPermsWithout("deal", principal.RowScopeAll)
	noDeals.Objects["relationship"] = principal.ObjectGrant{Read: true}
	blind, err := openDealPeople(t, e, noDeals, candidates)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a caller with no deal grant got %v, want the deal refusal", err)
	}
	if blind[seated.UUID] {
		t.Errorf("a caller who may not read deals is told one rests on this contact")
	}
}

// The deal's own status. A contact whose only deal has closed carries no money
// any more, and ranking their silence as if they did would send a rep chasing
// business that is already decided.
func TestAContactWhoseOnlyDealHasClosedCarriesNoOpenDeal(t *testing.T) {
	e := Setup(t)
	org := e.SeedOrg(t, "Kessler Systems", nil)
	orgID := ids.From[ids.OrganizationKind](org)
	seated := ids.From[ids.PersonKind](e.SeedPerson(t, "Seated Contact", nil))

	pipeline, open := pipelineFixtureFor(e.Admin(), t, e.Deals)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Retrofit", PipelineID: pipeline, StageID: open,
		OrganizationID: &orgID, Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the deal: %v", err)
	}
	dealID := ids.From[ids.DealKind](ids.UUID(deal.Id))
	champion := "champion"
	if _, err := e.People.CreateRelationship(e.Admin(), people.CreateRelationshipInput{
		Kind: "deal_stakeholder", PersonID: &seated, DealID: &dealID,
		Role: &champion, Source: "manual",
	}); err != nil {
		t.Fatalf("seating the champion: %v", err)
	}

	perms := fundedReaderPerms(true, principal.RowScopeAll)
	// Open first, so the closure below is what changes the answer rather than
	// the fixture never having produced one.
	if !fundedOrFail(t, e, perms, []ids.PersonID{seated})[seated.UUID] {
		t.Fatal("the seat carries no open deal before the deal was closed")
	}

	// Closed through the store's OWN advance path, not by writing the status
	// column: the read filters on a value the transition derives, and a
	// hand-set column would prove this test agrees with itself rather than
	// that a closed deal really stops counting.
	closeDealForTest(t, e, pipeline, dealID)

	if fundedOrFail(t, e, perms, []ids.PersonID{seated})[seated.UUID] {
		t.Errorf("a contact whose only deal has closed is still reported as carrying one")
	}
}

// closeDealForTest advances the deal onto a terminal stage, which is how the
// product closes one. `lost` rather than `won` because a win must account for
// the agreement behind it, and this fixture is about the status alone.
func closeDealForTest(t *testing.T, e *Env, pipeline ids.PipelineID, dealID ids.DealID) {
	t.Helper()
	ctx := e.Admin()
	stages, err := e.Deals.ListStages(ctx, &pipeline, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("listing the pipeline's stages: %v", err)
	}
	var lost ids.StageID
	for _, st := range stages {
		if st.Semantic == "lost" {
			lost = ids.From[ids.StageKind](ids.UUID(st.Id))
			break
		}
	}
	if lost.IsZero() {
		t.Fatal("the seeded pipeline has no lost stage to close onto")
	}
	reason := "the fixture closes it"
	if _, err := e.Deals.AdvanceDeal(ctx, dealID, deals.AdvanceDealInput{
		ToStageID: lost, LostReason: &reason,
	}); err != nil {
		t.Fatalf("closing the deal: %v", err)
	}
}
