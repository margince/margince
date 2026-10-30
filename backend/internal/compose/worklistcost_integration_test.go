// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// worklistStages are the stages a worklist read times on a day with an open
// task. A stage that loses its timer fails this list, and its statements move
// to outside_stages, whose count is fixed below.
var worklistStages = []string{
	"this_morning", "needs_you", "planned", "relationship_decay", "subject_labels",
	"waiting_customers", "plan", "awaiting", "deal_moves",
	"meeting_horizon", "rank", "row_names",
}

// worklistOutsideStatements are the statements a worklist read sends outside
// every stage: BEGIN, COMMIT and the reads before the day is assembled.
const worklistOutsideStatements = 5

// stageDelta is what one composed read added to the stage collector.
type stageDelta struct {
	statements, clipped int64
	runs                map[attention.StageKey]uint64
	byKey               map[attention.StageKey]int64
}

// meterRead runs read under a fresh meter and answers what the collector and
// the meter gained.
func meterRead(ctx context.Context, t *testing.T, read func(context.Context) error) (stageDelta, database.StatementCost) {
	t.Helper()
	ctx, meter := database.WithStatementMeter(ctx)
	before := attention.StageCosts()
	if err := read(ctx); err != nil {
		t.Fatal(err)
	}
	after := attention.StageCosts()
	delta := stageDelta{runs: map[attention.StageKey]uint64{}, byKey: map[attention.StageKey]int64{}}
	for key, cost := range after {
		runs := cost.DB.Count() - before[key].DB.Count()
		if runs == 0 {
			continue
		}
		delta.runs[key] = runs
		delta.byKey[key] = cost.Statements - before[key].Statements
		delta.statements += delta.byKey[key]
		delta.clipped += cost.Clipped - before[key].Clipped
		t.Logf("%s %-28s %3d statements, %6.2f ms pgx", key.Read, key.Stage, delta.byKey[key],
			(cost.DB.Sum()-before[key].DB.Sum())*1000)
	}
	return delta, meter.Cost()
}

func seedOpenTask(t *testing.T, e *integration.Env) {
	t.Helper()
	pipeline, stage, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Renewal", pipeline, stage, nil)
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: new("Send the renewal terms"), Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal}},
	}); err != nil {
		t.Fatal(err)
	}
}

// The stages of one worklist read account for every statement the pool's
// tracer saw for it, and the read opens its snapshot once.
func TestAWorklistReadsStatementsAddUpToWhatThePoolSaw(t *testing.T) {
	e := integration.Setup(t)
	seedOpenTask(t, e)
	at := clocktest.Now(t).UTC()
	service := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return at })
	read := func(ctx context.Context) error {
		_, err := service.Worklist(ctx, "mine", "all", ids.UUID{}, 0, "")
		return err
	}
	// The first read fills the stores' cached workspace, which a running
	// server has long since done.
	if err := read(e.Admin()); err != nil {
		t.Fatal(err)
	}

	delta, seen := meterRead(e.Admin(), t, read)

	if seen.Statements == 0 || delta.statements != seen.Statements {
		t.Fatalf("the stages counted %d statements, the pool's tracer saw %d; every statement belongs to one stage",
			delta.statements, seen.Statements)
	}
	for key := range delta.runs {
		if key.Read != attention.ReadWorklist {
			t.Errorf("a worklist read charged %s/%s", key.Read, key.Stage)
		}
	}
	for _, stage := range worklistStages {
		if delta.runs[attention.StageKey{Read: attention.ReadWorklist, Stage: stage}] != 1 {
			t.Errorf("stage %s was not metered once by the read", stage)
		}
	}
	if delta.runs[attention.StageKey{Read: attention.ReadWorklist, Stage: "done_for_you"}] != 0 {
		t.Error("the read assembled the receipt lane, which is never a queue row")
	}
	outside := delta.byKey[attention.StageKey{Read: attention.ReadWorklist, Stage: attention.OutsideStages}]
	if outside != worklistOutsideStatements {
		t.Errorf("%d statements ran outside every stage, want %d; a stage that lost its timer moves its statements here",
			outside, worklistOutsideStatements)
	}
	// The snapshot, and the walk freeze, which commits on its own connection.
	if seen.Acquires != 2 || seen.PoolWait <= 0 {
		t.Errorf("the read acquired %d connections waiting %v, want the snapshot and the walk freeze", seen.Acquires, seen.PoolWait)
	}
	if delta.clipped != 0 {
		t.Errorf("%d stage runs took less time than their pgx calls; a read's calls run one after another", delta.clipped)
	}
}

// Team exceptions reads the assembled day inside its own snapshot. Its stages
// are charged to it once, never to a read nested in it.
func TestTeamExceptionsChargesTheDayItReadsToItselfOnce(t *testing.T) {
	e := integration.Setup(t)
	seedOpenTask(t, e)
	at := clocktest.Now(t).UTC()
	service := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return at })

	delta, seen := meterRead(e.Admin(), t, func(ctx context.Context) error {
		_, err := service.TeamExceptions(ctx)
		return err
	})

	if seen.Statements == 0 || delta.statements != seen.Statements {
		t.Fatalf("the stages counted %d statements, the pool's tracer saw %d", delta.statements, seen.Statements)
	}
	for key, runs := range delta.runs {
		if key.Read != attention.ReadTeamExceptions || runs != 1 {
			t.Errorf("%s/%s ran %d times, want each stage once under %s",
				key.Read, key.Stage, runs, attention.ReadTeamExceptions)
		}
	}
	t.Logf("team exceptions acquired %d pool connections", seen.Acquires)
}
