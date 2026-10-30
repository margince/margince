// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package integration

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// dailyEdgeBatch bounds one fold transaction, so a full-tier corpus does not
// hold one transaction over half a million activities.
const dailyEdgeBatch = 5000

// dailyAICallsAtTier is a month of model calls for the bench's fourteen seats,
// enough for the spend report to price and group something real.
const dailyAICallsAtTier = 20_000

// dailyAITasks spreads the calls over the tasks the agent rail narrates.
var dailyAITasks = []ai.Task{ai.TaskSummarize, ai.TaskDraftReply, ai.TaskDocumentExtract, ai.TaskBriefRanking, ai.TaskAccountScan}

// interactionEdges folds the seeded mail into graph_interaction_edge through
// search's own fold, the projection the relationship-decay lane reads.
func (s dailySeeder) interactionEdges() {
	s.t.Helper()
	ctx := context.Background()
	rows, err := s.owner.Query(ctx, `SELECT id FROM daily_seed_activity WHERE kind = 'email' ORDER BY i`)
	if err != nil {
		s.t.Fatalf("seeding the daily corpus (interaction edges): %v", err)
	}
	activityIDs, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		s.t.Fatalf("seeding the daily corpus (interaction edges): %v", err)
	}
	for batch := range slices.Chunk(activityIDs, dailyEdgeBatch) {
		if err := s.foldEdges(ctx, batch); err != nil {
			s.t.Fatalf("seeding the daily corpus (interaction edges): %v", err)
		}
	}
	var edges int
	if err := s.owner.QueryRow(ctx, `SELECT count(*) FROM graph_interaction_edge`).Scan(&edges); err != nil {
		s.t.Fatalf("seeding the daily corpus (interaction edges): %v", err)
	}
	if edges == 0 {
		s.t.Fatalf("the fold of %d emails wrote no interaction edge, so the relationship-decay lane would read nothing", len(activityIDs))
	}
	s.t.Logf("seed: %d interaction edges", edges)
}

func (s dailySeeder) foldEdges(ctx context.Context, activityIDs []ids.UUID) (err error) {
	tx, err := s.owner.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			err = errors.Join(err, rollbackErr)
		}
	}()
	if err := search.RecomputeEdgesForActivities(ctx, tx, activityIDs); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// dailyAIModel is the model a tier's calls are recorded and priced under.
func dailyAIModel(tier ai.Tier) string { return "bench-" + string(tier) }

// seedDailyAICalls records the month's model calls through the meters the
// router itself writes with: one ai_call row and one ai_usage fold per call.
// Both models are priced first, through the admin's rate form, so the spend
// report sums a priced month. It returns how many calls it recorded.
func seedDailyAICalls(t *testing.T, e *apptest.AppEnv, scale float64) int {
	t.Helper()
	for _, tier := range []ai.Tier{ai.TierCheapCloud, ai.TierPremium} {
		mustCall(t, e, "POST", "/v1/ai-model-rates", AnyMap{
			"provider": "anthropic", "model_id": dailyAIModel(tier), "input_per_mtok": "3", "output_per_mtok": "15",
			"cache_read_per_mtok": "0.3", "cache_write_per_mtok": "3.75",
		}, http.StatusCreated, nil)
	}
	ctx := principal.WithWorkspaceID(context.Background(), apptest.InstallationWorkspaceUUID(context.Background(), t, e.Pool))
	calls, usage := ai.NewCallMeter(e.DB()), ai.NewMeter(e.DB())
	n := max(len(dailyAITasks), int(float64(dailyAICallsAtTier)*scale))
	for i := range n {
		task, tier := dailyAITasks[i%len(dailyAITasks)], ai.TierCheapCloud
		if i%7 == 0 {
			tier = ai.TierPremium
		}
		tokensIn, tokensOut := 800+i%1200, 150+i%400
		if err := calls.Record(ctx, []ai.Call{{
			LogicalCallID: ids.NewV7(), Attempt: 1, IsTerminal: true, Kind: "completion",
			Task: task, Tier: tier, Provider: "anthropic", ModelID: dailyAIModel(tier),
			ServedModel: dailyAIModel(tier), ServedIdentitySource: "response", RequestFingerprint: fmt.Sprintf("bench-%d", i),
			TokensIn: tokensIn, TokensOut: tokensOut,
		}}); err != nil {
			t.Fatalf("recording AI call %d: %v", i, err)
		}
		if err := usage.Record(ctx, ai.Usage{Task: task, Tier: tier, TokensIn: tokensIn, TokensOut: tokensOut}); err != nil {
			t.Fatalf("metering AI call %d: %v", i, err)
		}
	}
	t.Logf("seed: %d AI calls recorded", n)
	return n
}

// makeDailyBriefs assembles today's brief for every seat, the admin included,
// through the endpoint a rep's first morning visit takes. The morning load
// sends the admin through Home too.
func makeDailyBriefs(t *testing.T, e *apptest.AppEnv, seats Seats) {
	t.Helper()
	admin := Seat{Name: "admin", Role: "admin", Client: e.Client}
	for _, seat := range slices.Concat(seats.Managers, seats.Reps, []Seat{admin}) {
		var run struct {
			Items []struct{} `json:"items"`
		}
		mustCall(t, seat.env(e), "POST", "/v1/brief", nil, http.StatusCreated, &run)
		t.Logf("brief: %s holds %d items", seat.Name, len(run.Items))
	}
}

// worklistStageMark is the stage collector at one moment, which
// logWorklistStages later subtracts.
type worklistStageMark struct {
	durations map[attention.StageKey]httpserver.Histogram
	costs     map[attention.StageKey]attention.StageCost
}

func worklistStagesSoFar() worklistStageMark {
	return worklistStageMark{durations: attention.StageTimings(), costs: attention.StageCosts()}
}

// logWorklistStages prints each stage's mean over one seat's latency runs, by
// read and slowest first. Beside it go its statements, pgx time, pool wait and the
// rest per run. The collector is process-wide, so the reads before the step are
// subtracted.
func logWorklistStages(t *testing.T, seat string, before worklistStageMark) {
	t.Helper()
	now := worklistStagesSoFar()
	// outside_stages has costs and no duration, so its runs come from the costs.
	runs := func(key attention.StageKey) uint64 {
		return max(now.durations[key].Count(), now.costs[key].DB.Count()) -
			max(before.durations[key].Count(), before.costs[key].DB.Count())
	}
	perRun := func(key attention.StageKey, sum func(worklistStageMark) float64) float64 {
		return (sum(now) - sum(before)) / float64(max(1, runs(key)))
	}
	keys := slices.Collect(maps.Keys(now.costs))
	for key := range maps.Keys(now.durations) {
		if !slices.Contains(keys, key) {
			keys = append(keys, key)
		}
	}
	mean := func(key attention.StageKey) float64 {
		return perRun(key, func(m worklistStageMark) float64 { return m.durations[key].Sum() })
	}
	slices.SortFunc(keys, func(a, b attention.StageKey) int {
		return cmp.Or(cmp.Compare(a.Read, b.Read), cmp.Compare(mean(b), mean(a)))
	})
	for _, key := range keys {
		cost := func(read func(attention.StageCost) float64) float64 {
			return perRun(key, func(m worklistStageMark) float64 { return read(m.costs[key]) })
		}
		t.Logf("%-8s %-15s stage %-28s mean %8.1f ms over %d | %5.1f statements %8.1f ms pgx %6.1f ms pool %8.1f ms rest per run",
			seat, key.Read, key.Stage, mean(key)*1000, runs(key),
			cost(func(c attention.StageCost) float64 { return float64(c.Statements) }),
			cost(func(c attention.StageCost) float64 { return c.DB.Sum() })*1000,
			cost(func(c attention.StageCost) float64 { return c.PoolWait.Sum() })*1000,
			cost(func(c attention.StageCost) float64 { return c.App.Sum() })*1000)
	}
}
