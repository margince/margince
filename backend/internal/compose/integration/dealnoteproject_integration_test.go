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
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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
		// Rep1 owns every deal, so Rep1 may file notes onto them.
		in := deals.CreateDealInput{Name: name, PipelineID: pipeline, StageID: open, CompanyID: &companyID, OwnerID: userIDPtr(&e.Rep1)}
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

func logDealNote(ctx context.Context, t *testing.T, e *Env, links ...activities.ActivityLinkInput) ids.UUID {
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

// inheritedMarks counts the marked audit rows that filed activity under project.
func inheritedMarks(t *testing.T, e *Env, activity, project ids.UUID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND action = 'activity_relink'
		AND evidence->>'project_rung' = 'deal' AND after->>'entity_id' = $2`, activity, project.String())
}

// The note lands on the deal's project with the inherited mark, and the
// project's own timeline lists it. The link stamps no retention: while the
// deal is unqualified the note stays erasable.
func TestANoteLoggedOnADealIsFiledUnderTheDealsProject(t *testing.T) {
	e := Setup(t)
	f := seedDealNoteFixture(t, e)

	note := logDealNote(e.Admin(), t, e, onDeal(f.erpDeal))

	if projectLinks(t, e, note, f.erp) != 1 || projectsOf(t, e, note) != 1 {
		t.Fatalf("the note is on %d projects, want exactly the deal's own", projectsOf(t, e, note))
	}
	if n := inheritedMarks(t, e, note, f.erp); n != 1 {
		t.Errorf("%d marked audit rows record the inherited link, want 1", n)
	}
	if stamp := readProjectStamp(t, e, note); stamp.class != nil || stamp.evidence != 0 {
		t.Errorf("the inherited link stamped retention: class=%v evidence=%d, want neither", stamp.class, stamp.evidence)
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

	named := logDealNote(e.Admin(), t, e, onDeal(f.erpDeal),
		activities.ActivityLinkInput{EntityType: "project", EntityID: f.migration})
	if projectLinks(t, e, named, f.migration) != 1 || projectsOf(t, e, named) != 1 {
		t.Errorf("a named project was overridden: the note is on %d projects", projectsOf(t, e, named))
	}
	if n := projectsOf(t, e, logDealNote(e.Admin(), t, e, onDeal(f.looseDeal))); n != 0 {
		t.Errorf("a note on a deal with no project is on %d projects, want none", n)
	}
	if n := projectsOf(t, e, logDealNote(e.Admin(), t, e, onDeal(f.erpDeal), onDeal(f.migrationDeal))); n != 0 {
		t.Errorf("a note on deals of two projects is on %d projects, want none", n)
	}
}

// Only a member's own write inherits. An agent's note and a system note land
// on the deal alone rather than being refused.
func TestOnlyAMembersNoteOnADealTakesTheProject(t *testing.T) {
	e := Setup(t)
	f := seedDealNoteFixture(t, e)

	for name, ctx := range map[string]context.Context{
		"agent":  e.AgentFor(t, e.AdminUser, nil, AdminPerms),
		"system": principal.SystemActing(e.Admin(), "system:deal-room"),
	} {
		if n := projectsOf(t, e, logDealNote(ctx, t, e, onDeal(f.erpDeal))); n != 0 {
			t.Errorf("the %s note is on %d projects, want none", name, n)
		}
	}
}

// A member who may create notes but not change them files the deal alone:
// the refused project link does not refuse the note.
func TestAMemberWithoutUpdateFilesTheDealAlone(t *testing.T) {
	e := Setup(t)
	f := seedDealNoteFixture(t, e)
	perms := dealNotePerms(principal.ObjectGrant{Create: true, Read: true})

	note := logDealNote(e.As(e.Rep1, []ids.UUID{e.Team1}, perms), t, e, onDeal(f.erpDeal))

	if n := projectsOf(t, e, note); n != 0 {
		t.Errorf("a note by a member without activity.update is on %d projects, want none", n)
	}
}

// dealNotePerms is a team-scoped member reading deals and projects, with the
// given activity grant.
func dealNotePerms(activity principal.ObjectGrant) principal.Permissions {
	return principal.Permissions{
		RoleKeys: []string{roleRep},
		Objects: map[string]principal.ObjectGrant{
			objActivity: activity, objDeal: {Read: true, Update: true},
			"project": {Read: true}, objCompany: {Read: true},
		},
		RowScope: principal.RowScopeTeam,
	}
}

// Relinking a logged note onto a deal files it under the deal's project too,
// with its own audit row naming the deal it came from. A later change of the
// deal's project moves nothing already filed.
func TestRelinkingOntoADealFilesTheDealsProjectAndKeepsIt(t *testing.T) {
	e := Setup(t)
	f := seedDealNoteFixture(t, e)
	note := logDealNote(e.Admin(), t, e, activities.ActivityLinkInput{EntityType: "company", EntityID: f.company})

	if _, err := e.Activities.RelinkActivity(e.Admin(), ids.From[ids.ActivityKind](note),
		activities.RelinkActivityInput{EntityType: "deal", EntityID: f.erpDeal}); err != nil {
		t.Fatalf("relinking onto the deal: %v", err)
	}
	if projectLinks(t, e, note, f.erp) != 1 {
		t.Fatal("the relinked note is not on the deal's project")
	}
	if n := inheritedMarks(t, e, note, f.erp); n != 1 {
		t.Errorf("%d marked audit rows record the inherited link, want 1", n)
	}
	if stamp := readProjectStamp(t, e, note); stamp.evidence != 0 {
		t.Errorf("the relinked inherited link left %d project evidence rows, want none", stamp.evidence)
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

// The bulk and thread relink doors file each row they move onto the deal's
// project too.
func TestBatchRelinkingOntoADealFilesTheDealsProject(t *testing.T) {
	e := Setup(t)
	f := seedDealNoteFixture(t, e)
	onCompany := activities.ActivityLinkInput{EntityType: "company", EntityID: f.company}
	toDeal := activities.RelinkActivityInput{EntityType: "deal", EntityID: f.erpDeal}
	bulk, thread := logDealNote(e.Admin(), t, e, onCompany), logDealNote(e.Admin(), t, e, onCompany)

	if _, err := e.Activities.RelinkActivities(e.Admin(), []ids.UUID{bulk}, toDeal); err != nil {
		t.Fatalf("bulk relinking onto the deal: %v", err)
	}
	if _, err := e.Activities.RelinkThread(e.Admin(), ids.From[ids.ActivityKind](thread), toDeal); err != nil {
		t.Fatalf("thread relinking onto the deal: %v", err)
	}
	for name, note := range map[string]ids.UUID{"bulk": bulk, "thread": thread} {
		if projectLinks(t, e, note, f.erp) != 1 {
			t.Errorf("the %s-relinked note is not on the deal's project", name)
		}
	}
}
