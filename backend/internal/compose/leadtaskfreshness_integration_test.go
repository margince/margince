// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestLeadAndAssignedTaskReadsKeepHistoricalAutomaticWorkOut(t *testing.T) {
	e := integration.Setup(t)
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(-2, 0, 0)
	assignee := ids.From[ids.UserKind](e.AdminUser)
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: new("Task prospect"), LinkedInURL: new("https://www.linkedin.com/in/task-prospect"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	leadID := ids.From[ids.LeadKind](ids.UUID(lead.Id))
	source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "meeting", Subject: new("Old prospect meeting"), OccurredAt: &old, MeetingStatus: new("held"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	var tasks []crmcontracts.Activity
	for _, manual := range []bool{false, true} {
		actor := principal.SystemActing(e.Admin(), "system:signal_scan")
		title := "Old automatic commitment"
		if manual {
			actor = e.Admin()
			title = "My chosen follow-up"
		}
		task, _, err := e.Activities.LogActivity(actor, activities.LogActivityInput{Kind: "task", Subject: &title, DueAt: &old, AssigneeID: &assignee, Source: "manual", SourceSystem: new(activities.CommitmentTaskSource), SourceID: new(ids.NewV7().String()), SourceActivityID: new(ids.UUID(source.Id)), Links: []activities.ActivityLinkInput{{EntityType: "lead", EntityID: ids.UUID(lead.Id)}}})
		if err != nil {
			t.Fatal(err)
		}
		tasks = append(tasks, task)
	}
	assertLead := func(t *testing.T, got crmcontracts.Lead) {
		t.Helper()
		if got.OpenTaskCount == nil || *got.OpenTaskCount != 1 || got.NextTaskSubject == nil || *got.NextTaskSubject != "My chosen follow-up" {
			t.Fatalf("current lead work: %+v", got)
		}
	}
	got, err := e.Contacts.GetLead(e.Admin(), leadID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	assertLead(t, got)
	found, err := e.Contacts.FindLeadByLinkedInURL(e.Admin(), "https://www.linkedin.com/in/task-prospect")
	if err != nil || found == nil {
		t.Fatalf("lookup: %v %v", found, err)
	}
	assertLead(t, *found)
	for _, sort := range []*string{nil, new("next_task_due_at"), new("full_name")} {
		rows, _, err := e.Contacts.ListLeads(e.Admin(), contacts.ListLeadsInput{Sort: sort})
		if err != nil {
			t.Fatal(err)
		}
		matched := false
		for _, row := range rows {
			if row.Id == lead.Id {
				assertLead(t, row)
				matched = true
			}
		}
		if !matched {
			t.Fatal("lead missing from list")
		}
	}
	store := e.Activities.WithClock(func() time.Time { return now })
	in := activities.ListActivitiesInput{AssigneeID: &assignee, EntityType: new("lead"), EntityID: new(ids.UUID(lead.Id))}
	rows, _, err := store.ListActivities(e.Admin(), in)
	if err != nil {
		t.Fatal(err)
	}
	count, err := store.CountActivities(e.Admin(), in)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || len(rows) != 1 || rows[0].Id != tasks[1].Id {
		t.Fatalf("assigned page/count: %d %+v", count, rows)
	}
	history, _, err := store.ListActivities(e.Admin(), activities.ListActivitiesInput{EntityType: new("lead"), EntityID: new(ids.UUID(lead.Id))})
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 {
		t.Fatalf("history lost tasks: %+v", history)
	}
	if _, err := store.GetActivity(e.Admin(), ids.From[ids.ActivityKind](ids.UUID(tasks[0].Id)), storekit.LiveOnly); err != nil {
		t.Fatal(err)
	}
}
