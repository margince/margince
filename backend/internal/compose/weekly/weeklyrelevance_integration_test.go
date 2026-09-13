// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package weekly

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestACompletionAtTheClosingBoundaryRemainsCarriedOver(t *testing.T) {
	e := setupWeekly(t)
	due := weekClock.AddDate(0, 0, -7)
	assignee := ids.From[ids.UserKind](e.Rep1)
	task, _, err := e.Activities.LogActivity(e.repCtx, activities.LogActivityInput{
		Kind: "task", Source: "manual", DueAt: &due, AssigneeID: &assignee,
	})
	if err != nil {
		t.Fatal(err)
	}
	done := true
	finished, err := e.Activities.UpdateActivity(e.repCtx, ids.From[ids.ActivityKind](ids.UUID(task.Id)), activities.UpdateActivityInput{IsDone: &done})
	if err != nil {
		t.Fatal(err)
	}
	if finished.DoneAt == nil {
		t.Fatal("completion has no timestamp")
	}
	end := *finished.DoneAt
	// The real writer stamps creation and completion. Dating only the due date
	// exercises the cutoff without guessing how fast the machine executes.
	start := due.Add(-time.Hour)
	err = database.WithWorkspaceTx(e.repCtx, e.Pool, func(tx pgx.Tx) error {
		counts, err := countWeek(e.repCtx, tx, e.Rep1, start, end)
		if err != nil {
			return err
		}
		if counts.TasksDue != 1 || counts.TasksDone != 0 || counts.TasksCarriedOver != 1 {
			t.Errorf("at closing boundary: %+v", counts)
		}
		counts, err = countWeek(e.repCtx, tx, e.Rep1, start, end.Add(time.Microsecond))
		if err != nil {
			return err
		}
		if counts.TasksDone != 1 || counts.TasksCarriedOver != 0 {
			t.Errorf("completed before closing: %+v", counts)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAccountCoincidenceDoesNotProveAMeetingFollowUp(t *testing.T) {
	e := setupWeekly(t)
	assignee := ids.From[ids.UserKind](e.Rep1)
	contact, err := e.Contacts.CreateContact(e.repCtx, contacts.CreateContactInput{FullName: "Shared contact", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	links := []activities.ActivityLinkInput{{EntityType: "contact", EntityID: ids.UUID(contact.Id)}}
	held := "held"
	meeting, _, err := e.Activities.LogActivity(e.repCtx, activities.LogActivityInput{
		Kind: "meeting", Source: "manual", MeetingStatus: &held, HostUserID: &assignee, Links: links,
	})
	if err != nil {
		t.Fatal(err)
	}
	source := ids.UUID(meeting.Id)
	unrelated, _, err := e.Activities.LogActivity(e.repCtx, activities.LogActivityInput{Kind: "task", Source: "manual", AssigneeID: &assignee, Links: links})
	if err != nil {
		t.Fatal(err)
	}
	read := func(end time.Time, want int) {
		t.Helper()
		err := database.WithWorkspaceTx(e.repCtx, e.Pool, func(tx pgx.Tx) error {
			count, linked, err := countWeekMeetings(e.repCtx, tx, e.Rep1, meeting.OccurredAt.Add(-time.Hour), end)
			if err == nil && (count != 1 || linked != want) {
				t.Errorf("held=%d linked=%d, want 1 and %d", count, linked, want)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	read(unrelated.CreatedAt.Add(time.Second), 0)
	linked, _, err := e.Activities.LogActivity(e.repCtx, activities.LogActivityInput{Kind: "task", Source: "manual", AssigneeID: &assignee, SourceActivityID: &source})
	if err != nil {
		t.Fatal(err)
	}
	read(linked.CreatedAt, 0)
	read(linked.CreatedAt.Add(time.Second), 1)
}

func TestAnUnmeasuredTeamRecoversButAMeasuredWeekStaysFrozen(t *testing.T) {
	e := setupWeekly(t)
	members := []TeamMember{{UserID: e.Rep1, DisplayName: "One"}, {UserID: e.Rep2, DisplayName: "Two"}}
	empty, _, err := e.engine.AssembleTeamFor(e.repCtx, e.Team1, "Team", members, weekClock)
	if err != nil {
		t.Fatal(err)
	}
	if empty.Counts.RepsCounted != 0 || empty.RepsUnread != 2 {
		t.Fatalf("not an unmeasured fixture: %+v", empty)
	}
	for _, member := range members {
		if _, _, err := e.engine.AssembleFor(e.As(member.UserID, nil, integration.AdminPerms), weekClock); err != nil {
			t.Fatal(err)
		}
	}
	recovered, wrote, err := e.engine.AssembleTeamFor(e.repCtx, e.Team1, "Team", members, weekClock)
	if err != nil {
		t.Fatal(err)
	}
	if !wrote || recovered.ID != empty.ID || recovered.Counts.RepsCounted != 2 || recovered.RepsUnread != 0 {
		t.Fatalf("recovery failed: wrote=%v review=%+v", wrote, recovered)
	}
	frozen, wrote, err := e.engine.AssembleTeamFor(e.repCtx, e.Team1, "Renamed", members[:1], weekClock)
	if err != nil {
		t.Fatal(err)
	}
	if wrote || frozen.Counts.RepsCounted != 2 || frozen.TeamName != "Team" {
		t.Fatalf("measured week changed: %+v", frozen)
	}
	var audits int
	if err := integration.OwnerConn(t).QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE entity_type='team_weekly_review' AND entity_id=$1`, empty.ID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 2 {
		t.Fatalf("create and recovery need audit rows, got %d", audits)
	}
}

func TestTeamMoneyIsUnknownWhenMemberCurrenciesDiffer(t *testing.T) {
	e := setupWeekly(t)
	for i, user := range []ids.UUID{e.Rep1, e.Rep2} {
		currency := []string{"EUR", "USD"}[i]
		e.WsExec(t, `INSERT INTO setting(key,value) VALUES('installation.base_currency',to_jsonb($1::text)) ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value`, currency)
		if _, _, err := e.engine.AssembleFor(e.As(user, nil, integration.AdminPerms), weekClock); err != nil {
			t.Fatal(err)
		}
	}
	review, _, err := e.engine.AssembleTeamFor(e.repCtx, e.Team1, "Team", []TeamMember{{UserID: e.Rep1, DisplayName: "One"}, {UserID: e.Rep2, DisplayName: "Two"}}, weekClock)
	if err != nil {
		t.Fatal(err)
	}
	if review.Money.Known {
		t.Fatalf("mixed currency total: %+v", review.Money)
	}
}

func TestUndatedWorkCountsAsCompletedWithoutInventingADueDate(t *testing.T) {
	e := setupWeekly(t)
	task, _, err := e.Activities.LogActivity(e.repCtx, activities.LogActivityInput{Kind: "task", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	done := true
	finished, err := e.Activities.UpdateActivity(e.repCtx, ids.From[ids.ActivityKind](ids.UUID(task.Id)), activities.UpdateActivityInput{IsDone: &done})
	if err != nil {
		t.Fatal(err)
	}
	if finished.DoneAt == nil {
		t.Fatal("completion missing timestamp")
	}
	err = database.WithWorkspaceTx(e.repCtx, e.Pool, func(tx pgx.Tx) error {
		counts, err := countWeek(e.repCtx, tx, e.Rep1, task.CreatedAt.Add(-time.Hour), finished.DoneAt.Add(time.Second))
		if err != nil {
			return err
		}
		if counts.TasksCompleted == nil || *counts.TasksCompleted != 1 || counts.TasksDone != 0 || counts.TasksDue != 0 {
			t.Fatalf("undated completed work: %+v", counts)
		}
		review := Review{UserID: e.Rep1, LocalWeekStart: weekClock, AsOf: weekClock, Counts: counts}
		id, wrote, err := insertReview(e.repCtx, tx, review)
		if err != nil {
			return err
		}
		if !wrote {
			t.Fatal("fixture review already existed")
		}
		read, err := readReviewTx(e.repCtx, tx, id, e.Rep1)
		if err != nil {
			return err
		}
		wire := countsToWire(read.Counts)
		if wire.TasksCompleted == nil || *wire.TasksCompleted != 1 {
			t.Fatalf("completed work lost on the wire: %+v", wire)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestReopeningAfterTheCutoffDoesNotEraseCompletedWork(t *testing.T) {
	e := setupWeekly(t)
	task, _, err := e.Activities.LogActivity(e.repCtx, activities.LogActivityInput{Kind: "task", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.ActivityKind](ids.UUID(task.Id))
	done := true
	if _, err := e.Activities.UpdateActivity(e.repCtx, id, activities.UpdateActivityInput{IsDone: &done}); err != nil {
		t.Fatal(err)
	}
	done = false
	reopened, err := e.Activities.UpdateActivity(e.repCtx, id, activities.UpdateActivityInput{IsDone: &done})
	if err != nil {
		t.Fatal(err)
	}
	err = database.WithWorkspaceTx(e.repCtx, e.Pool, func(tx pgx.Tx) error {
		counts, err := countWeek(e.repCtx, tx, e.Rep1, task.CreatedAt.Add(-time.Hour), reopened.UpdatedAt)
		if err != nil {
			return err
		}
		if counts.TasksCompleted == nil || *counts.TasksCompleted != 1 {
			t.Fatalf("reopening rewrote the closed period: %+v", counts)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPersonalAuthorityCannotAssembleATeamReport(t *testing.T) {
	e := setupWeekly(t)
	perms := integration.AdminPerms
	perms.RowScope = principal.RowScopeOwn
	_, _, err := e.engine.AssembleTeamFor(e.As(e.Rep1, []ids.UUID{e.Team1}, perms), e.Team1, "Team", nil, weekClock)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("personal authority returned %v", err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM team_weekly_review`); got != 0 {
		t.Fatal("refusal left a team snapshot")
	}
}
