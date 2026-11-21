// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// A contact page lists its open tasks up to a cap. A promise whose task sorts
// past the cap is still owed, so its claim says the task is open. A claim
// whose task is done or archived says it is not, and the page leaves it out.
func TestALinkedTaskPastTheCapLeavesItsPromiseOwed(t *testing.T) {
	e := Setup(t)
	p := e.SeedPromiseThread(t)
	pastCap := e.SeedPromiseTask(t, p)
	earlier := p.SentAt.AddDate(0, 0, -5)
	for range 30 {
		if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
			Kind: "task", Subject: new("Prepare the site visit"), DueAt: &earlier, Source: provenance.RecordSourceManual,
			Links: []activities.ActivityLinkInput{{EntityType: objContact, EntityID: p.Contact.UUID}},
		}); err != nil {
			t.Fatalf("filing an earlier task: %v", err)
		}
	}
	done := e.promiseOnItsOwnTask(t, p, "Send the floor plan")
	if _, err := e.Activities.UpdateActivity(e.Admin(), ids.From[ids.ActivityKind](done), activities.UpdateActivityInput{IsDone: new(true)}); err != nil {
		t.Fatalf("completing a task: %v", err)
	}
	archived := e.promiseOnItsOwnTask(t, p, "Send the energy report")
	if _, err := e.Activities.ArchiveActivity(e.Admin(), ids.From[ids.ActivityKind](archived), nil); err != nil {
		t.Fatalf("archiving a task: %v", err)
	}

	asOf := p.SentAt.AddDate(0, 0, 1)
	svc := contact360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects, consent.NewStore(e.DB()),
		comms.NewStore(e.DB(), func() time.Time { return asOf }, activities.NewStore(e.DB())), ai.NewFeedbackStore(e.DB()),
		func() time.Time { return asOf })
	page, err := svc.Assemble(e.Admin(), p.Contact)
	if err != nil {
		t.Fatalf("assembling the page: %v", err)
	}
	if page.NextSteps == nil || !page.NextSteps.Page.HasMore {
		t.Fatal("the fixture did not fill next_steps past its cap, so it proves nothing about a task beyond it")
	}
	for _, task := range page.NextSteps.Data {
		if ids.UUID(task.Id) == pastCap {
			t.Fatal("the promise's task is on the capped page, so the fixture does not put it past the cap")
		}
	}
	want := map[ids.UUID]bool{pastCap: true, done: false, archived: false}
	for _, claim := range *page.Claims {
		if claim.TaskActivityId == nil {
			continue
		}
		task := ids.UUID(*claim.TaskActivityId)
		expected, known := want[task]
		if !known {
			continue
		}
		delete(want, task)
		if claim.TaskOpen == nil {
			t.Errorf("the claim on task %s carries no task_open, want %v", task, expected)
		} else if *claim.TaskOpen != expected {
			t.Errorf("the claim on task %s says task_open=%v, want %v", task, *claim.TaskOpen, expected)
		}
	}
	if len(want) != 0 {
		t.Errorf("the page carries no claim for tasks %v", want)
	}
}

// promiseOnItsOwnTask records one more promise from the thread and files the
// task that stands for it. Its own quote makes it a claim of its own.
func (e *Env) promiseOnItsOwnTask(t *testing.T, p PromiseThread, body string) ids.UUID {
	t.Helper()
	task, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &body, Source: provenance.RecordSourceManual,
		Links: []activities.ActivityLinkInput{{EntityType: objContact, EntityID: p.Contact.UUID}},
	})
	if err != nil {
		t.Fatalf("filing the task for %q: %v", body, err)
	}
	claim := e.RecordClaim(e.Admin(), t, contacts.ClaimInput{
		ContactID: p.Contact, Kind: "commitment_ours", Body: body, ActivityID: ids.UUID(p.Claim.SourceActivityId),
		Quote: "I will " + body + " this week.", Source: provenance.RecordSourceManual,
	})
	e.WsExec(t, `UPDATE conversation_claim SET task_activity_id = $1 WHERE id = $2`, ids.UUID(task.Id), ids.UUID(claim.Id))
	return ids.UUID(task.Id)
}
