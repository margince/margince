// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package org360

// Why the account's work in flight needs a person, against a real database.
//
// The unit lane cannot see any of what these pin: the row-scope predicates on
// people and activities are SQL, the DISTINCT ON ordering that decides WHICH
// fact wins is SQL, and the fold from "the caller has no activity grant" to
// "rows without reasons, and the payload says so" only happens once a real
// gate refuses.

import (
	"fmt"
	"testing"
	"time"

	"github.com/gradionhq/margince/backend/internal/compose/integration"
	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/modules/activities"
	"github.com/gradionhq/margince/backend/internal/modules/deals"
	"github.com/gradionhq/margince/backend/internal/modules/people"
	"github.com/gradionhq/margince/backend/internal/modules/projects"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

// overdueAt is a due date safely behind the read's pinned clock, and
// laterOverdueAt one that is overdue as well but by less — the pair that
// proves the section picks the MOST overdue rather than any overdue one.
var (
	overdueAt      = org360Clock.AddDate(0, 0, -21)
	laterOverdueAt = org360Clock.AddDate(0, 0, -3)
)

func TestOrg360_ADealCarriesItsMostOverdueTaskAsTheReason(t *testing.T) {
	e := integration.Setup(t)
	pipeline, stage, _ := integration.DealFixture(t, e)
	orgID := e.SeedOrg(t, "Overdue Account", nil)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Fleet retrofit", PipelineID: pipeline, StageID: stage,
		OrganizationID: ptrTo(ids.From[ids.OrganizationKind](orgID)), Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the deal: %v", err)
	}
	dealID := ids.UUID(deal.Id)
	logTask(t, e, "Chase the signature", laterOverdueAt, dealID)
	logTask(t, e, "Send the retrofit quote", overdueAt, dealID)

	view := assemble(t, e, orgID)
	attention := dealAttention(t, view, dealID)
	if attention.Kind != crmcontracts.WorkAttentionOverdueTask {
		t.Fatalf("kind = %q, want an overdue task", attention.Kind)
	}
	// Most overdue wins, and it must be stable: without the id tiebreaker two
	// tasks due the same instant swap between two reads of the same page.
	if attention.Title != "Send the retrofit quote" {
		t.Fatalf("reason = %q, want the task overdue by longest", attention.Title)
	}
	if attention.Who == nil || *attention.Who == "" {
		t.Fatal("the assignee is unnamed, though the task names one this caller may read")
	}
}

func TestOrg360_ADealCarriesNoReasonFromAnotherAccountsTask(t *testing.T) {
	e := integration.Setup(t)
	pipeline, stage, _ := integration.DealFixture(t, e)
	ours := e.SeedOrg(t, "Our Account", nil)
	theirs := e.SeedOrg(t, "Their Account", nil)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Shared name", PipelineID: pipeline, StageID: stage,
		OrganizationID: ptrTo(ids.From[ids.OrganizationKind](ours)), Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the deal: %v", err)
	}
	// A task linked to the deal but filed under the OTHER account: the link
	// alone would reach this page, which is what the account walk prevents.
	logTaskUnder(t, e, "Not this account's task", overdueAt,
		activities.ActivityLinkInput{EntityType: "organization", EntityID: theirs})

	view := assemble(t, e, ours)
	if row := findDeal(t, view, ids.UUID(deal.Id)); row.Attention != nil {
		t.Fatalf("the deal carries %q, read from another account's task", row.Attention.Title)
	}
}

func TestOrg360_AProjectCarriesTheOpenCommitmentTheyMade(t *testing.T) {
	e := integration.Setup(t)
	orgID := e.SeedOrg(t, "Committed Account", nil)
	project, err := e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{
		Name: "Depot fit-out", OrganizationID: ids.From[ids.OrganizationKind](orgID), Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the project: %v", err)
	}
	projectID := ids.UUID(project.Id)
	personID := e.SeedPerson(t, "Ida Keller", nil)
	// Through the real writer, so the claim carries the evidence and the
	// fingerprint a hand-inserted row would not have.
	body := "we'll confirm the depot slot once facilities sign off"
	recordClaim(t, e, personID, projectID, body, "open", false)

	view := assemble(t, e, orgID)
	attention := projectAttention(t, view, projectID)
	if attention.Kind != crmcontracts.WorkAttentionCommitmentTheirs {
		t.Fatalf("kind = %q, want a commitment they made", attention.Kind)
	}
	// Verbatim: the card quotes the body, and a paraphrase here would be the
	// model-written sentence the whole card exists to replace.
	if attention.Title != body {
		t.Fatalf("reason = %q, want the claim body verbatim", attention.Title)
	}
	if attention.SourceActivityId == nil {
		t.Fatal("no source activity, so the reader has no receipt to open")
	}
}

func TestOrg360_ADisputedOrSettledCommitmentIsNotStatedAsFact(t *testing.T) {
	e := integration.Setup(t)
	orgID := e.SeedOrg(t, "Disputed Account", nil)
	project, err := e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{
		Name: "Telemetry pilot", OrganizationID: ids.From[ids.OrganizationKind](orgID), Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the project: %v", err)
	}
	projectID := ids.UUID(project.Id)
	personID := e.SeedPerson(t, "Ida Keller", nil)
	// Newest first, and both must lose: a settled claim is no longer owed,
	// and needs_review means the extractor found contradicting evidence — the
	// claim contract calls newest-wins no resolution, so presenting either as
	// "they owe us this" states a contested thing as a fact.
	recordClaim(t, e, personID, projectID, "already delivered", "done", false)
	recordClaim(t, e, personID, projectID, "contradicted by a later mail", "open", true)

	view := assemble(t, e, orgID)
	if row := findProject(t, view, projectID); row.Attention != nil {
		t.Fatalf("the project carries %q, which is settled or disputed", row.Attention.Title)
	}
}

func TestOrg360_AReaderWithoutTheActivityGrantGetsRowsAndIsToldTheReasonsAreMissing(t *testing.T) {
	e := integration.Setup(t)
	pipeline, stage, _ := integration.DealFixture(t, e)
	orgID := e.SeedOrg(t, "Blind Account", nil)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Fleet retrofit", PipelineID: pipeline, StageID: stage,
		OrganizationID: ptrTo(ids.From[ids.OrganizationKind](orgID)), Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the deal: %v", err)
	}
	logTask(t, e, "Send the retrofit quote", overdueAt, ids.UUID(deal.Id))

	// The fixture the suite already keeps for exactly this reader: everything
	// but the activity grant. Reused rather than re-declared, so a grant added
	// to that rep reaches this assertion too.
	view, err := org360Service(e).Assemble(
		e.As(e.Rep1, []ids.UUID{e.Team1}, org360NoActivityPerms),
		ids.From[ids.OrganizationKind](orgID))
	if err != nil {
		t.Fatalf("assembling the 360: %v", err)
	}
	// The deal is present and true. Only the reason is missing, and the
	// payload says so — an unexplained row must not read as a settled one.
	row := findDeal(t, view, ids.UUID(deal.Id))
	if row.Attention != nil {
		t.Fatalf("a reader with no activity grant got %q", row.Attention.Title)
	}
	if view.AttentionWithheld == nil || !*view.AttentionWithheld {
		t.Fatal("the payload does not say the reasons were withheld")
	}
	// Withholding the reasons is not withholding the pipeline: reporting the
	// deals section as omitted would hide it from a reader who may read it.
	for _, omitted := range view.SectionsOmitted {
		if omitted == "deals" {
			t.Fatal("the deals section was named as omitted, though the caller may read it")
		}
	}
}

func assemble(t *testing.T, e *integration.Env, orgID ids.UUID) crmcontracts.Organization360 {
	t.Helper()
	view, err := org360Service(e).Assemble(e.Admin(), ids.From[ids.OrganizationKind](orgID))
	if err != nil {
		t.Fatalf("assembling the 360: %v", err)
	}
	return view
}

// logTask files an overdue task against the deal AND the account, which is how
// a task on a deal actually reaches the company page.
func logTask(t *testing.T, e *integration.Env, subject string, due time.Time, dealID ids.UUID) {
	t.Helper()
	logTaskUnder(t, e, subject, due,
		activities.ActivityLinkInput{EntityType: "deal", EntityID: dealID})
}

func logTaskUnder(
	t *testing.T, e *integration.Env, subject string, due time.Time,
	links ...activities.ActivityLinkInput,
) {
	t.Helper()
	assignee := ids.From[ids.UserKind](e.AdminUser)
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, DueAt: &due,
		AssigneeID: &assignee, Links: links, Source: "manual",
	}); err != nil {
		t.Fatalf("logging the task %q: %v", subject, err)
	}
}

// recordClaim writes one commitment through the real writer, grounded in a
// message filed under the project — the shape the extractor produces.
func recordClaim(
	t *testing.T, e *integration.Env, personID, projectID ids.UUID,
	body, status string, needsReview bool,
) {
	t.Helper()
	subject := "Depot slot"
	message, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: ptrTo("inbound"), Source: "manual",
		Links: []activities.ActivityLinkInput{
			{EntityType: "person", EntityID: personID},
			{EntityType: "project", EntityID: projectID},
		},
	})
	if err != nil {
		t.Fatalf("logging the evidence: %v", err)
	}
	claim, err := people.NewStore(e.DB()).RecordConversationClaim(e.Admin(), people.ClaimInput{
		PersonID: ids.From[ids.PersonKind](personID), Kind: "commitment_theirs",
		Body: body, ActivityID: ids.UUID(message.Id), Quote: body, Source: "manual",
	})
	if err != nil {
		t.Fatalf("recording the claim: %v", err)
	}
	// Status and needs_review are the extractor's own lifecycle columns and
	// have no writer on this path; set directly so the read's filter is
	// exercised against the states it exists to reject.
	if status != "open" || needsReview {
		e.WsExec(t, `UPDATE conversation_claim SET status = $2, needs_review = $3 WHERE id = $1`,
			ids.UUID(claim.Id), status, needsReview)
	}
}

func findDeal(t *testing.T, view crmcontracts.Organization360, dealID ids.UUID) crmcontracts.Organization360Deal {
	t.Helper()
	if view.Deals == nil {
		t.Fatal("no deals section, so no row to read")
	}
	for _, row := range view.Deals.Data {
		if ids.UUID(row.DealId) == dealID {
			return row
		}
	}
	t.Fatal("the deal is not on the page")
	return crmcontracts.Organization360Deal{}
}

func findProject(t *testing.T, view crmcontracts.Organization360, projectID ids.UUID) crmcontracts.Organization360Project {
	t.Helper()
	if view.Projects == nil {
		t.Fatal("no projects section, so no row to read")
	}
	for _, row := range *view.Projects {
		if ids.UUID(row.ProjectId) == projectID {
			return row
		}
	}
	t.Fatal("the project is not on the page")
	return crmcontracts.Organization360Project{}
}

func dealAttention(t *testing.T, view crmcontracts.Organization360, dealID ids.UUID) crmcontracts.Organization360WorkAttention {
	t.Helper()
	row := findDeal(t, view, dealID)
	if row.Attention == nil {
		t.Fatal("the deal carries no reason, though one of its tasks is overdue")
	}
	return *row.Attention
}

func projectAttention(t *testing.T, view crmcontracts.Organization360, projectID ids.UUID) crmcontracts.Organization360WorkAttention {
	t.Helper()
	row := findProject(t, view, projectID)
	if row.Attention == nil {
		t.Fatal("the project carries no reason, though a commitment on it is open")
	}
	return *row.Attention
}

func TestOrg360_ACommitmentNeedsThePersonGrantAndNotOnlyTheRowScope(t *testing.T) {
	e := integration.Setup(t)
	orgID := e.SeedOrg(t, "Person-blind Account", nil)
	project, err := e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{
		Name: "Depot fit-out", OrganizationID: ids.From[ids.OrganizationKind](orgID), Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the project: %v", err)
	}
	projectID := ids.UUID(project.Id)
	personID := e.SeedPerson(t, "Ida Keller", nil)
	body := "we'll confirm the depot slot once facilities sign off"
	recordClaim(t, e, personID, projectID, body, "open", false)

	// The claim names a PERSON and its row carries their name and what they
	// said. Row scope alone admits nobody to people — it narrows a set the
	// object grant has already opened, and for an unbounded actor it is no
	// predicate at all — so a reader without person:read must be refused
	// rather than handed both.
	view, err := org360Service(e).Assemble(
		e.As(e.Rep1, []ids.UUID{e.Team1}, org360NoPersonPerms),
		ids.From[ids.OrganizationKind](orgID))
	if err != nil {
		t.Fatalf("assembling the 360: %v", err)
	}
	row := findProject(t, view, projectID)
	if row.Attention != nil {
		t.Fatalf("a reader without person:read got %q", row.Attention.Title)
	}
	if view.AttentionWithheld == nil || !*view.AttentionWithheld {
		t.Fatal("the payload does not say the reasons were withheld")
	}
}

func TestOrg360_ACommitmentByAnInvisiblePersonIsReportedRatherThanDropped(t *testing.T) {
	e := integration.Setup(t)
	orgID := e.SeedOrg(t, "Scoped Account", nil)
	project, err := e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{
		Name: "Depot fit-out", OrganizationID: ids.From[ids.OrganizationKind](orgID), Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the project: %v", err)
	}
	projectID := ids.UUID(project.Id)
	// Captured PRIVATELY by somebody else. Customer identity is otherwise
	// workspace-readable, so capture privacy is the thing that actually hides
	// a person from a colleague — an owner change alone does not.
	other := e.Rep3
	personID := e.SeedPerson(t, "Ida Keller", &other)
	recordClaim(t, e, personID, projectID,
		"we'll confirm the depot slot once facilities sign off", "open", false)
	e.WsExec(t, `UPDATE person SET visibility = 'owner' WHERE id = $1`, personID)

	view, err := org360Service(e).Assemble(
		e.As(e.Rep1, []ids.UUID{e.Team1}, org360OwnScopePerms),
		ids.From[ids.OrganizationKind](orgID))
	if err != nil {
		t.Fatalf("assembling the 360: %v", err)
	}
	// The commitment is correctly absent — but silence alone would read as a
	// project with nothing outstanding, which is what the flag exists to
	// prevent. Present and unexplained beats absent and misread.
	row := findProject(t, view, projectID)
	if row.Attention != nil {
		t.Fatalf("an out-of-scope person's claim reached the page as %q", row.Attention.Title)
	}
	if view.AttentionWithheld == nil || !*view.AttentionWithheld {
		t.Fatal("a claim was dropped for row scope and the payload does not say so")
	}
}

// org360NoPersonPerms may read the account, its projects and its activities,
// and may not read people at all.
var org360NoPersonPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"organization":          {Read: true},
		"project":               {Read: true},
		"activity":              {Read: true},
		"relationship":          {Read: true},
		"installation_settings": {Read: true},
	},
	RowScope: principal.RowScopeAll,
}

// org360OwnScopePerms holds every grant this card reads and is bounded to its
// own rows — the reader for whom a colleague's privately captured contact is
// invisible rather than forbidden.
var org360OwnScopePerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"organization":          {Read: true},
		"person":                {Read: true},
		"project":               {Read: true},
		"deal":                  {Read: true},
		"activity":              {Read: true},
		"relationship":          {Read: true},
		"installation_settings": {Read: true},
	},
	RowScope: principal.RowScopeOwn,
}

func TestOrg360_ClosedProjectsOverflowingTheCapDoNotClaimMoreWorkInFlight(t *testing.T) {
	e := integration.Setup(t)
	orgID := e.SeedOrg(t, "Portfolio Account", nil)
	// One project in flight and enough closed ones to overflow the cap. The
	// card's order puts the closed ones last, so the cap cuts only history —
	// and a page reporting "1+ in flight" off a bare overflow flag would say
	// this account has live work it does not have.
	live, err := e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{
		Name: "Depot fit-out", OrganizationID: ids.From[ids.OrganizationKind](orgID), Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the live project: %v", err)
	}
	for i := range 25 {
		closed, err := e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{
			Name:           fmt.Sprintf("Finished %d", i),
			OrganizationID: ids.From[ids.OrganizationKind](orgID), Source: "manual",
		})
		if err != nil {
			t.Fatalf("creating a closed project: %v", err)
		}
		// Through the real writer: a closed project carries a reason the
		// schema insists on, and a hand-set phase never produces one.
		if _, err := e.Projects.AdvanceProjectPhase(e.Admin(),
			ids.From[ids.ProjectKind](ids.UUID(closed.Id)),
			projects.AdvanceProjectPhaseInput{ToPhase: "closed", Reason: ptrTo("delivered")},
		); err != nil {
			t.Fatalf("closing a project: %v", err)
		}
	}

	view := assemble(t, e, orgID)
	if view.ProjectsPage == nil {
		t.Fatal("no projects page, so the card cannot tell a full list from a cut one")
	}
	if view.ProjectsPage.HasMore {
		t.Error("the page reports more work in flight, though every project the cap cut is closed")
	}
	if findProject(t, view, ids.UUID(live.Id)).Phase == crmcontracts.Organization360ProjectPhaseClosed {
		t.Fatal("the live project is missing from the page, so the assertion above is vacuous")
	}
}
