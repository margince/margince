// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// leadTaskCard is what a lead row says about its open tasks.
type leadTaskCard struct {
	open    int
	subject string
	due     time.Time
}

func leadTaskCardOf(t *testing.T, lead crmcontracts.Lead) leadTaskCard {
	t.Helper()
	if lead.OpenTaskCount == nil {
		t.Fatalf("lead %s has no open task count", lead.Id)
	}
	card := leadTaskCard{open: *lead.OpenTaskCount}
	if lead.NextTaskSubject != nil {
		card.subject = *lead.NextTaskSubject
	}
	if lead.NextTaskDueAt != nil {
		card.due = lead.NextTaskDueAt.UTC()
	}
	return card
}

type leadTaskFixture struct {
	e     *integration.Env
	now   time.Time
	names map[crmcontracts.Id]string
}

func (f *leadTaskFixture) lead(t *testing.T, name string, owner ids.UUID) ids.UUID {
	t.Helper()
	ownerID := ids.From[ids.UserKind](owner)
	lead, _, err := f.e.Contacts.CreateLead(f.e.Admin(), contacts.CreateLeadInput{
		FullName: &name, LinkedInURL: new("https://www.linkedin.com/in/open-tasks-" + ids.NewV7().String()), Source: "manual", OwnerID: &ownerID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if lead.OwnerId == nil || ids.UUID(*lead.OwnerId) != owner {
		t.Fatalf("%s is owned by %v, want %s", name, lead.OwnerId, owner)
	}
	f.names[lead.Id] = name
	return ids.UUID(lead.Id)
}

func (f *leadTaskFixture) task(t *testing.T, lead ids.UUID, subject string, due *time.Time) ids.ActivityID {
	t.Helper()
	task, _, err := f.e.Activities.LogActivity(f.e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, DueAt: due, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "lead", EntityID: lead}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids.From[ids.ActivityKind](ids.UUID(task.Id))
}

func (f *leadTaskFixture) limit(t *testing.T, task ids.ActivityID) {
	t.Helper()
	if _, err := f.e.Activities.SetAudience(f.e.Admin(), task, activities.SetAudienceInput{
		Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: f.e.Rep2}},
	}); err != nil {
		t.Fatal(err)
	}
}

// The lead read counts every open current task, and names the soonest due one
// that every seat may read, with that task's own deadline. Done, archived and
// limited tasks drop out where the card says so, for every reader and every
// read the lead list offers.
func TestALeadCardCountsItsOpenTasksAndNamesTheNextReadableOne(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	f := &leadTaskFixture{e: e, now: clocktest.Now(t), names: map[crmcontracts.Id]string{}}
	day := func(n int) *time.Time { return new(f.now.AddDate(0, 0, n)) }

	busy := f.lead(t, "Busy lead", e.Rep1)
	callBack := day(3)
	f.task(t, busy, "Call back", callBack)
	f.task(t, busy, "Book a demo", day(3))
	f.limit(t, f.task(t, busy, "Send the private pricing", day(1)))
	f.task(t, busy, "Someday", nil)
	done := f.task(t, busy, "Already done", day(0))
	if completed, err := e.Activities.CompleteTask(e.Admin(), done, clocktest.Now(t)); err != nil || !completed {
		t.Fatalf("completing a task: %t %v", completed, err)
	}
	archived := f.task(t, busy, "Dropped", day(0))
	if _, err := e.Activities.ArchiveActivity(e.Admin(), archived, nil); err != nil {
		t.Fatal(err)
	}
	hidden := f.lead(t, "Only a limited task", e.Rep1)
	f.limit(t, f.task(t, hidden, "Private follow-up", day(1)))
	elsewhere := f.lead(t, "Another rep's lead", e.Rep2)
	theirs := day(2)
	f.task(t, elsewhere, "Their call", theirs)
	idle := f.lead(t, "No tasks", e.Rep1)

	want := map[ids.UUID]leadTaskCard{
		busy:      {open: 4, subject: "Call back", due: *callBack},
		hidden:    {open: 1},
		elsewhere: {open: 1, subject: "Their call", due: *theirs},
		idle:      {open: 0},
	}
	// Every seat reads every lead, so each reader below sees all four. The
	// second rep is in the limited tasks' audience and still gets the card
	// every seat reads.
	ownLeads := principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"lead": {Read: true}, "activity": {Read: true}},
		RowScope: principal.RowScopeOwn,
	}
	readers := map[string]context.Context{
		"admin":                       e.Admin(),
		"rep scoped to their own":     e.As(e.Rep1, []ids.UUID{e.Team1}, ownLeads),
		"rep in the limited audience": e.As(e.Rep2, []ids.UUID{e.Team1}, ownLeads),
	}
	for name, reader := range readers {
		t.Run(name, func(t *testing.T) {
			f.assertReads(reader, t, want)
		})
	}
}

func (f *leadTaskFixture) assertReads(ctx context.Context, t *testing.T, want map[ids.UUID]leadTaskCard) {
	t.Helper()
	for id, card := range want {
		got, err := f.e.Contacts.GetLead(ctx, ids.From[ids.LeadKind](id), storekit.LiveOnly)
		if err != nil {
			t.Fatalf("%s: %v", f.names[crmcontracts.Id(id)], err)
		}
		if got := leadTaskCardOf(t, got); got != card {
			t.Errorf("%s read alone: %+v, want %+v", f.names[crmcontracts.Id(id)], got, card)
		}
	}
	for _, sort := range []*string{nil, new("next_task_due_at"), new("full_name")} {
		rows, _, err := f.e.Contacts.ListLeads(ctx, contacts.ListLeadsInput{Sort: sort})
		if err != nil {
			t.Fatal(err)
		}
		var listed []ids.UUID
		for _, row := range rows {
			card, ours := want[ids.UUID(row.Id)]
			if !ours {
				continue
			}
			listed = append(listed, ids.UUID(row.Id))
			if got := leadTaskCardOf(t, row); got != card {
				t.Errorf("%s listed by %v: %+v, want %+v", f.names[row.Id], sort, got, card)
			}
		}
		if len(listed) != len(want) {
			t.Errorf("list by %v holds %d of the fixture's leads, want %d", sort, len(listed), len(want))
		}
		if sort != nil && *sort == "next_task_due_at" {
			f.assertDueOrder(t, rows, want)
		}
	}
}

// assertDueOrder checks that a list sorted by next_task_due_at is in the order
// of the deadlines it prints, with the leads that print none last.
func (f *leadTaskFixture) assertDueOrder(t *testing.T, rows []crmcontracts.Lead, want map[ids.UUID]leadTaskCard) {
	t.Helper()
	var dues []time.Time
	for _, row := range rows {
		if _, ours := want[ids.UUID(row.Id)]; ours {
			dues = append(dues, leadTaskCardOf(t, row).due)
		}
	}
	for i := 1; i < len(dues); i++ {
		earlier, later := dues[i-1], dues[i]
		if earlier.IsZero() && !later.IsZero() || !earlier.IsZero() && !later.IsZero() && later.Before(earlier) {
			t.Errorf("next_task_due_at order: %v", dues)
		}
	}
}
