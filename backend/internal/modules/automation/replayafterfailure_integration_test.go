// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package automation

// A redelivery after a failed Apply, against real Postgres and the real engine.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// deliver runs one event through the engine n times, as the bus redelivers a
// failed one, and answers the first delivery's error.
func deliver(t *testing.T, engine *WorkflowEngine, env kevents.Envelope, n int) error {
	t.Helper()
	var first error
	for i := range n {
		if err := engine.HandleEvent(context.Background(), env); i == 0 {
			first = err
		}
	}
	return first
}

func runStatus(t *testing.T, fx *autoFixture, handler string) string {
	t.Helper()
	var status string
	if err := fx.owner.QueryRow(context.Background(),
		`SELECT status FROM workflow_run WHERE handler = $1`, handler).Scan(&status); err != nil {
		t.Fatalf("reading the run of %s: %v", handler, err)
	}
	return status
}

func scriptedFailing(fx *autoFixture, name string, replays bool, fault error) (*WorkflowEngine, *int) {
	calls := new(int)
	return engineOverScripted(fx, scriptedWorkflow{
		name: name, replays: replays,
		apply: func(workflow.Event) (workflow.RunResult, error) {
			*calls++
			if *calls == 1 {
				return workflow.RunResult{}, fault
			}
			return workflow.RunResult{}, nil
		},
	}), calls
}

func TestAReplayingHandlerIsRunAgainByTheRedeliveryOfAFailedApply(t *testing.T) {
	t.Parallel()
	fx := setupAutomationDB(t)
	fx.seedAutomation(t, "replays")
	engine, calls := scriptedFailing(fx, "replays", true, errors.New("the downstream service was unreachable"))

	if err := deliver(t, engine, fx.stagedEvent(t, ids.NewV7()), 3); err == nil {
		t.Error("the first delivery must surface the failure so the bus redelivers")
	}
	if *calls != 2 || runStatus(t, fx, "replays") != "applied" {
		t.Errorf("Apply ran %d times and the run is %q; want 2 and applied, with the delivery after success doing nothing",
			*calls, runStatus(t, fx, "replays"))
	}
}

func TestARefusalIsNotReplayedWhateverItsWording(t *testing.T) {
	t.Parallel()
	fx := setupAutomationDB(t)
	fx.seedAutomation(t, "refused")
	engine, calls := scriptedFailing(fx, "refused", true, apperrors.ErrInvalidArgument)

	if err := deliver(t, engine, fx.stagedEvent(t, ids.NewV7()), 3); err == nil {
		t.Error("the first delivery must fail")
	}
	if *calls != 1 || runStatus(t, fx, "refused") != "failed" {
		t.Errorf("Apply ran %d times and the run is %q; want 1 and failed", *calls, runStatus(t, fx, "refused"))
	}
}

func TestAHandlerThatDoesNotDeclareReplayIsNotReplayed(t *testing.T) {
	t.Parallel()
	fx := setupAutomationDB(t)
	fx.seedAutomation(t, "no_flag")
	engine, calls := scriptedFailing(fx, "no_flag", false, errors.New("the downstream service was unreachable"))

	if err := deliver(t, engine, fx.stagedEvent(t, ids.NewV7()), 3); err == nil {
		t.Error("the first delivery must fail")
	}
	if *calls != 1 || runStatus(t, fx, "no_flag") != "failed" {
		t.Errorf("Apply ran %d times and the run is %q; want 1 and failed", *calls, runStatus(t, fx, "no_flag"))
	}
}

func TestARunWithAppliedOutputOrPastTheWindowIsNotReplayed(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, set string
		replayed  bool
	}{
		{"applied output present", `applied = '[{"kind":"notify"}]'::jsonb`, false},
		{"just inside the window", `created_at = now() - interval '59 minutes'`, true},
		{"just past the window", `created_at = now() - interval '61 minutes'`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := setupAutomationDB(t)
			fx.seedAutomation(t, "windowed")
			engine, calls := scriptedFailing(fx, "windowed", true, errors.New("the downstream service was unreachable"))
			env := fx.stagedEvent(t, ids.NewV7())
			if err := deliver(t, engine, env, 1); err == nil {
				t.Fatal("the first delivery must fail")
			}
			fx.exec(t, `UPDATE workflow_run SET `+tc.set+` WHERE handler = 'windowed'`)
			t.Logf("redelivery answered: %v", deliver(t, engine, env, 1))
			if replayed := *calls == 2; replayed != tc.replayed {
				t.Errorf("Apply ran %d times; replayed = %v, want %v", *calls, replayed, tc.replayed)
			}
		})
	}
}

// createProvider fails its first Create and keeps the claim the create took.
type createProvider struct {
	fakeReadProvider
	creates int
}

func (p *createProvider) Create(context.Context, datasource.CreateInput) (datasource.EntityRef, error) {
	p.creates++
	if p.creates == 1 {
		return datasource.EntityRef{}, errors.New("the system of record was unreachable")
	}
	return datasource.EntityRef{Type: datasource.EntityActivity, ID: ids.NewV7()}, nil
}

// A create goes through the effect claim, whose unconfirmed claim outlives the
// bus redelivery. Replaying it would fold on that claim and report success for
// a record nobody wrote, so such a handler stays terminal after one failure.
func TestACreateThroughTheEffectClaimIsNotReplayedOverItsOwnClaim(t *testing.T) {
	t.Parallel()
	fx := setupAutomationDB(t)
	fx.seedAutomation(t, "creates")
	db := database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws))
	provider := &createProvider{}
	ex := Executors{Provider: provider, Claims: NewEffectClaims(db)}
	engine := NewWorkflowEngine(db, fixtureResolver{})
	engine.RegisterWorkflow(creatingHandler{ex: ex})

	if err := deliver(t, engine, fx.stagedEvent(t, ids.NewV7()), 3); err == nil {
		t.Error("the first delivery must fail")
	}
	if provider.creates != 1 || runStatus(t, fx, "creates") != "failed" {
		t.Errorf("creates = %d and the run is %q; want 1 and failed, never an applied run over a lost record",
			provider.creates, runStatus(t, fx, "creates"))
	}
}

type creatingHandler struct{ ex Executors }

func (creatingHandler) Spec() workflow.Spec {
	return workflow.Spec{Name: "creates", Trigger: workflow.Trigger{EventType: scriptedTrigger}}
}

func (creatingHandler) Match(context.Context, workflow.Event) (bool, error) { return true, nil }

func (creatingHandler) Plan(_ context.Context, ev workflow.Event) (workflow.Effect, error) {
	return workflow.Effect{Actions: []workflow.Action{{Kind: workflow.ActionCreateTask, Target: ev.Entity, Args: []byte(`{"title":"x"}`)}}}, nil
}

func (h creatingHandler) Apply(ctx context.Context, _ workflow.Event, eff workflow.Effect, _ *workflow.ApprovalToken) (workflow.RunResult, error) {
	applied, err := ApplyActions(ctx, h.ex, eff)
	return workflow.RunResult{Applied: applied}, err
}

func (creatingHandler) IdempotencyKey(ev workflow.Event) string { return "creates:" + ev.ID.String() }

// keyedNotifier fails its first call and then records each dedupe key once, as
// the notice writer does.
type keyedNotifier struct {
	calls int
	sent  map[string]int
}

func (n *keyedNotifier) Notify(_ context.Context, _ ids.UUID, _, _ string, _ datasource.EntityRef, dedupe string, _ *crmcontracts.NoticeOrigin) error {
	n.calls++
	if n.calls == 1 {
		return errors.New("connection reset by peer")
	}
	if n.sent == nil {
		n.sent = map[string]int{}
	}
	n.sent[dedupe]++
	return nil
}

func TestTheStageChangeNotificationIsRetriedAfterATransientNoticeFailure(t *testing.T) {
	t.Parallel()
	fx := setupAutomationDB(t)
	fx.seedAutomation(t, stageChangeNotifyName)
	owner := fx.rep1
	fields, err := json.Marshal(dealOwnerFields{OwnerID: &owner, Name: "Fleet deal"})
	if err != nil {
		t.Fatal(err)
	}
	notifier := &keyedNotifier{}
	db := database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws))
	engine := NewWorkflowEngine(db, fixtureResolver{})
	engine.RegisterWorkflow(stageChangeNotify{ex: Executors{
		Provider: &fakeReadProvider{record: datasource.Record{Fields: fields}}, Notifier: notifier,
	}})
	env := kevents.Envelope{
		EventID: ids.NewV7(), Type: eventDealStageChanged, Version: 1, OccurredAt: clocktest.Now(t).UTC(),
		Entity: kevents.EntityRef{Type: "deal", ID: ids.NewV7()},
	}

	if err := deliver(t, engine, env, 3); err == nil {
		t.Error("the first delivery must surface the notice failure")
	}
	if notifier.calls != 2 || len(notifier.sent) != 1 || runStatus(t, fx, stageChangeNotifyName) != "applied" {
		t.Errorf("notify calls = %d, notices = %v, run = %q; want 2 calls, one notice and an applied run",
			notifier.calls, notifier.sent, runStatus(t, fx, stageChangeNotifyName))
	}
}
