// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package company360

// An overdue task the reader may not open, against a real database.
//
// Only a real gate can produce this: the activity row scope is SQL, and what it
// drops is invisible to a query that filters on it. The unit lane sees a map of
// tasks and cannot tell one that was never there from one that was withheld.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A project whose ONLY overdue task is out of the caller's activity row scope.
//
// This is the case the card must never get wrong: with the scope applied as a
// filter, the query returned nothing for this project and the row rendered
// exactly like a project with nothing outstanding. A rep reads the account as
// calm and moves on, and the task stays overdue.
func TestCompany360_WorkWhoseOnlyOverdueTaskIsWithheldSaysSo(t *testing.T) {
	e := integration.Setup(t)
	companyID := e.SeedCompany(t, "Quiet Account", nil)
	projectID := seedProject(t, e, companyID, "Depot retrofit")

	logProjectTask(t, e, "Chase the signature", overdueAt, projectID, companyID)
	// Narrowed to its participants, which is what actually hides an activity
	// from a colleague: the audience arm of the content clause refuses a reader
	// who is not on it, whatever their object grant says.
	withholdEveryTask(t, e, projectID)

	view := assembleScoped(t, e, companyID)

	row := findProject(t, view, projectID)
	if row.Attention != nil {
		t.Fatalf("a withheld task reached the page as %q", row.Attention.Title)
	}
	if view.AttentionWithheld == nil || !*view.AttentionWithheld {
		t.Fatal("the only overdue task was withheld and the payload does not say so — " +
			"the card reads as work with nothing outstanding")
	}
}

// And the mixed case, which is what keeps the fix from being "report nothing
// and flag everything": a visible task is still the reason, and the withheld
// one beside it still sets the flag.
func TestCompany360_AVisibleTaskIsStillTheReasonWhenAnotherIsWithheld(t *testing.T) {
	e := integration.Setup(t)
	companyID := e.SeedCompany(t, "Mixed Account", nil)
	projectID := seedProject(t, e, companyID, "Fleet retrofit")

	// The WITHHELD one is overdue by longest, so it would win the ordering if
	// visibility were not asked first — and the reader would be shown a subject
	// they may not read.
	logProjectTask(t, e, "Confidential escalation", overdueAt, projectID, companyID)
	withholdEveryTask(t, e, projectID)
	logProjectTask(t, e, "Send the retrofit quote", laterOverdueAt, projectID, companyID)

	view := assembleScoped(t, e, companyID)

	row := findProject(t, view, projectID)
	if row.Attention == nil {
		t.Fatal("a visible overdue task produced no reason at all")
	}
	if row.Attention.Title != "Send the retrofit quote" {
		t.Fatalf("reason = %q — the card is showing a task this reader may not open", row.Attention.Title)
	}
	if row.Attention.Kind != crmcontracts.Company360WorkAttentionKindWorkAttentionOverdueTask {
		t.Fatalf("kind = %q, want an overdue task", row.Attention.Kind)
	}
	if view.AttentionWithheld == nil || !*view.AttentionWithheld {
		t.Fatal("a second task was withheld and the payload claims the statuses are complete")
	}
}

// assembleScoped reads the page as a rep bounded to its own rows — the reader
// for whom an activity narrowed to its participants is invisible rather than
// forbidden. The admin reader every other case here uses would see both tasks,
// which is exactly the case that cannot exercise this.
func assembleScoped(t *testing.T, e *integration.Env, companyID ids.UUID) crmcontracts.Company360 {
	t.Helper()
	view, err := company360Service(e).Assemble(
		e.As(e.Rep1, []ids.UUID{e.Team1}, company360OwnScopePerms),
		ids.From[ids.CompanyKind](companyID))
	if err != nil {
		t.Fatalf("assembling the 360: %v", err)
	}
	return view
}

func seedProject(t *testing.T, e *integration.Env, companyID ids.UUID, name string) ids.UUID {
	t.Helper()
	project, err := e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{
		Name: name, CompanyID: ids.From[ids.CompanyKind](companyID), Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the project %q: %v", name, err)
	}
	return ids.UUID(project.Id)
}

// logProjectTask files an overdue task against the project AND the account.
// Both links, because the read walks the account: a task linked only to the
// project never reaches this page at all, which would make a case about
// withholding pass for the wrong reason.
func logProjectTask(
	t *testing.T, e *integration.Env, subject string, due time.Time,
	projectID, companyID ids.UUID,
) {
	t.Helper()
	logTaskUnder(t, e, subject, due,
		activities.ActivityLinkInput{EntityType: "project", EntityID: projectID},
		activities.ActivityLinkInput{EntityType: "company", EntityID: companyID})
}

// withholdEveryTask narrows every task currently linked to the project to its
// participants. Applied after the tasks that must stay readable are written it
// would narrow those too, which is why each caller seeds in the order it does.
func withholdEveryTask(t *testing.T, e *integration.Env, projectID ids.UUID) {
	t.Helper()
	e.WsExec(t, `
		UPDATE activity SET audience = 'participants'
		 WHERE kind = 'task'
		   AND id IN (SELECT activity_id FROM activity_link WHERE project_id = $1)`, projectID)
}
