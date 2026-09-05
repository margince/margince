// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A hidden contact is not an absent one.
//
// The account grain asks how broadly we know a company. Its stakeholders can be
// capture-private people — minted by a connector out of somebody's mailbox and
// never promoted — and those are invisible to every other seat, including an
// admin's. A count that simply omitted them would report an account with four
// contacts as single-threaded, which is a fact about the reader's permissions
// printed as a fact about the customer.
//
// Only a database can prove this half: the SAME account, read by two callers
// who differ in what they may open, must never come back as a confident wrong
// verdict for either.
//
// Seeded through the module stores that serve the HTTP writers, so the seats
// are the rows production writes.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/network"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// accountCoverageFor runs the reader as one caller.
func accountCoverageFor(
	t *testing.T, e *Env, perms principal.Permissions, orgID ids.UUID,
) network.AccountCoverage {
	t.Helper()
	ctx := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.WS),
		principal.Principal{
			Type: principal.PrincipalHuman, ID: "human:reader",
			UserID: ids.NewV7(), Permissions: perms,
		})
	var out network.AccountCoverage
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		out, err = network.AccountCoverageFor(ctx, tx, orgID)
		return err
	}); err != nil {
		t.Fatalf("reading account coverage: %v", err)
	}
	return out
}

// seatHidden puts a capture-private person on a deal with owner SQL.
//
// Not through the store, and the reason is the boundary under test: capture
// privacy does not yield to row scope, so the ADMIN seeding this fixture cannot
// see the contact either and CreateRelationship answers not-found. The seat is
// nonetheless a row production writes — a connector seats the people it
// captured — so the fixture writes the row the connector would.
func seatHidden(t *testing.T, owner *pgx.Conn, person, deal ids.UUID, role string) {
	t.Helper()
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO relationship (kind, person_id, deal_id, role, source, captured_by)
		VALUES ('deal_stakeholder', $1, $2, $3, 'manual', 'connector:gmail')`,
		person, deal, role); err != nil {
		t.Fatalf("seating a capture-private contact: %v", err)
	}
}

// seatOn puts one person on one deal as a champion.
//
// The role is fixed because no case here turns on which one it is: the reader
// counts distinct PEOPLE, and a fixture varying the role would suggest the
// count does something with it.
func seatOn(t *testing.T, e *Env, person ids.PersonID, deal ids.DealID) {
	t.Helper()
	role := "champion"
	if _, err := e.People.CreateRelationship(e.Admin(), people.CreateRelationshipInput{
		Kind: "deal_stakeholder", PersonID: &person, DealID: &deal,
		Role: &role, Source: "manual",
	}); err != nil {
		t.Fatalf("seating a stakeholder: %v", err)
	}
}

// An account whose contacts a reader cannot open is UNKNOWN, never
// single-threaded.
//
// This is the defect the whole reader exists against. Two of the three
// stakeholders are capture-private to somebody else; a count that dropped them
// would see one contact and report a finding about the customer that is really
// a fact about this reader.
func TestAnAccountWithHiddenContactsIsUnknownRatherThanSingleThreaded(t *testing.T) {
	e := Setup(t)
	org := e.SeedOrg(t, "Halden Werke", nil)
	orgID := ids.From[ids.OrganizationKind](org)
	pipeline, open := pipelineFixtureFor(e.Admin(), t, e.Deals)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Halden renewal", PipelineID: pipeline, StageID: open,
		OrganizationID: &orgID, Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the deal: %v", err)
	}
	dealID := ids.From[ids.DealKind](ids.UUID(deal.Id))

	// One contact everybody can see.
	visible := e.SeedPerson(t, "Open Contact", nil)
	seatOn(t, e, ids.From[ids.PersonKind](visible), dealID)

	// Two a connector captured into another seat's mailbox and nobody promoted.
	// Written the way capture writes them: owner-private, owned by a colleague.
	// Rep3 is a real seeded seat, and the person FK needs one. Capture privacy
	// does not yield to row scope, so a contact owned by any other seat is
	// invisible to this reader whatever their scope.
	colleague := e.Rep3
	owner := OwnerConn(t)
	for _, name := range []string{"Private Buyer", "Private Sponsor"} {
		var hidden ids.UUID
		if err := owner.QueryRow(context.Background(), `
			INSERT INTO person (full_name, owner_id, visibility, source, captured_by)
			VALUES ($1, $2, 'owner', 'manual', 'connector:gmail') RETURNING id`,
			name, colleague).Scan(&hidden); err != nil {
			t.Fatalf("seeding a capture-private contact: %v", err)
		}
		seatHidden(t, owner, hidden, dealID.UUID, "buyer")
	}

	perms := coverageReaderPerms(true)
	got := accountCoverageFor(t, e, perms, orgID.UUID)

	if len(got.VisibleStakeholders) != 1 {
		t.Fatalf("the reader sees %d stakeholders, want 1 — the fixture needs exactly one visible "+
			"contact for the verdict below to be the interesting one", len(got.VisibleStakeholders))
	}
	if !got.CoverageIncomplete {
		t.Error("coverage reads as complete; without knowing something was withheld the verdict " +
			"cannot tell an account it cannot see from one that is genuinely thin")
	}
	if got.Threading != network.ThreadingUnknown {
		t.Errorf("threading = %q, want %q — one visible contact beside two hidden ones is not a "+
			"finding about the customer, it is a fact about this reader",
			got.Threading, network.ThreadingUnknown)
	}
}

// An account genuinely resting on one relationship IS reported, and that is the
// other half: a reader who can see everything gets the finding.
//
// Without this arm the test above passes against a reader that answers
// "unknown" to every account, which would be a different defect wearing the
// same green.
func TestAnAccountWithOneVisibleContactAndNothingHiddenIsSingleThreaded(t *testing.T) {
	e := Setup(t)
	org := e.SeedOrg(t, "Solo Contact GmbH", nil)
	orgID := ids.From[ids.OrganizationKind](org)
	pipeline, open := pipelineFixtureFor(e.Admin(), t, e.Deals)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Solo renewal", PipelineID: pipeline, StageID: open,
		OrganizationID: &orgID, Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the deal: %v", err)
	}
	only := e.SeedPerson(t, "The Only Contact", nil)
	seatOn(t, e, ids.From[ids.PersonKind](only), ids.From[ids.DealKind](ids.UUID(deal.Id)))

	got := accountCoverageFor(t, e, coverageReaderPerms(true), orgID.UUID)
	if got.CoverageIncomplete {
		t.Fatal("coverage reads as incomplete; this fixture hides nothing, so the finding below " +
			"would be reported for the wrong reason")
	}
	if got.Threading != network.ThreadingSingle {
		t.Errorf("threading = %q, want %q — nothing is hidden and there is one contact, which is "+
			"the finding a manager needs", got.Threading, network.ThreadingSingle)
	}
}

// An account with no stakeholder at all is its own state, not single-threaded
// and not unknown. A manager acts differently on "nobody recorded" than on
// "one person carries it".
func TestAnAccountWithNoRecordedContactsSaysSo(t *testing.T) {
	e := Setup(t)
	org := e.SeedOrg(t, "Untouched AG", nil)
	orgID := ids.From[ids.OrganizationKind](org)
	pipeline, open := pipelineFixtureFor(e.Admin(), t, e.Deals)
	if _, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Untouched deal", PipelineID: pipeline, StageID: open,
		OrganizationID: &orgID, Source: "manual",
	}); err != nil {
		t.Fatalf("creating the deal: %v", err)
	}

	got := accountCoverageFor(t, e, coverageReaderPerms(true), orgID.UUID)
	if got.Threading != network.ThreadingNoContacts {
		t.Errorf("threading = %q, want %q — an account nobody has recorded a contact on is not "+
			"an account resting on one relationship", got.Threading, network.ThreadingNoContacts)
	}
	if len(got.VisibleStakeholders) != 0 || got.CoverageIncomplete {
		t.Errorf("visible = %d and incomplete = %v, want 0 and false",
			len(got.VisibleStakeholders), got.CoverageIncomplete)
	}
}

// Seeing ENOUGH is safe to report whatever is hidden: hidden contacts can only
// add to a count that already clears the floor.
func TestEnoughVisibleContactsIsMultiThreadedEvenWithSomethingHidden(t *testing.T) {
	e := Setup(t)
	org := e.SeedOrg(t, "Broad Coverage SE", nil)
	orgID := ids.From[ids.OrganizationKind](org)
	pipeline, open := pipelineFixtureFor(e.Admin(), t, e.Deals)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Broad renewal", PipelineID: pipeline, StageID: open,
		OrganizationID: &orgID, Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the deal: %v", err)
	}
	dealID := ids.From[ids.DealKind](ids.UUID(deal.Id))
	for _, name := range []string{"First Contact", "Second Contact"} {
		p := e.SeedPerson(t, name, nil)
		seatOn(t, e, ids.From[ids.PersonKind](p), dealID)
	}
	var hidden ids.UUID
	if err := OwnerConn(t).QueryRow(context.Background(), `
		INSERT INTO person (full_name, owner_id, visibility, source, captured_by)
		VALUES ('Private Extra', $1, 'owner', 'manual', 'connector:gmail') RETURNING id`,
		e.Rep3).Scan(&hidden); err != nil {
		t.Fatalf("seeding a capture-private contact: %v", err)
	}
	seatHidden(t, OwnerConn(t), hidden, dealID.UUID, "buyer")

	got := accountCoverageFor(t, e, coverageReaderPerms(true), orgID.UUID)
	if got.Threading != network.ThreadingMultiple {
		t.Errorf("threading = %q, want %q — two visible contacts already answer the question, and "+
			"a hidden third can only widen the account", got.Threading, network.ThreadingMultiple)
	}
	if !got.CoverageIncomplete {
		t.Error("the reader is not told their view is incomplete, though a contact is hidden")
	}
}

// One person seated on three deals of one account is ONE relationship. Counting
// the edges instead would clear the threading floor on a single contact and
// report an account as broadly covered when one person carries all of it.
func TestOnePersonOnThreeDealsIsOneRelationship(t *testing.T) {
	e := Setup(t)
	org := e.SeedOrg(t, "Repeated Seat Ltd", nil)
	orgID := ids.From[ids.OrganizationKind](org)
	pipeline, open := pipelineFixtureFor(e.Admin(), t, e.Deals)
	only := ids.From[ids.PersonKind](e.SeedPerson(t, "Everywhere Contact", nil))
	for _, name := range []string{"Deal one", "Deal two", "Deal three"} {
		deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
			Name: name, PipelineID: pipeline, StageID: open,
			OrganizationID: &orgID, Source: "manual",
		})
		if err != nil {
			t.Fatalf("creating %s: %v", name, err)
		}
		seatOn(t, e, only, ids.From[ids.DealKind](ids.UUID(deal.Id)))
	}

	got := accountCoverageFor(t, e, coverageReaderPerms(true), orgID.UUID)
	if len(got.VisibleStakeholders) != 1 {
		t.Fatalf("counted %d stakeholders, want 1 — one person on three deals is one relationship",
			len(got.VisibleStakeholders))
	}
	if got.Threading != network.ThreadingSingle {
		t.Errorf("threading = %q, want %q — three seats held by one person is exactly the account "+
			"a manager needs warning about", got.Threading, network.ThreadingSingle)
	}
}

// A stakeholder seated only on a PROJECT counts toward the account.
//
// An account's coverage is every contact we know there, and delivery work is
// where half of them are. Without the project arm this reader would report a
// company with three project stakeholders as having nobody — and no other test
// here seeds a project, so the arm could be deleted green.
func TestAProjectStakeholderCountsTowardTheAccount(t *testing.T) {
	e := Setup(t)
	org := e.SeedOrg(t, "Delivery Only KG", nil)
	orgID := ids.From[ids.OrganizationKind](org)
	project, err := e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{
		Name: "Rollout", OrganizationID: orgID, Source: "manual",
	})
	if err != nil {
		t.Fatalf("opening the project: %v", err)
	}
	projectID := ids.From[ids.ProjectKind](ids.UUID(project.Id))
	for _, name := range []string{"Delivery Lead", "Ops Sponsor"} {
		person := ids.From[ids.PersonKind](e.SeedPerson(t, name, nil))
		role := "sponsor"
		if _, err := e.People.CreateRelationship(e.Admin(), people.CreateRelationshipInput{
			Kind: "project_stakeholder", PersonID: &person, ProjectID: &projectID,
			Role: &role, Source: "manual",
		}); err != nil {
			t.Fatalf("seating a project stakeholder: %v", err)
		}
	}

	got := accountCoverageFor(t, e, coverageReaderPerms(true), orgID.UUID)
	if len(got.VisibleStakeholders) != 2 {
		t.Fatalf("counted %d stakeholders, want 2 — a contact known through delivery is still a "+
			"contact at this account", len(got.VisibleStakeholders))
	}
	if got.Threading != network.ThreadingMultiple {
		t.Errorf("threading = %q, want %q", got.Threading, network.ThreadingMultiple)
	}
}

// An account the caller may not open is refused, not answered.
//
// Without the organization admission this function answers about any id a
// caller can name — and the incompleteness flag then reports something about a
// company they were never admitted to.
func TestAnAccountTheReaderCannotOpenIsRefused(t *testing.T) {
	e := Setup(t)
	ctx := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.WS),
		principal.Principal{
			Type: principal.PrincipalHuman, ID: "human:reader", UserID: ids.NewV7(),
			Permissions: coverageReaderPerms(true),
		})
	// An id naming no organization this workspace holds. A caller who guessed
	// one must learn nothing from the answer.
	absent := ids.NewV7()
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := network.AccountCoverageFor(ctx, tx, absent)
		return err
	})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("reading coverage for an unknown account answered %v, want not-found — an "+
			"account nobody may open owes no answer about its contacts", err)
	}
}

// A caller without the edge grant is told what was withheld, rather than served
// an empty account that reads as an uncovered one.
func TestAccountCoverageIsNamedRatherThanEmptyWithoutTheEdgeGrant(t *testing.T) {
	e := Setup(t)
	org := e.SeedOrg(t, "Edge Gated NV", nil)
	orgID := ids.From[ids.OrganizationKind](org)
	pipeline, open := pipelineFixtureFor(e.Admin(), t, e.Deals)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Gated renewal", PipelineID: pipeline, StageID: open,
		OrganizationID: &orgID, Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the deal: %v", err)
	}
	p := e.SeedPerson(t, "Real Contact", nil)
	seatOn(t, e, ids.From[ids.PersonKind](p), ids.From[ids.DealKind](ids.UUID(deal.Id)))

	granted := accountCoverageFor(t, e, coverageReaderPerms(true), orgID.UUID)
	if len(granted.SectionsOmitted) != 0 || len(granted.VisibleStakeholders) != 1 {
		t.Fatalf("the granted caller sees %d stakeholders and is told %v was omitted; the fixture "+
			"then proves nothing about the withheld one",
			len(granted.VisibleStakeholders), granted.SectionsOmitted)
	}

	withheld := accountCoverageFor(t, e, coverageReaderPerms(false), orgID.UUID)
	if len(withheld.SectionsOmitted) == 0 {
		t.Error("the withheld caller is told nothing was omitted; an empty account with nothing " +
			"naming it renders as an account nobody has contacts on")
	}
	if withheld.Threading == network.ThreadingSingle || withheld.Threading == network.ThreadingNoContacts {
		t.Errorf("threading = %q for a caller who may read no edges at all — that is a verdict "+
			"about the customer drawn from a permission", withheld.Threading)
	}
}
