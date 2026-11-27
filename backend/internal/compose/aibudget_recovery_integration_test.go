// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func TestAllowanceRecoveryAdvancesBothWebsiteClocksAndIsIdempotent(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	grantBudgetRequester(t, e, e.Rep1)
	company := insertCompany(t, e, e.Rep1, "acme.example", "")
	read, args := startDeepRead(t, e, company)
	next := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	worker, _ := newDeepReadTestWorker(e, acmeDeepSite(), budgetDeferringBrain{next: next})
	if err := worker.run(context.Background(), args); err == nil {
		t.Fatal("expected budget deferral")
	}
	runner, err := jobs.NewInserter(e.Pool, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Enqueue(e.Admin(), args, &river.InsertOpts{ScheduledAt: next, MaxAttempts: 3}); err != nil {
		t.Fatal(err)
	}
	recovery := newAIBudgetResumeWorker(e.Pool, slog.New(slog.DiscardHandler))
	setRecoveryAllowance(t, e, 1)
	if err := ai.NewMeter(e.DB()).Record(e.Admin(), ai.Usage{Task: ai.TaskSiteExtract, Tier: ai.TierLocalSmall, TokensIn: 2}); err != nil {
		t.Fatal(err)
	}
	if err := recovery.resumeWorkspace(context.Background(), e.WS); err != nil {
		t.Fatal(err)
	}
	blocked, err := e.Contacts.GetSiteRead(e.As(e.Rep1, nil, integration.AdminPerms), companyIDOf(company), read.ID)
	if err != nil {
		t.Fatal(err)
	}
	if blocked.NextAttemptAt == nil || !blocked.NextAttemptAt.Equal(next) {
		t.Fatal("exhausted allowance resumed work")
	}
	setRecoveryAllowance(t, e, 1000)

	for range 2 {
		if err := recovery.resumeWorkspace(context.Background(), e.WS); err != nil {
			t.Fatal(err)
		}
	}
	current, err := e.Contacts.GetSiteRead(e.As(e.Rep1, nil, integration.AdminPerms), companyIDOf(company), read.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.NextAttemptAt == nil || !current.NextAttemptAt.Before(next) || current.RequestedBy != read.RequestedBy {
		t.Fatalf("carrier %+v", current)
	}
	err = e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		var count int
		var maxAttempts int
		var scheduled time.Time
		err := tx.QueryRow(e.Admin(), `SELECT count(*),max(max_attempts),max(scheduled_at) FROM river_job WHERE kind='site_deep_read' AND args->>'site_read_id'=$1`, read.ID.String()).Scan(&count, &maxAttempts, &scheduled)
		if err == nil && (count != 1 || maxAttempts != 3 || !scheduled.Before(next)) {
			t.Errorf("job: count=%d max=%d at=%s", count, maxAttempts, scheduled)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryPagesContainFailuresAndReachLaterRows(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	ctx := e.Admin()
	idsToVisit := make([]ids.UUID, 205)
	for i := range idsToVisit {
		idsToVisit[i] = ids.NewV7()
	}
	failed := errors.New("one row cannot resume")
	visited := map[ids.UUID]bool{}
	pages := 0
	err := storekit.RecoverPages(ctx, e.DB().Tx, func(_ pgx.Tx, after ids.UUID) ([]ids.UUID, error) {
		pages++
		start := 0
		for start < len(idsToVisit) && idsToVisit[start].String() <= after.String() {
			start++
		}
		return idsToVisit[start:min(start+100, len(idsToVisit))], nil
	}, func(id ids.UUID) ids.UUID { return id }, func(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
		if id == idsToVisit[0] {
			return failed
		}
		// Real SQL after a failed savepoint proves the transaction remains usable.
		var one int
		if err := tx.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
			return err
		}
		if one != 1 {
			t.Fatal("unexpected database result")
		}
		visited[id] = true
		return nil
	})
	if !errors.Is(err, failed) || len(visited) != 204 || pages != 3 {
		t.Fatalf("err=%v visited=%d pages=%d", err, len(visited), pages)
	}
}

func grantBudgetRequester(t *testing.T, e *integration.Env, user ids.UUID) {
	t.Helper()
	// The module harness seeds seats without roles. This migration fixture is
	// held equal to the bootstrap writer by the seeded-RBAC gate; assignment
	// still goes through the real identity writer and recovery resolves it live.
	raw, err := os.ReadFile("../../migrations/testdata/rbac_seeded_defaults.json")
	if err != nil {
		t.Fatal(err)
	}
	var roles map[string]json.RawMessage
	if err := json.Unmarshal(raw, &roles); err != nil {
		t.Fatal(err)
	}
	owner := integration.OwnerConn(t)
	for key, permissions := range roles {
		args := []any{ids.NewV7(), key, permissions}
		query := fmt.Sprintf(`INSERT INTO role(id,key,name,is_system,permissions) VALUES($%d,$%d,$%d,true,$%d) ON CONFLICT(key) DO NOTHING`, len(args)-2, len(args)-1, len(args)-1, len(args))
		if _, err := owner.Exec(t.Context(), query, args...); err != nil {
			t.Fatal(err)
		}
	}
	service := identity.NewService(e.Pool)
	actor := recoveryAdminIdentity(e)
	if err := service.ChangeUserRole(e.Admin(), actor, actor.UserID, "admin"); err != nil {
		t.Fatal(err)
	}
	if err := service.ChangeUserRole(e.Admin(), actor, ids.From[ids.UserKind](user), "rep"); err != nil {
		t.Fatal(err)
	}
}

func recoveryAdminIdentity(e *integration.Env) identity.Identity {
	return identity.Identity{UserID: ids.From[ids.UserKind](e.AdminUser), WorkspaceID: ids.From[ids.WorkspaceKind](e.WS), SeatType: "full", Roles: []string{"admin"}, Permissions: integration.AdminPerms}
}

func TestAllowanceRecoveryRequeuesVoiceBuildWithoutResettingItsAttempt(t *testing.T) {
	t.Parallel()
	env, build := seedVoiceBuild(t, "A recovery quote.", 6)
	grantBudgetRequester(t, env.e, env.e.Rep1)
	ctx := env.workerCtx(t)
	input, claimed, err := env.store.ClaimBuild(ctx, env.profile.ID, build.ID, time.Minute)
	if err != nil || !claimed {
		t.Fatalf("claim: %v %v", claimed, err)
	}
	next := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := env.store.DeferBuild(ctx, build.ID, *input.Build.StartedAt, "allowance", next); err != nil {
		t.Fatal(err)
	}
	recovery := newAIBudgetResumeWorker(env.e.Pool, slog.New(slog.DiscardHandler))
	for range 2 {
		if err := recovery.resumeWorkspace(env.e.Admin(), env.e.WS); err != nil {
			t.Fatal(err)
		}
	}
	current, err := env.store.GetBuild(env.owner, env.profile.ID, build.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.NextAttemptAt == nil || !current.NextAttemptAt.Before(next) || current.Attempt != input.Build.Attempt {
		t.Fatalf("recovery changed claim: %+v", current)
	}
	if err := env.e.DB().Tx(env.e.Admin(), func(tx pgx.Tx) error {
		args := []any{build.ID.String()}
		var count int
		query := fmt.Sprintf(`SELECT count(*) FROM river_job WHERE kind='voice_build' AND args->>'build_id'=$%d`, len(args))
		err := tx.QueryRow(ctx, query, args...).Scan(&count)
		if err == nil && count != 1 {
			t.Errorf("queued %d jobs for one build", count)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func setRecoveryAllowance(t *testing.T, e *integration.Env, tokens int64) {
	t.Helper()
	store := ai.NewAdminStore(e.DB(), NewSettingsStore(e.Pool), StoredConfig(e.Pool), budgetFullUsers, aiDeferredWork(e.Pool))
	current, err := store.ReadBudget(e.Admin())
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.ReplaceBudget(e.Admin(), ai.BudgetChange{ExpectedRevision: current.Revision, Config: ai.BudgetConfig{TokensPerFullUser: current.Config.TokensPerFullUser, CompanyMonthlyTokens: &tokens}})
	if err != nil {
		t.Fatal(err)
	}
}

// A step the router declines reaches the runner as the runner's own refusal,
// so the run says why no provider was called. A call that was made is not one.
func TestAnAgentStepTheRouterDeclinedReachesTheRunnerAsItsReason(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	ctx := principal.WithWorkspaceID(t.Context(), e.WS)
	meter := ai.NewMeter(InstallationDB(e.Pool))
	if err := meter.Record(ctx, ai.Usage{Task: ai.TaskAgentLoop, Tier: ai.TierCheapCloud, TokensIn: 10}); err != nil {
		t.Fatalf("spending the allowance: %v", err)
	}
	cfg, err := ai.ParseRouting([]byte("profile: eu_hosted\ntiers:\n  cheap_cloud: {provider: fake, model: m}\nembeddings: {provider: fake, model: e, dimensions: 8}\n"))
	router, routerErr := ai.NewRouter(cfg, meter, ai.StaticBudget(5), nil, false, nil)
	if err != nil || routerErr != nil {
		t.Fatalf("binding the router: %v, %v", err, routerErr)
	}
	brain := agentBrain{router: router, companyContext: newCompanyContextProvider(nil)}
	_, _, err = brain.Complete(ctx, model.Request{Messages: []model.Message{{Role: "user", Content: "go"}}})
	if !errors.Is(err, runner.ErrAllowanceUsedUp) || !errors.Is(err, ai.ErrBudgetDeferred) {
		t.Errorf("budget stop = %v, want the runner's allowance refusal over the router's deferral", err)
	}
	if err := agentStepRefusal(&ai.ProviderDownError{Provider: "p", Health: model.HealthDown}); !errors.Is(err, runner.ErrProviderBlocked) {
		t.Errorf("blocked provider = %v, want the runner's blocked-provider refusal", err)
	}
	if err := agentStepRefusal(&ai.ProviderDownError{Health: model.HealthDown, Cause: ai.ErrProviderUnavailable}); errors.Is(err, runner.ErrProviderBlocked) {
		t.Errorf("the call that tripped the provider = %v, want it left a failed call: it was made", err)
	}
}
