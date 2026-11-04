// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// An open customer-support review is one Home row for the deal's owner. Its
// task does not also appear, it stays after the task is done, and a colleague's
// own page does not carry it.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// homeRows reads every page of the worklist for the scope, as ctx.
func (v *viabilityEnv) homeRows(ctx context.Context, t *testing.T, scope string) []crmcontracts.WorklistItem {
	t.Helper()
	svc := newAttentionService(v.Pool, approvals.NewService(v.DB()), func() time.Time { return v.now })
	var out []crmcontracts.WorklistItem
	for cursor := ""; ; {
		page, err := svc.Worklist(ctx, scope, "", ids.Nil, 100, cursor)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, page.Queue...)
		if page.NextCursor == nil {
			return out
		}
		cursor = *page.NextCursor
	}
}

// rowsAbout counts the rows about the review: the review row itself, its task
// and any at-risk row of its deal.
func rowsAbout(rows []crmcontracts.WorklistItem, review, task, deal string) (reviews, tasks, risks int) {
	for _, row := range rows {
		switch {
		case row.Source == crmcontracts.WorklistItemSourceDealReview && row.Id == review:
			reviews++
		case row.Source == crmcontracts.WorklistItemSourceTask && row.Id == task:
			tasks++
		case row.Source == crmcontracts.WorklistItemSourceDealAtRisk && row.Id == deal:
			risks++
		}
	}
	return reviews, tasks, risks
}

func TestOneHomeActionPerReview(t *testing.T) {
	v := setupViability(t)
	v.check(t)
	review := v.WsScalar(t, `SELECT id::text FROM assurance_exception WHERE type = $1 AND subject_id = $2`,
		assurance.TypeDealViability, v.silent)
	task := v.WsScalar(t, `SELECT task_activity_id::text FROM assurance_exception WHERE id = $1`, review)
	owner := v.As(v.Rep1, []ids.UUID{v.Team1}, integration.AdminPerms)

	reviews, tasks, risks := rowsAbout(v.homeRows(owner, t, "mine"), review, task, v.silent.String())
	if reviews != 1 || tasks != 0 || risks != 0 {
		t.Fatalf("the owner's Home holds %d review, %d task and %d at-risk rows about the review, want one review row",
			reviews, tasks, risks)
	}

	taskID, err := ids.Parse(task)
	if err != nil {
		t.Fatal(err)
	}
	done := true
	if _, err := v.Activities.UpdateActivity(v.Admin(), ids.From[ids.ActivityKind](taskID),
		activities.UpdateActivityInput{IsDone: &done}); err != nil {
		t.Fatal(err)
	}
	if reviews, _, _ := rowsAbout(v.homeRows(owner, t, "mine"), review, task, v.silent.String()); reviews != 1 {
		t.Errorf("after the task was done the owner's Home holds %d review rows, want the open review still there", reviews)
	}

	colleague := v.As(v.Rep3, []ids.UUID{v.Team2}, integration.AdminPerms)
	if reviews, _, _ := rowsAbout(v.homeRows(colleague, t, "mine"), review, task, v.silent.String()); reviews != 0 {
		t.Errorf("a colleague's own Home carries %d rows of Rep1's review, want none", reviews)
	}
}

// The review's task reassigned to a colleague moves the review to the
// colleague's Home alone, while the task is open and once it is done.
func TestAReassignedReviewTaskPutsTheReviewOnOneHome(t *testing.T) {
	v := setupViability(t)
	v.check(t)
	review := v.WsScalar(t, `SELECT id::text FROM assurance_exception WHERE type = $1 AND subject_id = $2`,
		assurance.TypeDealViability, v.silent)
	task := v.WsScalar(t, `SELECT task_activity_id::text FROM assurance_exception WHERE id = $1`, review)
	taskID, err := ids.Parse(task)
	if err != nil {
		t.Fatal(err)
	}
	colleague := ids.From[ids.UserKind](v.Rep2)
	if _, err := v.Activities.UpdateActivity(v.Admin(), ids.From[ids.ActivityKind](taskID),
		activities.UpdateActivityInput{AssigneeID: &colleague}); err != nil {
		t.Fatalf("reassigning the review task: %v", err)
	}
	owner := v.As(v.Rep1, []ids.UUID{v.Team1}, integration.AdminPerms)
	assignee := v.As(v.Rep2, []ids.UUID{v.Team1}, integration.AdminPerms)
	homes := func(when string) {
		t.Helper()
		ownerRows, _, _ := rowsAbout(v.homeRows(owner, t, "mine"), review, task, v.silent.String())
		assigneeRows, _, _ := rowsAbout(v.homeRows(assignee, t, "mine"), review, task, v.silent.String())
		if ownerRows != 0 || assigneeRows != 1 {
			t.Errorf("%s: the deal owner's Home holds %d review rows and the assignee's %d, want 0 and 1",
				when, ownerRows, assigneeRows)
		}
	}
	homes("with the task open")
	done := true
	if _, err := v.Activities.UpdateActivity(v.Admin(), ids.From[ids.ActivityKind](taskID),
		activities.UpdateActivityInput{IsDone: &done}); err != nil {
		t.Fatal(err)
	}
	homes("with the task done")
}
