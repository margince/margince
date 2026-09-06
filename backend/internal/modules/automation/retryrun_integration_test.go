// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package automation

// Retrying a failed firing, against real Postgres.
//
// Each case drives a REAL failure through the engine — a scripted Apply that
// returns an error on its first pass — rather than hand-inserting a 'failed'
// row. A seeded row would prove the SQL reads what the test wrote and nothing
// about whether the engine's own failure path leaves a run a retry can find.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// retryInstant is this file's frozen clock: every envelope it stages occurred
// at the same moment, so nothing here depends on when the suite runs.
var retryInstant = time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC)

// stagedEvent puts one envelope in event_outbox and returns it, so a run
// triggered by it has a trigger event a retry can recover. The relay publishes
// from this table and never deletes the row, which is what makes a retry
// possible at all.
func (fx *autoFixture) stagedEvent(t *testing.T, entity ids.UUID) kevents.Envelope {
	t.Helper()
	env := kevents.Envelope{
		EventID:    ids.NewV7(),
		Type:       scriptedTrigger,
		Version:    1,
		OccurredAt: retryInstant,
		Entity:     kevents.EntityRef{Type: "lead", ID: entity},
	}
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshalling the envelope: %v", err)
	}
	fx.exec(t, `
		INSERT INTO event_outbox (id, stream, envelope, published_at)
		VALUES ($1, $2, $3::jsonb, now())`,
		env.EventID, "crm.lead", raw)
	return env
}

// failingOnce is an Apply that fails its first call and succeeds after — the
// transient failure a retry exists for. It counts calls so a test can prove
// the retry actually re-entered Apply rather than reporting success from the
// stored run.
type failingOnce struct{ calls int }

func (f *failingOnce) apply() func(workflow.Event) (workflow.RunResult, error) {
	return func(workflow.Event) (workflow.RunResult, error) {
		f.calls++
		if f.calls == 1 {
			return workflow.RunResult{}, errors.New("the downstream service was unreachable")
		}
		return workflow.RunResult{}, nil
	}
}

// engineOverScripted registers one scripted handler on a real engine.
func engineOverScripted(fx *autoFixture, h scriptedWorkflow) *WorkflowEngine {
	db := database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws))
	engine := NewWorkflowEngine(db, fixtureResolver{})
	engine.RegisterWorkflow(h)
	return engine
}

// failedRunID is the id of the single run the fixture's handler recorded.
func (fx *autoFixture) failedRunID(t *testing.T, handler string) (ids.UUID, string) {
	t.Helper()
	var id ids.UUID
	var status string
	if err := fx.owner.QueryRow(context.Background(),
		`SELECT id, status FROM workflow_run WHERE handler = $1`, handler).Scan(&id, &status); err != nil {
		t.Fatalf("reading the recorded run: %v", err)
	}
	return id, status
}

func TestARetriedRunAppliesOnTheSecondPass(t *testing.T) {
	fx := setupAutomationDB(t)
	const handler = "retry_me"
	fx.seedAutomation(t, handler)

	script := &failingOnce{}
	engine := engineOverScripted(fx, scriptedWorkflow{
		name: handler, apply: script.apply(), redrivable: true,
	})
	env := fx.stagedEvent(t, ids.NewV7())
	if err := engine.HandleEvent(context.Background(), env); err == nil {
		t.Fatal("the scripted Apply failed, so HandleEvent must surface it")
	}
	runID, status := fx.failedRunID(t, handler)
	if status != "failed" {
		t.Fatalf("the first pass recorded status %q, want failed", status)
	}

	outcome, err := engine.RetryRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("RetryRun: %v", err)
	}
	if !outcome.Retried {
		t.Fatalf("a failed run of a re-drivable handler refused the retry: %q", outcome.Refusal)
	}
	// The proof the retry re-DISPATCHED rather than reporting the stored run:
	// Apply ran a second time.
	if script.calls != 2 {
		t.Fatalf("Apply ran %d times, want 2 — the retry did not re-enter the handler", script.calls)
	}
}

func TestARetryRefusesARunThatDidNotFail(t *testing.T) {
	fx := setupAutomationDB(t)
	const handler = "applied_cleanly"
	fx.seedAutomation(t, handler)

	engine := engineOverScripted(fx, scriptedWorkflow{name: handler, redrivable: true})
	if err := engine.HandleEvent(context.Background(), fx.stagedEvent(t, ids.NewV7())); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	runID, status := fx.failedRunID(t, handler)
	if status != "applied" {
		t.Fatalf("the run recorded status %q, want applied", status)
	}

	outcome, err := engine.RetryRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("RetryRun: %v", err)
	}
	if outcome.Retried || outcome.Refusal != RetryRefusedNotFailed {
		t.Fatalf("retrying an applied run answered (%v, %q), want a not_failed refusal",
			outcome.Retried, outcome.Refusal)
	}
}

func TestARetryRefusesAHandlerThatWouldRepeatItsEffect(t *testing.T) {
	fx := setupAutomationDB(t)
	const handler = "unaudited"
	fx.seedAutomation(t, handler)

	script := &failingOnce{}
	// redrivable stays false: the default a handler nobody has audited carries.
	engine := engineOverScripted(fx, scriptedWorkflow{name: handler, apply: script.apply()})
	if err := engine.HandleEvent(context.Background(), fx.stagedEvent(t, ids.NewV7())); err == nil {
		t.Fatal("the scripted Apply failed, so HandleEvent must surface it")
	}
	runID, _ := fx.failedRunID(t, handler)

	outcome, err := engine.RetryRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("RetryRun: %v", err)
	}
	if outcome.Retried || outcome.Refusal != RetryRefusedRepeats {
		t.Fatalf("retrying an unaudited handler answered (%v, %q), want a repeats_its_effect refusal",
			outcome.Retried, outcome.Refusal)
	}
	if script.calls != 1 {
		t.Fatalf("Apply ran %d times, want 1 — the refusal must not have re-entered the handler", script.calls)
	}
}

func TestARetryRefusesARunWhoseTriggerEventIsGone(t *testing.T) {
	fx := setupAutomationDB(t)
	const handler = "clock_shaped"
	fx.seedAutomation(t, handler)

	script := &failingOnce{}
	engine := engineOverScripted(fx, scriptedWorkflow{
		name: handler, apply: script.apply(), redrivable: true,
	})
	// An envelope the engine never staged: this is the shape of a CLOCK firing,
	// whose trigger_event is synthesized per evaluation pass (timescan.go) and
	// was never an outbox row. Nothing can rebuild the event, so nothing may
	// claim to re-drive it.
	unstaged := kevents.Envelope{
		EventID:    ids.NewV7(),
		Type:       scriptedTrigger,
		Version:    1,
		OccurredAt: retryInstant,
		Entity:     kevents.EntityRef{Type: "lead", ID: ids.NewV7()},
	}
	if err := engine.HandleEvent(context.Background(), unstaged); err == nil {
		t.Fatal("the scripted Apply failed, so HandleEvent must surface it")
	}
	runID, _ := fx.failedRunID(t, handler)

	outcome, err := engine.RetryRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("RetryRun: %v", err)
	}
	if outcome.Retried || outcome.Refusal != RetryRefusedEventGone {
		t.Fatalf("retrying a run with no recoverable event answered (%v, %q), "+
			"want a trigger_event_unavailable refusal", outcome.Retried, outcome.Refusal)
	}
	if script.calls != 1 {
		t.Fatalf("Apply ran %d times, want 1 — the refusal must not have re-entered the handler", script.calls)
	}
}

func TestARetriedRunStaysVisibleToTheHealthLane(t *testing.T) {
	fx := setupAutomationDB(t)
	const handler = "still_broken"
	fx.seedAutomation(t, handler)

	// An Apply that fails every time, so the retry's own run is failed too and
	// the health lane has something to find.
	engine := engineOverScripted(fx, scriptedWorkflow{
		name:       handler,
		redrivable: true,
		apply: func(workflow.Event) (workflow.RunResult, error) {
			return workflow.RunResult{}, errors.New("the downstream service was unreachable")
		},
	})
	if err := engine.HandleEvent(context.Background(), fx.stagedEvent(t, ids.NewV7())); err == nil {
		t.Fatal("the scripted Apply failed, so HandleEvent must surface it")
	}
	runID, _ := fx.failedRunID(t, handler)
	// The retry re-runs an Apply that fails again, so RetryRun surfaces that
	// failure exactly as the first dispatch did. The run row lands either way,
	// which is what this test is about.
	if _, err := engine.RetryRun(context.Background(), runID); err == nil {
		t.Fatal("the scripted Apply fails every time, so the retry must surface it too")
	}

	// THE POINT OF THE PREFIX. troubledRunsSQL correlates a run to its
	// automation with `idempotency_key LIKE '%@' || a.id`, anchored at the END.
	// An attempt marker appended after the automation id would leave the
	// retry's run row matching nothing — the failure would vanish from the
	// very lane that offered the retry, and no test anywhere would fail.
	both := fx.count(t, `
		SELECT count(*) FROM workflow_run r
		  JOIN automation a ON a.archived_at IS NULL AND a.enabled
		   AND r.handler = a.key
		   AND r.idempotency_key LIKE '%@' || a.id
		 WHERE r.status = 'failed' AND r.handler = $1`, handler)
	if both != 2 {
		t.Fatalf("the health lane's join found %d failed runs, want 2 — the original and its "+
			"retry. A retried run that stops matching this join disappears from the lane "+
			"that offered the retry, silently", both)
	}
}

func TestRetryingARetryIsItsOwnAttempt(t *testing.T) {
	fx := setupAutomationDB(t)
	const handler = "twice_broken"
	fx.seedAutomation(t, handler)

	applies := 0
	engine := engineOverScripted(fx, scriptedWorkflow{
		name:       handler,
		redrivable: true,
		apply: func(workflow.Event) (workflow.RunResult, error) {
			applies++
			return workflow.RunResult{}, errors.New("the downstream service was unreachable")
		},
	})
	if err := engine.HandleEvent(context.Background(), fx.stagedEvent(t, ids.NewV7())); err == nil {
		t.Fatal("the scripted Apply failed, so HandleEvent must surface it")
	}

	// Retry twice. The SECOND retry is a retry OF A RETRY, whose own run key
	// already carries an attempt marker. An attempt count that cannot see
	// through that marker restarts at zero, re-mints the first retry's key,
	// loses the claim, and returns having applied nothing — while reporting
	// success, which is the worst shape a failure can take.
	for attempt := range 2 {
		var failed ids.UUID
		if err := fx.owner.QueryRow(context.Background(),
			`SELECT id FROM workflow_run WHERE handler = $1 AND status = 'failed'
			  ORDER BY created_at DESC, id DESC LIMIT 1`, handler).Scan(&failed); err != nil {
			t.Fatalf("reading the newest failed run: %v", err)
		}
		if _, err := engine.RetryRun(context.Background(), failed); err == nil {
			t.Fatalf("retry %d: the scripted Apply fails every time, so the retry must surface it", attempt+1)
		}
	}

	if applies != 3 {
		t.Fatalf("Apply ran %d times, want 3 — the original firing and two retries. "+
			"A retry that reports success without re-entering Apply is a false green", applies)
	}
	runs := fx.count(t, `SELECT count(*) FROM workflow_run WHERE handler = $1`, handler)
	if runs != 3 {
		t.Fatalf("recorded %d runs, want 3 — each attempt claims its own row", runs)
	}
}
