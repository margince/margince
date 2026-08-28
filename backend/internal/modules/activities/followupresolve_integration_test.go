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
