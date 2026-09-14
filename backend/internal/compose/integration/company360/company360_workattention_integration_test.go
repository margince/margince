// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package company360

// Why the account's work in flight needs a contact, against a real database.
//
// The unit lane cannot see any of what these pin: the row-scope predicates on
// contacts and activities are SQL, the DISTINCT ON ordering that decides WHICH
// fact wins is SQL, and the fold from "the caller has no activity grant" to
// "rows without reasons, and the payload says so" only happens once a real
// gate refuses.

import (
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// overdueAt is a due date safely behind the read's pinned clock, and
// laterOverdueAt one that is overdue as well but by less — the pair that
// proves the section picks the MOST overdue rather than any overdue one.
var (
	overdueAt      = company360Clock.AddDate(0, 0, -21)
	laterOverdueAt = company360Clock.AddDate(0, 0, -3)
)

func TestCompany360_ADealCarriesItsMostOverdueTaskAsTheReason(t *testing.T) {
	e := integration.Setup(t)
	pipeline, stage, _ := integration.DealFixture(t, e)
	companyID := e.SeedCompany(t, "Overdue Account", nil)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Fleet retrofit", PipelineID: pipeline, StageID: stage,
		CompanyID: ptrTo(ids.From[ids.CompanyKind](companyID)), Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the deal: %v", err)
	}
	dealID := ids.UUID(deal.Id)
	logTask(t, e, "Chase the signature", laterOverdueAt, dealID)
	logTask(t, e, "Send the retrofit quote", overdueAt, dealID)

	view := assemble(t, e, companyID)
	attention := dealAttention(t, view, dealID)
	if attention.Kind != crmcontracts.Company360WorkAttentionKindWorkAttentionOverdueTask {
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

func TestCompany360_ADealCarriesNoReasonFromAnotherAccountsTask(t *testing.T) {
	e := integration.Setup(t)
	pipeline, stage, _ := integration.DealFixture(t, e)
	ours := e.SeedCompany(t, "Our Account", nil)
	theirs := e.SeedCompany(t, "Their Account", nil)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Shared name", PipelineID: pipeline, StageID: stage,
		CompanyID: ptrTo(ids.From[ids.CompanyKind](ours)), Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the deal: %v", err)
	}
	// A task linked to the deal but filed under the OTHER account: the link
	// alone would reach this page, which is what the account walk prevents.
	logTaskUnder(t, e, "Not this account's task", overdueAt,
		activities.ActivityLinkInput{EntityType: "company", EntityID: theirs})

	view := assemble(t, e, ours)
	if row := findDeal(t, view, ids.UUID(deal.Id)); row.Attention != nil {
		t.Fatalf("the deal carries %q, read from another account's task", row.Attention.Title)
	}
}

func TestCompany360_AProjectCarriesTheOpenCommitmentTheyMade(t *testing.T) {
	e := integration.Setup(t)
	companyID := e.SeedCompany(t, "Committed Account", nil)
	project, err := e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{
		Name: "Depot fit-out", CompanyID: ids.From[ids.CompanyKind](companyID), Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the project: %v", err)
	}
	projectID := ids.UUID(project.Id)
	contactID := e.SeedContact(t, "Ida Keller", nil)
	// Through the real writer, so the claim carries the evidence and the
	// fingerprint a hand-inserted row would not have.
	body := "we'll confirm the depot slot once facilities sign off"
	recordClaim(t, e, contactID, projectID, body, "open", false)

	view := assemble(t, e, companyID)
	attention := projectAttention(t, view, projectID)
	if attention.Kind != crmcontracts.Company360WorkAttentionKindWorkAttentionCommitmentTheirs {
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

func TestCompany360_ADisputedOrSettledCommitmentIsNotStatedAsFact(t *testing.T) {
	e := integration.Setup(t)
	companyID := e.SeedCompany(t, "Disputed Account", nil)
	project, err := e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{
		Name: "Telemetry pilot", CompanyID: ids.From[ids.CompanyKind](companyID), Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the project: %v", err)
	}
	projectID := ids.UUID(project.Id)
	contactID := e.SeedContact(t, "Ida Keller", nil)
	// Newest first, and both must lose: a settled claim is no longer owed,
	// and needs_review means the extractor found contradicting evidence — the
	// claim contract calls newest-wins no resolution, so presenting either as
	// "they owe us this" states a contested thing as a fact.
	recordClaim(t, e, contactID, projectID, "already delivered", "done", false)
	recordClaim(t, e, contactID, projectID, "contradicted by a later mail", "open", true)

	view := assemble(t, e, companyID)
	if row := findProject(t, view, projectID); row.Attention != nil {
		t.Fatalf("the project carries %q, which is settled or disputed", row.Attention.Title)
	}
}

func TestCompany360_AReaderWithoutTheActivityGrantGetsRowsAndIsToldTheReasonsAreMissing(t *testing.T) {
	e := integration.Setup(t)
	pipeline, stage, _ := integration.DealFixture(t, e)
	companyID := e.SeedCompany(t, "Blind Account", nil)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Fleet retrofit", PipelineID: pipeline, StageID: stage,
		CompanyID: ptrTo(ids.From[ids.CompanyKind](companyID)), Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the deal: %v", err)
	}
	logTask(t, e, "Send the retrofit quote", overdueAt, ids.UUID(deal.Id))

	// The fixture the suite already keeps for exactly this reader: everything
	// but the activity grant. Reused rather than re-declared, so a grant added
	// to that rep reaches this assertion too.
	view, err := company360Service(e).Assemble(
		e.As(e.Rep1, []ids.UUID{e.Team1}, company360NoActivityPerms),
		ids.From[ids.CompanyKind](companyID))
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

func assemble(t *testing.T, e *integration.Env, companyID ids.UUID) crmcontracts.Company360 {
	t.Helper()
	view, err := company360Service(e).Assemble(e.Admin(), ids.From[ids.CompanyKind](companyID))
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
	t *testing.T, e *integration.Env, contactID, projectID ids.UUID,
	body, status string, needsReview bool,
) {
	t.Helper()
	subject := "Depot slot"
	message, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: ptrTo("inbound"), Source: "manual",
		Links: []activities.ActivityLinkInput{
			{EntityType: "contact", EntityID: contactID},
			{EntityType: "project", EntityID: projectID},
		},
	})
	if err != nil {
		t.Fatalf("logging the evidence: %v", err)
	}
	claim, err := contacts.NewStore(e.DB()).RecordConversationClaim(e.Admin(), contacts.ClaimInput{
		ContactID: ids.From[ids.ContactKind](contactID), Kind: "commitment_theirs",
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

func findDeal(t *testing.T, view crmcontracts.Company360, dealID ids.UUID) crmcontracts.Company360Deal {
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
	return crmcontracts.Company360Deal{}
}

func findProject(t *testing.T, view crmcontracts.Company360, projectID ids.UUID) crmcontracts.Company360Project {
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
	return crmcontracts.Company360Project{}
}

func dealAttention(t *testing.T, view crmcontracts.Company360, dealID ids.UUID) crmcontracts.Company360WorkAttention {
	t.Helper()
	row := findDeal(t, view, dealID)
	if row.Attention == nil {
		t.Fatal("the deal carries no reason, though one of its tasks is overdue")
	}
	return *row.Attention
}

func projectAttention(t *testing.T, view crmcontracts.Company360, projectID ids.UUID) crmcontracts.Company360WorkAttention {
	t.Helper()
	row := findProject(t, view, projectID)
	if row.Attention == nil {
		t.Fatal("the project carries no reason, though a commitment on it is open")
	}
	return *row.Attention
}

func TestCompany360_ACommitmentNeedsTheContactGrantAndNotOnlyTheRowScope(t *testing.T) {
	e := integration.Setup(t)
	companyID := e.SeedCompany(t, "Contact-blind Account", nil)
	project, err := e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{
		Name: "Depot fit-out", CompanyID: ids.From[ids.CompanyKind](companyID), Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the project: %v", err)
	}
	projectID := ids.UUID(project.Id)
	contactID := e.SeedContact(t, "Ida Keller", nil)
	body := "we'll confirm the depot slot once facilities sign off"
	recordClaim(t, e, contactID, projectID, body, "open", false)

	// The claim names a CONTACT and its row carries their name and what they
	// said. Row scope alone admits nobody to contacts — it narrows a set the
	// object grant has already opened, and for an unbounded actor it is no
	// predicate at all — so a reader without contact:read must be refused
	// rather than handed both.
	view, err := company360Service(e).Assemble(
		e.As(e.Rep1, []ids.UUID{e.Team1}, company360NoContactPerms),
		ids.From[ids.CompanyKind](companyID))
	if err != nil {
		t.Fatalf("assembling the 360: %v", err)
	}
	row := findProject(t, view, projectID)
	if row.Attention != nil {
		t.Fatalf("a reader without contact:read got %q", row.Attention.Title)
	}
	if view.AttentionWithheld == nil || !*view.AttentionWithheld {
		t.Fatal("the payload does not say the reasons were withheld")
	}
}

func TestCompany360_ACommitmentByAnInvisibleContactIsReportedRatherThanDropped(t *testing.T) {
	e := integration.Setup(t)
	companyID := e.SeedCompany(t, "Scoped Account", nil)
	project, err := e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{
		Name: "Depot fit-out", CompanyID: ids.From[ids.CompanyKind](companyID), Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the project: %v", err)
	}
	projectID := ids.UUID(project.Id)
	// Captured PRIVATELY by somebody else. Customer identity is otherwise
	// workspace-readable, so capture privacy is the thing that actually hides
	// a contact from a colleague — an owner change alone does not.
	other := e.Rep3
	contactID := e.SeedContact(t, "Ida Keller", &other)
	recordClaim(t, e, contactID, projectID,
		"we'll confirm the depot slot once facilities sign off", "open", false)
	e.WsExec(t, `UPDATE contact SET visibility = 'owner' WHERE id = $1`, contactID)

	view, err := company360Service(e).Assemble(
		e.As(e.Rep1, []ids.UUID{e.Team1}, company360OwnScopePerms),
		ids.From[ids.CompanyKind](companyID))
	if err != nil {
		t.Fatalf("assembling the 360: %v", err)
	}
	// The commitment is correctly absent — but silence alone would read as a
	// project with nothing outstanding, which is what the flag exists to
	// prevent. Present and unexplained beats absent and misread.
	row := findProject(t, view, projectID)
	if row.Attention != nil {
		t.Fatalf("an out-of-scope contact's claim reached the page as %q", row.Attention.Title)
	}
	if view.AttentionWithheld == nil || !*view.AttentionWithheld {
		t.Fatal("a claim was dropped for row scope and the payload does not say so")
	}
}

// company360NoContactPerms may read the account, its projects and its activities,
// and may not read contacts at all.
var company360NoContactPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"company":               {Read: true},
		"project":               {Read: true},
		"activity":              {Read: true},
		"relationship":          {Read: true},
		"installation_settings": {Read: true},
	},
	RowScope: principal.RowScopeAll,
}

// company360OwnScopePerms holds every grant this card reads and is bounded to its
// own rows — the reader for whom a colleague's privately captured contact is
// invisible rather than forbidden.
var company360OwnScopePerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"company":               {Read: true},
		"contact":               {Read: true},
		"project":               {Read: true},
		"deal":                  {Read: true},
		"activity":              {Read: true},
		"relationship":          {Read: true},
		"installation_settings": {Read: true},
	},
	RowScope: principal.RowScopeOwn,
}

func TestCompany360_ClosedProjectsOverflowingTheCapDoNotClaimMoreWorkInFlight(t *testing.T) {
	e := integration.Setup(t)
	companyID := e.SeedCompany(t, "Portfolio Account", nil)
	// One project in flight and enough closed ones to overflow the cap. The
	// card's order puts the closed ones last, so the cap cuts only history —
	// and a page reporting "1+ in flight" off a bare overflow flag would say
	// this account has live work it does not have.
	live, err := e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{
		Name: "Depot fit-out", CompanyID: ids.From[ids.CompanyKind](companyID), Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the live project: %v", err)
	}
	for i := range 25 {
		closed, err := e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{
			Name:      fmt.Sprintf("Finished %d", i),
			CompanyID: ids.From[ids.CompanyKind](companyID), Source: "manual",
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

	view := assemble(t, e, companyID)
	if view.ProjectsPage == nil {
		t.Fatal("no projects page, so the card cannot tell a full list from a cut one")
	}
	if view.ProjectsPage.HasMore {
		t.Error("the page reports more work in flight, though every project the cap cut is closed")
	}
	if findProject(t, view, ids.UUID(live.Id)).Phase == crmcontracts.Company360ProjectPhaseClosed {
		t.Fatal("the live project is missing from the page, so the assertion above is vacuous")
	}
}
