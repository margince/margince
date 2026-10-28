// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package integration

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/search"
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

// seedDailyAICalls records the month's model calls through the meters the
// router itself writes with: one ai_call row and one ai_usage fold per call.
func seedDailyAICalls(t *testing.T, e *apptest.AppEnv, scale float64) {
	t.Helper()
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
			Task: task, Tier: tier, Provider: "anthropic", ModelID: "bench-" + string(tier),
			ServedIdentitySource: "response", RequestFingerprint: fmt.Sprintf("bench-%d", i),
			TokensIn: tokensIn, TokensOut: tokensOut,
		}}); err != nil {
			t.Fatalf("recording AI call %d: %v", i, err)
		}
		if err := usage.Record(ctx, ai.Usage{Task: task, Tier: tier, TokensIn: tokensIn, TokensOut: tokensOut}); err != nil {
			t.Fatalf("metering AI call %d: %v", i, err)
		}
	}
	t.Logf("seed: %d AI calls recorded", n)
}

// makeDailyBriefs assembles today's brief for every seat through the endpoint
// a rep's first morning visit takes, so Home reads a brief with items.
func makeDailyBriefs(t *testing.T, e *apptest.AppEnv, seats Seats) {
	t.Helper()
	for _, seat := range slices.Concat(seats.Managers, seats.Reps) {
		var run struct {
			Items []struct{} `json:"items"`
		}
		mustCall(t, seat.env(e), "POST", "/v1/brief", nil, http.StatusCreated, &run)
		t.Logf("brief: %s holds %d items", seat.Name, len(run.Items))
	}
}
