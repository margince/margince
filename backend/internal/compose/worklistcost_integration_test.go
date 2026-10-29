// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The stages of one worklist read account for every statement the pool's
// tracer saw for it. So the stage counters on /metrics add up to the read.
func TestAWorklistReadsStatementsAddUpToWhatThePoolSaw(t *testing.T) {
	e := integration.Setup(t)
	pipeline, stage, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Renewal", pipeline, stage, nil)
	subject := "Send the renewal terms"
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal}},
	}); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC()
	service := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return at })
	ctx, meter := database.WithStatementMeter(e.Admin())
	before, waitsBefore := attention.StageCosts(), attention.PoolWaits()

	if _, err := service.Worklist(ctx, "mine", "all", ids.UUID{}, 0, ""); err != nil {
		t.Fatalf("worklist: %v", err)
	}
	after, waitsAfter := attention.StageCosts(), attention.PoolWaits()
	timed := attention.StageTimings()
	var staged int64
	for _, name := range slices.Sorted(maps.Keys(after)) {
		if _, ok := timed[name]; !ok && name != attention.OutsideStages {
			t.Errorf("cost label %q names no timed stage", name)
		}
		statements := after[name].Statements - before[name].Statements
		staged += statements
		t.Logf("stage %-28s %3d statements, %6.2f ms database", name, statements,
			(after[name].DB.Sum()-before[name].DB.Sum())*1000)
	}
	seen := meter.Cost()
	if seen.Statements == 0 || staged != seen.Statements {
		t.Fatalf("the stages counted %d statements, the pool's tracer saw %d; every statement belongs to one stage",
			staged, seen.Statements)
	}
	for _, name := range []string{attention.OutsideStages, "needs_you", "planned", "rank"} {
		if after[name].Statements <= before[name].Statements {
			t.Errorf("stage %s ran no statement on a day with an open task", name)
		}
	}
	if waitsAfter.Count() != waitsBefore.Count()+1 || seen.PoolWait <= 0 {
		t.Errorf("the read recorded %d pool waits of %v, want one measured wait",
			waitsAfter.Count()-waitsBefore.Count(), seen.PoolWait)
	}
}
