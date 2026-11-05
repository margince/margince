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
	"github.com/margince/margince/backend/internal/modules/assurance"
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

// answerStamps reads who an answered finding names.
func (v *viabilityEnv) answerStamps(t *testing.T, review ids.UUID) (actor, capturedBy string) {
	t.Helper()
	return v.WsScalar(t, `SELECT actor_id::text FROM assurance_resolution WHERE exception_id = $1`, review),
		v.WsScalar(t, `SELECT captured_by FROM assurance_resolution WHERE exception_id = $1`, review)
}

func (v *viabilityEnv) assertAnswerNamesTheAgent(t *testing.T, review ids.UUID) {
	t.Helper()
	actor, capturedBy := v.answerStamps(t, review)
	if actor != v.AdminUser.String() || !strings.HasPrefix(capturedBy, "agent:") {
		t.Errorf("the answer names user %s and author %q, want the admin and the agent", actor, capturedBy)
	}
}

// An agent waits on a finding about a real deal, as the user does on the panel.
func TestAnAgentDefersAnInputCheckOnARealDeal(t *testing.T) {
	v := setupViability(t)
	agent := agentOfTheAdmin(t, v.Env)
	v.check(t)
	review := v.reviewOf(t, v.silent)
	until := clocktest.Now(t).AddDate(0, 0, 30).Format(time.DateOnly)
	if rec := v.postAnswerAs(agent, t, review, "wait", map[string]any{"until": until}); rec.Code != http.StatusNoContent {
		t.Fatalf("the agent's wait answered %d %s", rec.Code, rec.Body)
	}
	if got := v.WsScalar(t, `SELECT coalesce(wait_until::text, '') FROM deal WHERE id = $1`, v.silent); got != until {
		t.Errorf("the deal waits until %q, want %s", got, until)
	}
	v.assertAnswerNamesTheAgent(t, review)
}

// An agent closes the deal behind a finding, which closes the finding too.
func TestAnAgentClosesARealDealLostFromAnInputCheck(t *testing.T) {
	v := setupViability(t)
	agent := agentOfTheAdmin(t, v.Env)
	v.check(t)
	review := v.reviewOf(t, v.silent)
	if rec := v.postAnswerAs(agent, t, review, "close-lost", map[string]any{"lost_reason": "They chose a rival."}); rec.Code != http.StatusNoContent {
		t.Fatalf("the agent's close answered %d %s", rec.Code, rec.Body)
	}
	if got := v.WsScalar(t, `SELECT status FROM deal WHERE id = $1`, v.silent); got != "lost" {
		t.Errorf("the deal is %q, want lost", got)
	}
	if v.openReviews(t, v.silent) != 0 {
		t.Error("the finding stayed open")
	}
	v.assertAnswerNamesTheAgent(t, review)
}

// A revival is refused the agent as it is refused the user, until the customer is
// back, and the answer then names the agent.
func TestAnAgentRevivesARealDealOnlyOnceTheCustomerIsBack(t *testing.T) {
	v := setupViability(t)
	agent := agentOfTheAdmin(t, v.Env)
	v.check(t)
	review := v.reviewOf(t, v.silent)
	revival := assurance.Resolution{Outcome: assurance.OutcomeFixedRecord}
	if err := v.reviewStore().Resolve(agent, review, revival, clocktest.Now(t)); err == nil {
		t.Fatal("an agent revived a deal whose customer is still silent")
	}
	v.customerWrites(t, v.silent, clocktest.Now(t).Add(-time.Hour))
	if err := v.reviewStore().Resolve(agent, review, revival, clocktest.Now(t)); err != nil {
		t.Fatalf("the agent's revival: %v", err)
	}
	v.assertAnswerNamesTheAgent(t, review)
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
