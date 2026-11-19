// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An activity filed on a deal is filed on the deal's project too. The account
// here runs two projects, so a note left off its project would show as
// general correspondence on the other one.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type dealNoteFixture struct {
	company, erp, migration ids.UUID
	// erpDeal belongs to erp; looseDeal belongs to no project.
	erpDeal, migrationDeal, looseDeal ids.UUID
}

func seedDealNoteFixture(t *testing.T, e *Env) dealNoteFixture {
	t.Helper()
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	f := dealNoteFixture{
		company:   company,
		erp:       seedProjectInPhase(t, e, company, "ERP rollout", "delivering"),
		migration: seedProjectInPhase(t, e, company, "Datacentre migration", "delivering"),
	}
	pipeline, open, _ := DealFixture(t, e)
	newDeal := func(name string, project ids.UUID) ids.UUID {
		companyID := companyIDOf(company)
		in := deals.CreateDealInput{Name: name, PipelineID: pipeline, StageID: open, CompanyID: &companyID}
		if !project.IsZero() {
			in.ProjectID = new(projectIDOf(project))
		}
		d, err := e.Deals.CreateDeal(e.Admin(), in)
		if err != nil {
			t.Fatalf("creating deal %q: %v", name, err)
		}
		return ids.UUID(d.Id)
	}
	f.erpDeal = newDeal("ERP licences", f.erp)
	f.migrationDeal = newDeal("Rack move", f.migration)
	f.looseDeal = newDeal("Support retainer", ids.Nil)
	return f
}

func logDealNote(t *testing.T, e *Env, ctx context.Context, links ...activities.ActivityLinkInput) ids.UUID {
	t.Helper()
	logged, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{
		Kind: "note", Body: new("Budget confirmed for phase two"), Links: links,
	})
	if err != nil {
		t.Fatalf("logging the note: %v", err)
	}
	return ids.UUID(logged.Id)
}

func onDeal(deal ids.UUID) activities.ActivityLinkInput {
	return activities.ActivityLinkInput{EntityType: "deal", EntityID: deal}
}

func projectsOf(t *testing.T, e *Env, activity ids.UUID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND entity_type = 'project'`, activity)
}

// The note lands on the deal's project with the stamp any filing earns, and
// the project's own timeline lists it.
func TestANoteLoggedOnADealIsFiledUnderTheDealsProject(t *testing.T) {
	e := Setup(t)
	f := seedDealNoteFixture(t, e)

	note := logDealNote(t, e, e.Admin(), onDeal(f.erpDeal))

	if projectLinks(t, e, note, f.erp) != 1 || projectsOf(t, e, note) != 1 {
		t.Fatalf("the note is on %d projects, want exactly the deal's own", projectsOf(t, e, note))
	}
	if stamp := readProjectStamp(t, e, note); stamp.evidence != 1 {
		t.Errorf("the inherited filing left %d project evidence rows, want 1", stamp.evidence)
	}
	page, _, err := e.Activities.ListActivities(e.Admin(), activities.ListActivitiesInput{EntityType: new("project"), EntityID: &f.erp})
	if err != nil {
		t.Fatalf("reading the project timeline: %v", err)
	}
	if len(page) != 1 || ids.UUID(page[0].Id) != note {
		t.Errorf("the project timeline holds %d rows, want the note", len(page))
	}
}

// A project the caller named wins over the deal's, and a deal with no project
// or two deals on two projects add none.
func TestADealsProjectIsInheritedOnlyWhenItIsTheOneAnswer(t *testing.T) {
	e := Setup(t)
	f := seedDealNoteFixture(t, e)

	named := logDealNote(t, e, e.Admin(), onDeal(f.erpDeal),
		activities.ActivityLinkInput{EntityType: "project", EntityID: f.migration})
	if projectLinks(t, e, named, f.migration) != 1 || projectsOf(t, e, named) != 1 {
		t.Errorf("a named project was overridden: the note is on %d projects", projectsOf(t, e, named))
	}
	if n := projectsOf(t, e, logDealNote(t, e, e.Admin(), onDeal(f.looseDeal))); n != 0 {
		t.Errorf("a note on a deal with no project is on %d projects, want none", n)
	}
	if n := projectsOf(t, e, logDealNote(t, e, e.Admin(), onDeal(f.erpDeal), onDeal(f.migrationDeal))); n != 0 {
		t.Errorf("a note on deals of two projects is on %d projects, want none", n)
	}
}

// An agent files no project unattended, so its note on the deal lands on the
// deal alone rather than being refused.
func TestAnAgentsNoteOnADealStaysOffTheProject(t *testing.T) {
	e := Setup(t)
	f := seedDealNoteFixture(t, e)

	note := logDealNote(t, e, e.AgentFor(t, e.AdminUser, nil, AdminPerms), onDeal(f.erpDeal))

	if n := projectsOf(t, e, note); n != 0 {
		t.Errorf("the agent's note is on %d projects, want none", n)
	}
}

// Relinking a logged note onto a deal files it under the deal's project too,
// with its own audit row naming the deal it came from. A later change of the
// deal's project moves nothing already filed.
func TestRelinkingOntoADealFilesTheDealsProjectAndKeepsIt(t *testing.T) {
	e := Setup(t)
	f := seedDealNoteFixture(t, e)
	note := logDealNote(t, e, e.Admin(), activities.ActivityLinkInput{EntityType: "company", EntityID: f.company})

	if _, err := e.Activities.RelinkActivity(e.Admin(), ids.From[ids.ActivityKind](note),
		activities.RelinkActivityInput{EntityType: "deal", EntityID: f.erpDeal}); err != nil {
		t.Fatalf("relinking onto the deal: %v", err)
	}
	if projectLinks(t, e, note, f.erp) != 1 {
		t.Fatal("the relinked note is not on the deal's project")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND action = 'activity_relink'
		AND evidence->>'project_rung' = 'deal' AND evidence->>'deal_id' = $2`, note, f.erpDeal.String()); n != 1 {
		t.Errorf("%d audit rows name the deal the project came from, want 1", n)
	}

	if _, err := e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](f.erpDeal), deals.UpdateDealInput{
		ProjectID: new(projectIDOf(f.migration)),
	}); err != nil {
		t.Fatalf("moving the deal to the other project: %v", err)
	}
	if projectLinks(t, e, note, f.erp) != 1 || projectsOf(t, e, note) != 1 {
		t.Error("moving the deal re-filed a note it had already filed")
	}
}

// The bulk relink door files each row it moves onto the deal's project too.
func TestBulkRelinkingOntoADealFilesTheDealsProject(t *testing.T) {
	e := Setup(t)
	f := seedDealNoteFixture(t, e)
	note := logDealNote(t, e, e.Admin(), activities.ActivityLinkInput{EntityType: "company", EntityID: f.company})

	if _, err := e.Activities.RelinkActivities(e.Admin(), []ids.UUID{note},
		activities.RelinkActivityInput{EntityType: "deal", EntityID: f.erpDeal}); err != nil {
		t.Fatalf("relinking onto the deal: %v", err)
	}
	if projectLinks(t, e, note, f.erp) != 1 {
		t.Error("the bulk-relinked note is not on the deal's project")
	}
}
