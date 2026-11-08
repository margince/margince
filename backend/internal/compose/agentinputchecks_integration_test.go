// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// agentOfTheAdmin is the admin's agent on a passport the database holds.
func agentOfTheAdmin(t *testing.T, e *integration.Env) context.Context {
	t.Helper()
	e.GrantRole(t, e.AdminUser, "admin")
	passport := e.SeedPassportFor(t, integration.OwnerConn(t), "input-check agent", e.AdminUser)
	return e.AgentOnPassport(t, e.AdminUser, nil, integration.AdminPerms, ids.From[ids.PassportKind](passport))
}

// capturingEnqueuer keeps the job a start asked for.
type capturingEnqueuer struct{ args river.JobArgs }

func (c *capturingEnqueuer) EnqueueTxUnique(_ context.Context, _ pgx.Tx, args river.JobArgs, _ *river.InsertOpts) (bool, error) {
	c.args = args
	return true, nil
}

// A check the agent starts is its own, and its run says so.
func TestAnAgentStartsAnInputCheckThatNamesIt(t *testing.T) {
	v := setupViability(t)
	agent := agentOfTheAdmin(t, v.Env)
	queue := &capturingEnqueuer{}
	h := assuranceRunHandlers{pool: v.Pool, enqueue: queue, now: func() time.Time { return clocktest.Now(t) }}
	rec := httptest.NewRecorder()
	h.StartForecastAssuranceRun(rec, httptest.NewRequest(http.MethodPost, "/v1/forecast/assurance/runs", nil).WithContext(agent))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("the agent's start answered %d %s", rec.Code, rec.Body)
	}
	args, ok := queue.args.(AssuranceRunArgs)
	if !ok || !strings.HasPrefix(args.RequestedBy, "agent:") {
		t.Fatalf("the run was asked for by %+v, want the agent", queue.args)
	}
	worker := &assuranceRunWorker{sweep: v.worker}
	if err := worker.Work(context.Background(), &river.Job[AssuranceRunArgs]{Args: args}); err != nil {
		t.Fatalf("a run the agent asked for: %v", err)
	}
	if got := v.WsScalar(t, `SELECT requested_by FROM assurance_run ORDER BY id DESC LIMIT 1`); got != args.RequestedBy {
		t.Errorf("the run names %q, want %q", got, args.RequestedBy)
	}
}
