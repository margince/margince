// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// An open task more than a month overdue leaves the Worklist, its count and
// the team board's overdue figure, unless a pin the reader holds in effect keeps
// it. The task stays open on its contact.
func TestATaskMoreThanAMonthOverdueLeavesTheWorklistUnlessPinned(t *testing.T) {
	e := integration.Setup(t)
	now := time.Now().UTC()
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Mara Lindqvist"})
	if err != nil {
		t.Fatalf("creating the contact: %v", err)
	}
	contactID := ids.UUID(contact.Id)
	logOn := func(subject string, overdueDays int) string {
		due := now.AddDate(0, 0, -overdueDays)
		row, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
			Kind: "task", Subject: &subject, DueAt: &due, Source: "manual",
			Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contactID}},
		})
		if err != nil {
			t.Fatalf("logging task %q: %v", subject, err)
		}
		return ids.UUID(row.Id).String()
	}
	agedOut := logOn("Send the revised quote", 31)
	recent := logOn("Confirm the workshop date", 29)
	pinned := logOn("Chase the signed contract", 31)
	pushedOut := logOn("Return the signed NDA", 31)
	pin := func(rowID string) {
		if err := e.Activities.PinWorklistRow(e.Admin(), activities.WorklistRowRef{
			Source: string(crmcontracts.WorklistItemSourceTask), RowID: rowID,
		}); err != nil {
			t.Fatalf("pinning %s: %v", rowID, err)
		}
	}
	// The oldest pin, then 49 newer ones and the kept task's: a reader keeps
	// their 50 newest pins in effect, so pushedOut's pin is the 51st and holds
	// nothing on the Worklist.
	pin(pushedOut)
	for range 49 {
		pin(ids.NewV7().String())
	}
	pin(pinned)

	day := assembleFeed(e.Admin(), t, e, now)
	if got := taskIDsOn(day.Planned); !slices.Equal(got, sorted(recent, pinned)) {
		t.Errorf("the planned lane carries %v, want the 29-day task %s and the pinned one %s", got, recent, pinned)
	}
	if day.Counts.Planned != 2 {
		t.Errorf("the planned count is %d, want 2: the badge must drop what the page drops", day.Counts.Planned)
	}

	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return now })
	page, err := feed.Worklist(e.Admin(), "mine", "all", ids.Nil, 100, "")
	if err != nil {
		t.Fatalf("reading the Worklist: %v", err)
	}
	var queued []string
	for _, row := range page.Queue {
		if row.Source == crmcontracts.WorklistItemSourceTask {
			queued = append(queued, row.Id)
		}
	}
	slices.Sort(queued)
	if !slices.Equal(queued, sorted(recent, pinned)) {
		t.Errorf("the Worklist queue carries tasks %v, want %s and the pinned %s", queued, recent, pinned)
	}

	load, err := e.Activities.OverdueLoadByAssignee(e.Admin(), now)
	if err != nil {
		t.Fatalf("reading the team board's overdue load: %v", err)
	}
	boardCount := 0
	for _, row := range load {
		if row.OwnerID == e.AdminUser {
			boardCount = row.Overdue
		}
	}
	if boardCount != 2 {
		t.Errorf("the team board counts %d overdue, want the 2 the rep's own day shows", boardCount)
	}

	entity, kind := "contact", "task"
	onContact, _, err := e.Activities.ListActivities(e.Admin(), activities.ListActivitiesInput{
		EntityType: &entity, EntityID: &contactID, Kind: &kind,
	})
	if err != nil {
		t.Fatalf("listing the contact's tasks: %v", err)
	}
	if !slices.ContainsFunc(onContact, func(a crmcontracts.Activity) bool {
		return ids.UUID(a.Id).String() == agedOut && a.IsDone != nil && !*a.IsDone
	}) {
		t.Errorf("the aged-out task %s is gone from its contact, want it still open there", agedOut)
	}
}

func taskIDsOn(items []crmcontracts.AttentionItem) []string {
	var out []string
	for _, item := range items {
		if item.Source == "task" {
			out = append(out, item.Id)
		}
	}
	slices.Sort(out)
	return out
}

func sorted(values ...string) []string {
	slices.Sort(values)
	return values
}
