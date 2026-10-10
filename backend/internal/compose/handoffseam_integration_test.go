// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The handoff reads the project through the page's own service. On one seeded
// project it shows what GET /projects/{id}/360 shows: the same deals, seats and
// open tasks, cut at the same row. The one stated difference is that the
// clock's own reminders are not a promise anyone made.

import (
	"context"
	"fmt"
	"slices"
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

type handoffScene struct {
	project  ids.ProjectID
	contact  ids.UUID
	human    ids.UUID
	reminder ids.UUID
}

// seedHandoffScene files a project with dealCount deals, one seat, one task a
// colleague filed and one reminder the clock filed, through the real writers.
func seedHandoffScene(t *testing.T, e *integration.Env, dealCount int) handoffScene {
	t.Helper()
	admin := e.Admin()
	pipeline, open, _ := integration.DealFixture(t, e)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	companyID := ids.From[ids.CompanyKind](company)
	owner := ids.From[ids.UserKind](e.Rep1)
	created, err := e.Projects.CreateProject(admin, projects.CreateProjectInput{
		Name: "ERP rollout", CompanyID: companyID, OwnerID: &owner, Source: "manual",
	})
	if err != nil {
		t.Fatalf("create the project: %v", err)
	}
	scene := handoffScene{project: ids.From[ids.ProjectKind](ids.UUID(created.Id))}
	for i := range dealCount {
		amount, currency := int64(1_000+i), "EUR"
		if _, err := e.Deals.CreateDeal(admin, deals.CreateDealInput{
			Name: fmt.Sprintf("Licence %02d", i), PipelineID: pipeline, StageID: open, CompanyID: &companyID,
			ProjectID: &scene.project, AmountMinor: &amount, Currency: &currency, OwnerID: &owner,
		}); err != nil {
			t.Fatalf("create deal %d: %v", i, err)
		}
	}
	scene.contact = e.SeedContact(t, "Dana Buyer", &e.Rep1)
	if _, err := e.Contacts.SetProjectStakeholder(admin, contacts.SetProjectStakeholderInput{
		ProjectID: scene.project, ContactID: integration.ContactIDOf(scene.contact), Role: "champion",
	}); err != nil {
		t.Fatalf("seat the stakeholder: %v", err)
	}
	due := time.Now().UTC().Add(-24 * time.Hour)
	onProject := activities.ActivityLinkInput{EntityType: "project", EntityID: scene.project.UUID}
	scene.human = logProjectTask(admin, t, e.Activities, "Send the revised SOW", due, onProject)
	system := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), "system:time-scan")
	scene.reminder = logProjectTask(system, t, e.Activities, "Check in on the quiet project", due, onProject)
	return scene
}

func logProjectTask(ctx context.Context, t *testing.T, store *activities.Store, subject string, due time.Time, link activities.ActivityLinkInput) ids.UUID {
	t.Helper()
	task, _, err := store.LogActivity(ctx, activities.LogActivityInput{
		Kind: "task", Subject: &subject, DueAt: &due, Links: []activities.ActivityLinkInput{link},
	})
	if err != nil {
		t.Fatalf("file the task %q: %v", subject, err)
	}
	return ids.UUID(task.Id)
}

func TestTheHandoffShowsTheProjectAsItsPageDoes(t *testing.T) {
	e := integration.Setup(t)
	scene := seedHandoffScene(t, e, 2)
	admin := e.Admin()

	page, err := newProject360Service(e.Pool).Assemble(admin, scene.project)
	if err != nil {
		t.Fatalf("read the page: %v", err)
	}
	facts, err := handoffReader(e.Pool)(admin, scene.project.UUID)
	if err != nil {
		t.Fatalf("read the handoff: %v", err)
	}

	var pageDeals, handoffDeals []ids.UUID
	for _, d := range page.Deals.Data {
		pageDeals = append(pageDeals, ids.UUID(d.Id))
	}
	for _, d := range facts.Deals {
		handoffDeals = append(handoffDeals, d.DealID)
	}
	if !slices.Equal(pageDeals, handoffDeals) || len(handoffDeals) != 2 {
		t.Errorf("the handoff lists deals %v, the page lists %v", handoffDeals, pageDeals)
	}
	if len(facts.Stakeholders) != 1 || facts.Stakeholders[0].ContactID != scene.contact ||
		len(page.Stakeholders.Data) != 1 || ids.UUID(page.Stakeholders.Data[0].ContactId) != scene.contact {
		t.Errorf("the handoff seats %+v, the page seats %+v", facts.Stakeholders, page.Stakeholders.Data)
	}

	pageTasks := map[ids.UUID]bool{}
	for _, c := range page.Commitments.Data {
		pageTasks[ids.UUID(c.ActivityId)] = true
	}
	if !pageTasks[scene.human] || !pageTasks[scene.reminder] {
		t.Fatalf("the page lists tasks %v, want the filed task and the clock's reminder", pageTasks)
	}
	if len(facts.OpenCommitments) != 1 || facts.OpenCommitments[0].TaskID == nil || *facts.OpenCommitments[0].TaskID != scene.human {
		t.Errorf("the handoff lists %+v, want only the task a colleague filed", facts.OpenCommitments)
	}
}

func TestTheHandoffCutsEachListWhereThePageDoes(t *testing.T) {
	e := integration.Setup(t)
	scene := seedHandoffScene(t, e, 27)
	facts, err := handoffReader(e.Pool)(e.Admin(), scene.project.UUID)
	if err != nil {
		t.Fatalf("read the handoff: %v", err)
	}
	if len(facts.Deals) != 25 || !facts.DealsTruncated {
		t.Errorf("the handoff lists %d deals (truncated %v), want the page's 25 with the cut reported",
			len(facts.Deals), facts.DealsTruncated)
	}
}

// A caller who may read the project and not its contacts gets the seat without
// a name, as the page gives it. The role is what a handover is judged on.
func TestAHandoffWithoutTheContactGrantNamesNoSeatAndStillAnswers(t *testing.T) {
	e := integration.Setup(t)
	scene := seedHandoffScene(t, e, 1)
	reader := e.As(e.Rep1, nil, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects: map[string]principal.ObjectGrant{
			"project":      {Read: true},
			"deal":         {Read: true},
			"activity":     {Read: true},
			"relationship": {Read: true},
		},
		RowScope: principal.RowScopeAll,
	})
	facts, err := handoffReader(e.Pool)(reader, scene.project.UUID)
	if err != nil {
		t.Fatalf("a caller without the contact grant was refused the handoff: %v", err)
	}
	if len(facts.Stakeholders) != 1 || facts.Stakeholders[0].Name != "" || facts.Stakeholders[0].Role != "champion" {
		t.Errorf("the seat is %+v, want the role with no name", facts.Stakeholders)
	}
}

// A section the caller holds no grant for is named, not returned empty.
func TestAHandoffWithoutTheDealGrantNamesTheOmittedList(t *testing.T) {
	e := integration.Setup(t)
	scene := seedHandoffScene(t, e, 1)
	reader := e.As(e.Rep1, nil, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects: map[string]principal.ObjectGrant{
			"project":      {Read: true},
			"contact":      {Read: true},
			"activity":     {Read: true},
			"relationship": {Read: true},
		},
		RowScope: principal.RowScopeAll,
	})
	facts, err := handoffReader(e.Pool)(reader, scene.project.UUID)
	if err != nil {
		t.Fatalf("a caller without the deal grant was refused the handoff: %v", err)
	}
	want := []string{string(crmcontracts.Project360SectionDeals)}
	if !slices.Equal(facts.SectionsOmitted, want) || len(facts.Deals) != 0 {
		t.Errorf("omitted %v with deals %v, want %v named and no deals listed", facts.SectionsOmitted, facts.Deals, want)
	}
}
