// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The promise a person page leads with is the FIRST row of its next-steps
// section, so which row that is has to be decided by the database rather than
// by re-sorting what the section happened to carry. The section is capped at
// 25 rows; a rung re-sorting them in memory cannot see the 26th, and the row
// it cannot see is exactly the one a busy record has waited longest on.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/person360"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheOldestPromiseLeadsEvenPastTheSectionCap(t *testing.T) {
	e := integration.Setup(t)
	person := seedLinkedPerson(t, e, "vielbeschaeftigt@kunde.example")
	personID := ids.From[ids.PersonKind](person)
	now := time.Now()

	// Filed FIRST and due SOONEST, then buried under thirty newer tasks. In
	// the timeline's order (newest first) this row sits past the section's
	// 25-row cap, so a page that carried the newest 25 could not name it.
	oldest := logTaskFor(t, e, person, "Send the signed contract", at(now.Add(24*time.Hour)))
	for i := range 30 {
		logTaskFor(t, e, person, "Later chore "+string(rune('a'+i%26)), at(now.Add(time.Duration(200+i)*time.Hour)))
	}

	svc := person360.NewService(e.Pool, e.People, e.Deals, e.Projects,
		consent.NewStore(InstallationDB(e.Pool)),
		comms.NewStore(InstallationDB(e.Pool), time.Now, activities.NewStore(InstallationDB(e.Pool))),
		ai.NewFeedbackStore(InstallationDB(e.Pool)), time.Now)

	page, err := svc.Assemble(e.Admin(), personID)
	if err != nil {
		t.Fatalf("assembling person360: %v", err)
	}
	if page.NextSteps == nil || len(page.NextSteps.Data) == 0 {
		t.Fatal("the next-steps section is empty; the ordering below would prove nothing")
	}
	if !page.NextSteps.Page.HasMore {
		t.Fatalf("the section carries %d of 31 open tasks without saying so — this test needs the "+
			"cap to bite, and a reader needs to know the list is partial", len(page.NextSteps.Data))
	}
	if got := page.NextSteps.Data[0].Id; ids.UUID(got) != oldest {
		t.Errorf("the section leads with %v, want the soonest-due task %v — the card names the first "+
			"row, so a section ordered by filing date hands it a chore instead of the contract",
			got, oldest)
	}
	if page.Moment == nil {
		t.Fatal("no moment on a record that owes a signed contract")
	}
	if page.Moment.Headline != "You owe them: Send the signed contract" {
		t.Errorf("headline = %q, want the soonest-due promise", page.Moment.Headline)
	}
}

// at is a due date as the writer wants it. The package's own ptr helper takes
// an int.
func at(t time.Time) *time.Time { return &t }

// The card says whose promise it is, and it can only do that if the assignee
// survives the section's own projection. It did not: the query selected
// due_at and is_done but not assignee_id, so every assembled task looked
// unassigned and every reader was told they owed a colleague's work.
func TestAnAssignedPromiseKeepsItsHolderThroughTheSection(t *testing.T) {
	e := integration.Setup(t)
	person := seedLinkedPerson(t, e, "zugewiesen@kunde.example")
	personID := ids.From[ids.PersonKind](person)

	holder := ids.From[ids.UserKind](e.Rep1)
	logAssignedTaskFor(t, e, person, "Send the signed contract", holder)

	svc := person360.NewService(e.Pool, e.People, e.Deals, e.Projects,
		consent.NewStore(InstallationDB(e.Pool)),
		comms.NewStore(InstallationDB(e.Pool), time.Now, activities.NewStore(InstallationDB(e.Pool))),
		ai.NewFeedbackStore(InstallationDB(e.Pool)), time.Now)

	page, err := svc.Assemble(e.Admin(), personID)
	if err != nil {
		t.Fatalf("assembling person360: %v", err)
	}
	if page.NextSteps == nil || len(page.NextSteps.Data) != 1 {
		t.Fatalf("the section carries %v tasks, want the one just written", page.NextSteps)
	}
	if got := page.NextSteps.Data[0].AssigneeId; got == nil {
		t.Fatal("the task came back unassigned — the card then tells every reader they owe a " +
			"colleague's promise, and the frontend has nothing to tell them apart with")
	}
	if page.Moment == nil || page.Moment.Headline != "Owed to them: Send the signed contract" {
		t.Errorf("headline = %v, want the promise attributed to the desk that holds it", page.Moment)
	}
}

// logAssignedTaskFor writes one open task somebody holds, through the real
// activity writer.
func logAssignedTaskFor(t *testing.T, e *integration.Env, person ids.UUID, subject string, assignee ids.UserID) {
	t.Helper()
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, Source: "manual", AssigneeID: &assignee,
		Links: []activities.ActivityLinkInput{{EntityType: "person", EntityID: person}},
	}); err != nil {
		t.Fatalf("logging an assigned task: %v", err)
	}
}

// logTaskFor writes one open task filed against a person, through the real
// activity writer — a hand-inserted row would not exercise the links the
// section reads through.
func logTaskFor(t *testing.T, e *integration.Env, person ids.UUID, subject string, due *time.Time) ids.UUID {
	t.Helper()
	row, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, DueAt: due, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "person", EntityID: person}},
	})
	if err != nil {
		t.Fatalf("logging task %q: %v", subject, err)
	}
	return ids.UUID(row.Id)
}
