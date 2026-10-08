// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/providerwait"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// deferredLine reads one carrier's row of the admin's deferred-work list.
func deferredLine(t *testing.T, e *integration.Env, carrier string) (budget, provider int64) {
	t.Helper()
	lines, err := aiDeferredWork(e.Pool)(e.Admin())
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range lines {
		if line.Carrier == carrier {
			return budgetCount(t, line), providerCount(t, line)
		}
	}
	t.Fatalf("no %s line in %+v", carrier, lines)
	return 0, 0
}

func budgetCount(t *testing.T, line crmcontracts.AiDeferredWork) int64 {
	t.Helper()
	if !line.Available || line.Count == nil {
		t.Fatalf("%s is unavailable: %+v", line.Carrier, line)
	}
	return *line.Count
}

func providerCount(t *testing.T, line crmcontracts.AiDeferredWork) int64 {
	t.Helper()
	if line.WaitingOnProvider == nil {
		t.Fatalf("%s carries no provider-wait count: %+v", line.Carrier, line)
	}
	return *line.WaitingOnProvider
}

var (
	farFuture = time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	outage    = &ai.ProviderDownError{Provider: "acme", Health: model.HealthDown, RetryAfter: farFuture}
	exhausted = &ai.BudgetDeferralError{Task: ai.TaskSiteExtract, NextAttemptAt: farFuture}
)

// The CHECK constraints forbid a NULL detail on a deferred row today, so the
// only way to prove NotClause keeps one is to evaluate it against a NULL.
func TestNotClauseKeepsARowWhoseDetailIsNull(t *testing.T) {
	e := integration.Setup(t)
	var kept, matched int
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(context.Background(),
			`SELECT count(*) FROM (SELECT NULL::text AS status_detail) r WHERE `+providerwait.NotClause).Scan(&kept); err != nil {
			return err
		}
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM (SELECT NULL::text AS status_detail) r WHERE `+providerwait.Clause).Scan(&matched)
	})
	if err != nil {
		t.Fatal(err)
	}
	if kept != 1 || matched != 0 {
		t.Fatalf("a NULL detail: NotClause kept %d, Clause matched %d, want 1 and 0", kept, matched)
	}
}

// A website read deferred for a provider outage is the provider's to resume,
// so a budget raise neither counts nor wakes it; one deferred for the budget
// is the opposite. Both are written by the real worker.
func TestWebsiteReadsSplitTheBudgetWaitFromTheProviderWait(t *testing.T) {
	e := integration.Setup(t)
	grantBudgetRequester(t, e, e.Rep1)
	inserter, err := jobs.NewInserter(e.Pool, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	causes := map[string]error{"budget": exhausted, "provider": outage}
	nextAttempt := map[string]func() (time.Time, error){}
	for name, cause := range causes {
		company := insertCompany(t, e, e.Rep1, name+".example", "")
		read, args := startDeepRead(t, e, company)
		worker, _ := newDeepReadTestWorker(e, acmeDeepSite(), budgetDeferringBrain{err: cause})
		if err := worker.run(context.Background(), args); err == nil {
			t.Fatalf("%s: expected a deferral", name)
		}
		if err := inserter.Enqueue(e.Admin(), args, &river.InsertOpts{ScheduledAt: farFuture, MaxAttempts: 3}); err != nil {
			t.Fatal(err)
		}
		nextAttempt[name] = func() (time.Time, error) {
			current, err := e.Contacts.GetSiteRead(e.As(e.Rep1, nil, integration.AdminPerms), companyIDOf(company), read.ID)
			if err != nil || current.NextAttemptAt == nil {
				return time.Time{}, err
			}
			return *current.NextAttemptAt, nil
		}
	}
	budget, provider := deferredLine(t, e, "site_read")
	if budget != 1 || provider != 1 {
		t.Fatalf("site_read counts budget=%d provider=%d, want 1 and 1", budget, provider)
	}
	if err := newAIBudgetResumeWorker(e.Pool, slog.New(slog.DiscardHandler)).resumeWorkspace(context.Background(), e.WS); err != nil {
		t.Fatal(err)
	}
	if next, err := nextAttempt["budget"](); err != nil || !next.Before(farFuture) {
		t.Fatalf("a budget raise left the budget wait at %v (%v)", next, err)
	}
	if next, err := nextAttempt["provider"](); err != nil || !next.Equal(farFuture) {
		t.Fatalf("a budget raise woke the provider wait: now %v (%v)", next, err)
	}
}

func TestVoiceBuildsSplitTheBudgetWaitFromTheProviderWait(t *testing.T) {
	cases := map[string]struct {
		cause           error
		budget, waiting int64
	}{
		"budget":   {cause: exhausted, budget: 1},
		"provider": {cause: outage, waiting: 1},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			env, build := seedVoiceBuild(t, "A deferred quote.", 6)
			grantBudgetRequester(t, env.e, env.e.Rep1)
			worker := newVoiceBuildWorker(env.e.Pool, brainFunc(func(context.Context, model.Request) (model.Response, error) {
				return model.Response{}, c.cause
			}), slog.New(slog.DiscardHandler))
			if err := worker.Work(context.Background(), voiceBuildJob(env, build)); err != nil {
				t.Fatal(err)
			}
			budget, provider := deferredLine(t, env.e, "voice_build")
			if budget != c.budget || provider != c.waiting {
				t.Fatalf("voice_build counts budget=%d provider=%d, want %d and %d", budget, provider, c.budget, c.waiting)
			}
			if err := newAIBudgetResumeWorker(env.e.Pool, slog.New(slog.DiscardHandler)).resumeWorkspace(env.e.Admin(), env.e.WS); err != nil {
				t.Fatal(err)
			}
			current, err := env.store.GetBuild(env.owner, env.profile.ID, build.ID)
			if err != nil || current.NextAttemptAt == nil {
				t.Fatalf("build after recovery: %+v (%v)", current, err)
			}
			woken := current.NextAttemptAt.Before(farFuture)
			if woken != (c.budget == 1) {
				t.Fatalf("recovery woke the build = %v, want %v", woken, c.budget == 1)
			}
		})
	}
}
