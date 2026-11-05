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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestHistoricalAutomaticTasksSupplyNeitherDealNextStepsNorHealthEvidence(t *testing.T) {
	for _, adopted := range []bool{false, true} {
		t.Run(map[bool]string{false: "automatic", true: "human adoption"}[adopted], func(t *testing.T) {
			e := integration.Setup(t)
			now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
			old := now.AddDate(-2, 0, 0)
			pipeline, stage, _ := integration.DealFixture(t, e)
			deal := e.SeedDeal(t, "Account expansion", pipeline, stage, &e.Rep1)
			source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "meeting", Source: "manual", OccurredAt: &old, MeetingStatus: new("held")})
			if err != nil {
				t.Fatal(err)
			}
			actor := principal.SystemActing(e.Admin(), "system:signal_scan")
			due := old
			if adopted {
				actor = e.Admin()
				due = now.Add(-time.Hour)
			}
			task, _, err := e.Activities.LogActivity(actor, activities.LogActivityInput{Kind: "task", Subject: new("Send the account plan"), Source: "manual", AssigneeID: new(ids.From[ids.UserKind](e.Rep1)), DueAt: &due, SourceSystem: new(activities.CommitmentTaskSource), SourceID: new(ids.NewV7().String()), SourceActivityID: new(ids.UUID(source.Id)), Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal}}})
			if err != nil {
				t.Fatal(err)
			}
			missing, err := dealsWithNoOpenNextStep(e.Admin(), e.Pool, []ids.UUID{deal}, now)
			if err != nil {
				t.Fatal(err)
			}
			if missing[deal] == adopted {
				t.Errorf("missing next step=%v, adopted=%v", missing[deal], adopted)
			}
			health, err := e.Deals.DealHealth(e.Admin(), ids.From[ids.DealKind](deal), now)
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if adopted {
				want = 1
			}
			if len(health.Evidence.OverdueTaskIDs) != want {
				t.Fatalf("health overdue tasks=%v, want %d", health.Evidence.OverdueTaskIDs, want)
			}
			if adopted && health.Evidence.OverdueTaskIDs[0] != ids.UUID(task.Id) {
				t.Fatalf("health names another task: %v", health.Evidence.OverdueTaskIDs)
			}
		})
	}
}

func TestTeamPromiseCountsRequireCurrentAssignedCommitmentsDueNow(t *testing.T) {
	e := integration.Setup(t)
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(-2, 0, 0)
	source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "meeting", Source: "manual", OccurredAt: &old, MeetingStatus: new("held")})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"due", "future", "undated", "historical", "ordinary"} {
		actor := e.Admin()
		due := new(now)
		system := new(activities.CommitmentTaskSource)
		switch state {
		case "future":
			due = new(now.AddDate(0, 0, 1))
		case "undated":
			due = nil
		case "historical":
			actor = principal.SystemActing(e.Admin(), "system:signal_scan")
			due = &old
		case "ordinary":
			system = nil
			due = new(now.Add(-time.Hour))
		}
		_, _, err := e.Activities.LogActivity(actor, activities.LogActivityInput{Kind: "task", Subject: new("Account plan " + state), Source: "manual", AssigneeID: new(ids.From[ids.UserKind](e.Rep1)), DueAt: due, SourceSystem: system, SourceID: new(ids.NewV7().String()), SourceActivityID: new(ids.UUID(source.Id))})
		if err != nil {
			t.Fatal(err)
		}
	}
	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return now })
	reader := e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms)
	board, err := feed.TeamBoard(reader)
	if err != nil {
		t.Fatal(err)
	}
	for _, member := range board.Members {
		if ids.UUID(member.UserId) == e.Rep1 {
			if member.Counts.PromisesDue != 1 || member.Counts.Overdue != 1 {
				t.Fatalf("assigned counts=%+v, want one due promise and one overdue task", member.Counts)
			}
			return
		}
	}
	t.Fatal("assigned teammate missing from board")
}
