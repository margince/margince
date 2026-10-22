// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package weekly

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestWeeklyCountsExcludeUnadoptedHistoricalAutomaticWork(t *testing.T) {
	e := integration.Setup(t)
	old := time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC)
	source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "meeting", Subject: new("Previous account review"), OccurredAt: &old, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	pipeline, stage, _ := integration.DealFixture(t, e)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{Name: "Account plan", PipelineID: pipeline, StageID: stage, Source: "manual", OwnerID: new(ids.From[ids.UserKind](e.Rep1))})
	if err != nil {
		t.Fatal(err)
	}
	assignee := ids.From[ids.UserKind](e.Rep1)
	task, _, err := e.Activities.LogActivity(principal.SystemActing(e.Admin(), "system:signal_scan"), activities.LogActivityInput{Kind: "task", Subject: new("Send the previous account plan"), Source: "manual", AssigneeID: &assignee, DueAt: &old, SourceSystem: new(activities.CommitmentTaskSource), SourceID: new(ids.NewV7().String()), SourceActivityID: new(ids.UUID(source.Id)), Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: ids.UUID(deal.Id)}}})
	if err != nil {
		t.Fatal(err)
	}
	end := task.CreatedAt.Add(time.Hour)
	start := end.AddDate(0, 0, -7)
	check := func(want int) {
		t.Helper()
		err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
			counts, err := countWeek(e.Admin(), tx, e.Rep1, start, end)
			if err != nil {
				return err
			}
			if counts.TasksCarriedOver != want {
				t.Errorf("carried over=%d, want %d", counts.TasksCarriedOver, want)
			}
			block, err := scoreDeals(e.Admin(), tx, e.Rep1, start, end)
			if err != nil {
				return err
			}
			if block == nil || block.WithNextStep != want {
				t.Errorf("deal next-step count=%+v, want %d", block, want)
			}
			starts, err := readFigureStarts(e.Admin(), tx, []ids.UUID{e.Rep1}, end)
			if err != nil {
				return err
			}
			if (starts.tasks != nil) != (want > 0) {
				t.Errorf("task coverage start=%v for carried=%d", starts.tasks, want)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	check(0)
	if _, err := e.Activities.UpdateActivity(e.Admin(), ids.From[ids.ActivityKind](ids.UUID(task.Id)), activities.UpdateActivityInput{Subject: new("I will send the revised plan")}); err != nil {
		t.Fatal(err)
	}
	check(1)
}
