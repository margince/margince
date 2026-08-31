// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// follow_up_auto_resolve against real Postgres: a captured touch on the
// lead completes the SYSTEM's open follow-up tasks and only those — the
// human's own task survives — a lead leaving the open pool completes
// them too, and the captured task itself never resolves anything (it IS
// the follow-up, not the follow-up happening). Rows are seeded raw
// because the state under repair is precisely what the pre-claim engine
// wrote; the completions themselves run through UpdateActivity, the
// module's real writer.

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

type resolveEnv struct {
	owner *pgx.Conn
	pool  *pgxpool.Pool
	ws    ids.UUID
	rep   ids.UUID
	lead  ids.UUID
}

func setupResolve(t *testing.T) *resolveEnv {
	t.Helper()
	ownerDSN := os.Getenv("MARGINCE_TEST_DSN")
	appDSN := os.Getenv("MARGINCE_TEST_APP_DSN")
	if ownerDSN == "" || appDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN / MARGINCE_TEST_APP_DSN not set — run `make db-up` " +
			"(integration tests fail loudly, they never skip)")
	}
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	e := &resolveEnv{owner: owner, ws: ids.NewV7(), rep: ids.NewV7(), lead: ids.NewV7()}
	e.exec(t, `INSERT INTO workspace (id) VALUES ($1)`, e.ws)
	e.exec(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Rep')`,
		e.rep, "rep-"+e.rep.String()+"@resolve.test")
	e.exec(t, `INSERT INTO lead (id, full_name, status, source, captured_by, owner_id)
		VALUES ($1, 'Resolve Lead', 'new', 'manual', $2, $3)`, e.lead, "human:"+e.rep.String(), e.rep)
	pool, err := testdb.Pool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testdb.AssertPoolsQuiesced(t) })
	e.pool = pool
	return e
}

func (e *resolveEnv) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("seed %q: %v", sql, err)
	}
}

// seedTask writes one open task linked to the lead, with the given
// provenance pair — source 'system' + captured_by 'system' is the
// engine's own shape; anything else is a person's row, whatever its
// source claims.
func (e *resolveEnv) seedTask(t *testing.T, source, capturedBy string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, due_at, source, captured_by)
		VALUES ($1, 'task', 'Follow up with the new lead', now(), now() + interval '1 day', $2, $3)`,
		id, source, capturedBy)
	e.exec(t, `INSERT INTO activity_link (activity_id, entity_type, lead_id) VALUES ($1, 'lead', $2)`, id, e.lead)
	return id
}

func (e *resolveEnv) isDone(t *testing.T, id ids.UUID) bool {
	t.Helper()
	var done bool
	if err := e.owner.QueryRow(context.Background(),
		`SELECT is_done FROM activity WHERE id = $1`, id).Scan(&done); err != nil {
		t.Fatal(err)
	}
	return done
}

// systemCtx is the principal the workflow engine runs handlers under.
func (e *resolveEnv) systemCtx() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: "system"})
}

// handlerFor picks one registered arm by trigger, so the test drives the
// exact handler compose registers rather than a rebuilt copy.
func handlerFor(t *testing.T, store *Store, trigger string) workflow.Handler {
	t.Helper()
	for _, h := range FollowUpWorkflows(store) {
		if h.Spec().Trigger.EventType == trigger {
			return h
		}
	}
	t.Fatalf("no follow-up workflow registered for trigger %s", trigger)
	return nil
}

// fire runs the full Match → Plan → Apply of one arm for one event.
func fire(ctx context.Context, t *testing.T, h workflow.Handler, ev workflow.Event) workflow.RunResult {
	t.Helper()
	matched, err := h.Match(ctx, ev)
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if !matched {
		return workflow.RunResult{}
	}
	eff, err := h.Plan(ctx, ev)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	result, err := h.Apply(ctx, ev, eff, nil)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	return result
}

func capturedEvent(t *testing.T, activity ids.UUID, kind string) workflow.Event {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"kind": kind})
	if err != nil {
		t.Fatal(err)
	}
	return workflow.Event{
		ID:         ids.NewV7(),
		Type:       "activity.captured",
		OccurredAt: time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC),
		Entity:     datasource.EntityRef{Type: datasource.EntityActivity, ID: activity},
		Payload:    payload,
	}
}

func TestACapturedTouchCompletesTheSystemTaskAndLeavesTheHumans(t *testing.T) {
	e := setupResolve(t)
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	systemTask := e.seedTask(t, "system", "system")
	humanTask := e.seedTask(t, "web", "human:"+e.rep.String())
	// A task a caller PLANTED with source "system": captured_by names the
	// person, because no client write can spell the system principal — and
	// that is exactly why the resolver must leave it open.
	forgedTask := e.seedTask(t, "system", "human:"+e.rep.String())

	// The touch: a captured call, linked to the same lead.
	call := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'call', 'Spoke with the lead', now(), 'web', $2)`, call, "human:"+e.rep.String())
	e.exec(t, `INSERT INTO activity_link (activity_id, entity_type, lead_id) VALUES ($1, 'lead', $2)`, call, e.lead)

	ctx := e.systemCtx()
	h := handlerFor(t, store, "activity.captured")
	result := fire(ctx, t, h, capturedEvent(t, call, "call"))
	if len(result.Applied) == 0 {
		t.Fatal("a captured call on the lead resolved nothing — the follow-up loop stays open forever")
	}
	if !e.isDone(t, systemTask) {
		t.Error("the system follow-up task is still open after the follow-up happened")
	}
	if e.isDone(t, humanTask) {
		t.Error("the HUMAN's task was completed — the system claimed work a person may not consider done")
	}
	if e.isDone(t, forgedTask) {
		t.Error("a task with a forged source 'system' was completed — captured_by, not source, decides what the system minted")
	}
	// Replay: nothing left to resolve, and nothing breaks.
	replay := fire(ctx, t, h, capturedEvent(t, call, "call"))
	if len(replay.Applied) != 0 {
		t.Error("a replayed touch claimed to resolve tasks a first pass already completed")
	}
}

func TestACapturedTaskResolvesNothing(t *testing.T) {
	e := setupResolve(t)
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	systemTask := e.seedTask(t, "system", "system")

	ctx := e.systemCtx()
	h := handlerFor(t, store, "activity.captured")
	// The minted follow-up task arrives as its own activity.captured; were
	// this a match, every reminder would complete the moment it was created.
	fire(ctx, t, h, capturedEvent(t, systemTask, "task"))
	if e.isDone(t, systemTask) {
		t.Error("the follow-up task's own capture event completed it — a reminder that self-destructs on arrival")
	}
}

func TestALeadLeavingTheOpenPoolCompletesItsSystemTasks(t *testing.T) {
	e := setupResolve(t)
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	systemTask := e.seedTask(t, "system", "system")

	ctx := e.systemCtx()
	h := handlerFor(t, store, "lead.promoted")
	fire(ctx, t, h, workflow.Event{
		ID:         ids.NewV7(),
		Type:       "lead.promoted",
		OccurredAt: time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC),
		Entity:     datasource.EntityRef{Type: "lead", ID: e.lead},
	})
	if !e.isDone(t, systemTask) {
		t.Error("the promoted lead's system follow-up is still open — a reminder chasing a closed loop")
	}
}

// Two firings for one lead — an activity.captured racing a lead.promoted —
// both select the same open task while it is still open. Only the one that
// actually flips it may write a completion: the other adds an audit row and an
// activity.updated event saying the task was finished, about a task that was
// already finished, and the record's history then shows the same thing
// happening twice.
//
// The interleaving is FORCED rather than hoped for. A sequential second call
// proves nothing here — it re-runs the open-task query, finds nothing, and
// writes nothing whatever the completion path does. What has to be reproduced
// is a firing that selected the task while it was open and reaches its write
// after somebody else finished it, so the row is locked under it until that
// has happened.
func TestAFiringThatSelectedBeforeSomebodyElseFinishedWritesNothing(t *testing.T) {
	e := setupResolve(t)
	task := e.seedTask(t, "system", "system")
	ctx := e.systemCtx()
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))

	// Hold the row so the firing below gets past its selection and stops at
	// its write, which is exactly where the loser of the race stands.
	blocking, err := e.owner.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := blocking.Exec(context.Background(),
		`SELECT id FROM activity WHERE id = $1 FOR UPDATE`, task); err != nil {
		t.Fatal(err)
	}

	loser := make(chan firingResult, 1)
	go func() {
		completed, err := store.CompleteOpenSystemTasksForLead(ctx, ids.From[ids.LeadKind](e.lead))
		loser <- firingResult{completed, err}
	}()
	// However this test ends, the lock is released and the firing is waited
	// for: a goroutine still holding a pooled connection outlives the test and
	// the next one resets the database under it.
	t.Cleanup(func() {
		//craft:ignore swallowed-errors the lock is normally released by the Commit above and this rollback is then a designed no-op; on the failure paths it is the release itself, and there is nothing a failing release could tell this test that its own assertions have not already said
		_ = blocking.Rollback(context.Background())
		select {
		case <-loser:
		case <-time.After(10 * time.Second):
			t.Error("the firing never returned after the lock was released")
		}
	})

	e.waitForBlockedWriter(t, loser)

	// The winner: the task is completed and its version moves, under the lock
	// the loser is waiting on.
	if _, err := blocking.Exec(context.Background(),
		`UPDATE activity SET is_done = true, done_at = now(), version = version + 1 WHERE id = $1`, task); err != nil {
		t.Fatal(err)
	}
	if err := blocking.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}

	var got firingResult
	select {
	case got = <-loser:
	case <-time.After(10 * time.Second):
		t.Fatal("the firing never returned after the winner committed")
	}
	// Drained here, so the cleanup above finds the channel empty and does not
	// wait for a result that has already been read.
	loser <- got
	if got.err != nil {
		t.Fatalf("the losing firing failed instead of standing down: %v", got.err)
	}
	if got.completed != 0 {
		t.Errorf("the losing firing reports %d completions, want none — it completed nothing", got.completed)
	}
	if events := e.completionEvents(t, task); events != 0 {
		t.Fatalf("the losing firing wrote %d activity.updated event(s) for a task it did not finish", events)
	}
}

// firingResult is one CompleteOpenSystemTasksForLead call's answer, carried
// back from the goroutine that raced.
type firingResult struct {
	completed int
	err       error
}

// waitForBlockedWriter waits until the firing under test is stopped on the row
// lock. Polled rather than slept on: the point of the test is the ordering,
// and a fixed pause would either be flaky or slow.
func (e *resolveEnv) waitForBlockedWriter(t *testing.T, firing <-chan firingResult) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		// A firing that RETURNED never reached the write, so the ordering this
		// test needs did not happen and nothing it goes on to assert would
		// mean anything. Said here rather than as a timeout, because "it
		// finished early" and "it never started" are different faults.
		select {
		case got := <-firing:
			t.Fatalf("the firing returned before it blocked (completed=%d err=%v): the race was never set up",
				got.completed, got.err)
		default:
		}
		var blocked int
		// A waiter on a ROW lock does not appear as an ungranted lock on the
		// relation — it waits on the holder's transaction id — so the wait
		// event is what says it is stopped. Not its `state`: a backend parked
		// on a lock inside a transaction reports "idle in transaction", and
		// requiring "active" here looks right and finds nothing.
		if err := e.owner.QueryRow(context.Background(),
			`SELECT count(*) FROM pg_stat_activity
			  WHERE wait_event_type = 'Lock' AND pid <> pg_backend_pid()
			    AND datname = current_database()`).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked > 0 {
			return
		}
		//craft:ignore test-sleep the wait IS on the condition — a backend stopped on the lock — and this is only the interval between checks; the deadline above fails the test loudly rather than letting it pass on a race that never happened
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no writer ever blocked on the row: the race this test reproduces did not happen, so it proves nothing")
}

// completionEvents counts the activity.updated events this task produced — the
// noise a redundant write leaves behind.
func (e *resolveEnv) completionEvents(t *testing.T, id ids.UUID) int {
	t.Helper()
	var count int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM event_outbox
		  WHERE envelope->>'type' = 'activity.updated'
		    AND envelope->'entity'->>'id' = $1::text`, id.String()).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
