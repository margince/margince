// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestReassignedTasksLeavePersonalWorkEvenOnAResumedWalk(t *testing.T) {
	e := integration.Setup(t)
	now := clocktest.Now(t)
	due := now.Add(-time.Hour)
	mine := ids.From[ids.UserKind](e.AdminUser)
	colleague := ids.From[ids.UserKind](e.Rep1)
	tasks := make([]ids.ActivityID, 0, 2)
	for _, title := range []string{"Send the quote", "Confirm the workshop"} {
		contact := e.SeedContact(t, title, &e.AdminUser)
		row, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
			Kind: "task", Subject: &title, DueAt: &due, Source: "manual", AssigneeID: &mine,
			Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
		})
		if err != nil {
			t.Fatal(err)
		}
		tasks = append(tasks, ids.From[ids.ActivityKind](ids.UUID(row.Id)))
	}
	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return now })
	first, err := feed.Worklist(e.Admin(), "mine", "all", ids.Nil, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if first.NextCursor == nil || len(first.Queue) != 1 || first.Summary.Total != 2 {
		t.Fatalf("expected a two-task frozen walk: %+v", first)
	}
	for _, task := range tasks {
		if _, err := e.Activities.UpdateActivity(e.Admin(), task, activities.UpdateActivityInput{AssigneeID: &colleague}); err != nil {
			t.Fatal(err)
		}
	}
	for _, cursor := range []string{"", *first.NextCursor} {
		page, err := feed.Worklist(e.Admin(), "mine", "all", ids.Nil, 1, cursor)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Queue) != 0 || (page.Focus != nil && len(page.Focus.Items) != 0) {
			t.Fatalf("former assignee still has another seat's work: %+v", page)
		}
	}
	recipient := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	page, err := feed.Worklist(recipient, "mine", "all", ids.Nil, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Queue) != 2 || page.Summary.Total != 2 {
		t.Fatalf("new assignee lost handed-over work: %+v", page)
	}
	for _, row := range page.Queue {
		if row.Owner == nil || row.Owner.Id == nil || ids.UUID(*row.Owner.Id) != e.Rep1 {
			t.Fatalf("wrong task owner: %+v", row)
		}
	}
}
