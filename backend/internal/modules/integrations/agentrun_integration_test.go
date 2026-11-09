// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integrations

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// agentCtx is the environment's rep acting through an agent on a passport.
func (e *runsEnv) agentCtx() context.Context {
	actor, ok := principal.Actor(e.ctx)
	if !ok {
		panic("the runs environment has no actor")
	}
	actor.Type, actor.ID, actor.PassportID, actor.OnBehalfOf = principal.PrincipalAgent, "agent:test", ids.NewV7(), actor.UserID
	return principal.WithActor(e.ctx, actor)
}

func (e *runsEnv) setConnection(t *testing.T, status string, limit *int, refreshDays *int) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE provider_connection SET status = $2, daily_run_limit = $3, refresh_after_days = $4 WHERE provider = $1`,
		"surfe", status, limit, refreshDays); err != nil {
		t.Fatalf("setting the connection: %v", err)
	}
}

// An agent spends a provider's credits only under a daily limit someone set. It
// does not spend while the connection is out of credits or being rate limited.
func TestAnAgentsRunNeedsALimitAndAHealthyConnection(t *testing.T) {
	e := setupRuns(t, runsConfig{})
	in := provider.QueueInput{
		ContactID: e.mine.String(), Provider: "surfe", Trigger: provider.TriggerManual,
		Categories: []provider.Category{"professional_email"},
	}
	limit := 10
	e.setConnection(t, "connected", nil, nil)
	if _, err := e.store.QueueRun(e.agentCtx(), in); !errors.Is(err, ErrAgentNeedsDailyLimit) {
		t.Errorf("no daily limit answered %v", err)
	}
	for _, status := range []string{"insufficient_credits", "rate_limited", "provider_error"} {
		e.setConnection(t, status, &limit, nil)
		if _, err := e.store.QueueRun(e.agentCtx(), in); !errors.Is(err, ErrConnectionNotHealthyForAgent) {
			t.Errorf("%s answered %v", status, err)
		}
		if _, err := e.store.QueueRun(e.ctx, in); err != nil {
			t.Errorf("a user's deliberate probe on %s was refused: %v", status, err)
		}
		if _, err := e.owner.Exec(context.Background(), `DELETE FROM provider_run WHERE contact_id = $1`, e.mine); err != nil {
			t.Fatal(err)
		}
	}
	e.setConnection(t, "invalid_credentials", &limit, nil)
	if _, err := e.store.QueueRun(e.agentCtx(), in); !errors.Is(err, ErrConnectionNotHealthyForAgent) {
		t.Errorf("a key the provider refused answered %v", err)
	}
	e.setConnection(t, "connected", &limit, nil)
	run, err := e.store.QueueRun(e.agentCtx(), in)
	if err != nil || run.State != provider.RunQueued {
		t.Fatalf("a healthy connection under a limit answered %+v, %v", run, err)
	}
}

// A user asking knows something the timestamps do not. An agent does not, so a
// fresh answer stands and its run is skipped.
func TestAnAgentsRunKeepsTheFreshnessCheck(t *testing.T) {
	e := setupRuns(t, runsConfig{})
	limit, days := 10, 30
	e.setConnection(t, "connected", &limit, &days)
	in := provider.QueueInput{
		ContactID: e.mine.String(), Provider: "surfe", Trigger: provider.TriggerManual,
		Categories: []provider.Category{"professional_email"},
	}
	if _, err := e.store.QueueRun(e.ctx, in); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE provider_run SET state = 'completed', completed_at = now() WHERE contact_id = $1`, e.mine); err != nil {
		t.Fatal(err)
	}
	run, err := e.store.QueueRun(e.agentCtx(), in)
	if err != nil {
		t.Fatal(err)
	}
	if run.State != provider.RunSkipped || run.SkipReason != provider.SkipAlreadyFresh {
		t.Errorf("the agent's run is %s/%s, want skipped/already_fresh", run.State, run.SkipReason)
	}
	human, err := e.store.QueueRun(e.ctx, in)
	if err != nil || human.State == provider.RunSkipped {
		t.Errorf("a user's run is %+v, %v, want it to run", human, err)
	}
}

// An agent names what it buys. Naming nothing takes only what costs nothing, so a
// priced detail is never bought by leaving the argument out.
func TestAnAgentBuysAPricedCategoryOnlyByNamingIt(t *testing.T) {
	e := setupRuns(t, runsConfig{})
	limit := 10
	e.setConnection(t, "connected", &limit, nil)
	if _, err := e.store.QueueRun(e.agentCtx(), provider.QueueInput{
		ContactID: e.mine.String(), Provider: "surfe", Trigger: provider.TriggerManual,
	}); !errors.Is(err, ErrNothingFreeToBuy) {
		t.Errorf("naming nothing on a priced connection answered %v", err)
	}
	if run, err := e.store.QueueRun(e.ctx, provider.QueueInput{
		ContactID: e.mine.String(), Provider: "surfe", Trigger: provider.TriggerManual,
	}); err != nil || run.State != provider.RunQueued {
		t.Errorf("a user's plain lookup answered %+v, %v", run, err)
	}
}
