// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
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

	page, err := taskOpenService(e, p.SentAt.AddDate(0, 0, 1)).Assemble(e.Admin(), p.Contact)
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

// A task filed only under a contact private to a colleague is hidden from a
// rep on another team. Their page still shows the promise, and says its task is
// not open to them, so the promise is not counted as owed.
func TestATaskTheReaderCannotSeeLeavesNoPromiseOwed(t *testing.T) {
	e := Setup(t)
	colleague := e.As(e.Rep3, []ids.UUID{e.Team2}, activityLifecyclePerms)
	contact := ids.From[ids.ContactKind](e.SeedContact(t, "Lena Kunde", &e.Rep3))
	private := e.SeedContact(t, "Their Contact", &e.Rep3)
	e.MakeCapturePrivate(t, objContact, private, e.Rep3)
	note, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "note", Subject: new("Call notes"), Body: new("I will send the floor plan."), Source: provenance.RecordSourceManual,
		Links: []activities.ActivityLinkInput{{EntityType: objContact, EntityID: contact.UUID}},
	})
	if err != nil {
		t.Fatalf("logging the source note: %v", err)
	}
	claim := e.RecordClaim(e.Admin(), t, contacts.ClaimInput{
		ContactID: contact, Kind: "commitment_ours", Body: "Send the floor plan", ActivityID: ids.UUID(note.Id),
		Quote: "I will send the floor plan.", Source: provenance.RecordSourceManual,
	})
	task, _, err := e.Activities.LogActivity(colleague, activities.LogActivityInput{
		Kind: "task", Subject: new("Send the floor plan"), Source: provenance.RecordSourceManual,
		Links: []activities.ActivityLinkInput{{EntityType: objContact, EntityID: private}},
	})
	if err != nil {
		t.Fatalf("filing the hidden task: %v", err)
	}
	e.LinkClaimTask(colleague, t, ids.UUID(claim.Id), ids.UUID(task.Id))
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	if _, err := e.Activities.GetActivity(rep, ids.From[ids.ActivityKind](ids.UUID(task.Id)), storekit.LiveOnly); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("the rep reads the task (%v), so the fixture does not hide it", err)
	}

	page, err := taskOpenService(e, time.Now()).Assemble(rep, contact)
	if err != nil {
		t.Fatalf("assembling the rep's page: %v", err)
	}
	if page.Claims == nil {
		t.Fatal("the rep's page carries no claims section, so it proves nothing about task_open")
	}
	for _, got := range *page.Claims {
		if got.Id != claim.Id {
			continue
		}
		if got.TaskOpen == nil {
			t.Error("the claim on a task the rep cannot see carries no task_open, want false")
		} else if *got.TaskOpen {
			t.Error("the claim on a task the rep cannot see says task_open=true, want false")
		}
		return
	}
	t.Error("the rep's page does not carry the claim")
}

func taskOpenService(e *Env, asOf time.Time) *contact360.Service {
	return contact360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects, consent.NewStore(e.DB()),
		comms.NewStore(e.DB(), func() time.Time { return asOf }, activities.NewStore(e.DB())), ai.NewFeedbackStore(e.DB()),
		func() time.Time { return asOf })
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
	e.LinkClaimTask(e.Admin(), t, ids.UUID(claim.Id), ids.UUID(task.Id))
	return ids.UUID(task.Id)
}
