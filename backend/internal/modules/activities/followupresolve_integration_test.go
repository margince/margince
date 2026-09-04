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

// seedPerson writes a bare person row, the target a promotion carries the
// lead's activities onto.
func (e *resolveEnv) seedPerson(t *testing.T) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	e.exec(t, `INSERT INTO person (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Resolve Person', $2, 'manual', $3)`, id, e.rep, "human:"+e.rep.String())
	return id
}

// seedTaskLinkedToPerson writes one open task linked to a person — the shape
// carryLeadActivities (people/promote.go) leaves a lead's follow-up task in,
// inside the SAME transaction that promotes the lead: entity_type 'person',
// person_id set, lead_id NULL. A resolver that still looks this task up by
// the lead id finds nothing.
func (e *resolveEnv) seedTaskLinkedToPerson(t *testing.T, source, capturedBy string, person ids.UUID) ids.UUID {
	t.Helper()
	return e.seedTaskLinkedToPersonAt(t, source, capturedBy, person, -time.Hour)
}

// seedTaskLinkedToPersonAt is seedTaskLinkedToPerson with the task's created_at
// placed explicitly relative to the PERSON's own, which is the offset the
// resolver's bound actually reads. Expressed as an offset rather than an
// absolute instant, and computed by Postgres from the person row, so the
// fixture is written by the same clock the assertion is about — a test that
// stamped these from the test host would be proving something about its own
// machine.
func (e *resolveEnv) seedTaskLinkedToPersonAt(t *testing.T, source, capturedBy string, person ids.UUID, fromPersonCreation time.Duration) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, due_at, source, captured_by, created_at)
		VALUES ($1, 'task', 'Follow up with the new lead', now(), now() + interval '1 day', $2, $3,
			(SELECT p.created_at FROM person p WHERE p.id = $4) + make_interval(secs => $5))`,
		id, source, capturedBy, person, fromPersonCreation.Seconds())
	e.exec(t, `INSERT INTO activity_link (activity_id, entity_type, person_id) VALUES ($1, 'person', $2)`, id, person)
	return id
}

// leadPromotedEvent is the real shape people.QualifyLead emits — the payload
// names the person the lead became, which is the only place, once
// carryLeadActivities has run, that a caller can still learn where the lead's
// tasks went. dedupeOutcome is "created" for a fresh person, "merged" for an
// existing survivor — see decodeLeadPromoted's doc for why the resolver reads
// it. OccurredAt is the API host's own stamp and the resolver's bound no
// longer reads it — TestAPromotedLeadCompletesItsCarriedTaskWhenTheHostClockTrails
// is the test that holds that, by moving this field and expecting no effect.
func leadPromotedEvent(t *testing.T, lead, person ids.UUID, dedupeOutcome string) workflow.Event {
	t.Helper()
	payload, err := json.Marshal(map[string]string{
		"promoted_person_id": person.String(), "dedupe_outcome": dedupeOutcome, "trigger": "human_qualify",
	})
	if err != nil {
		t.Fatal(err)
	}
	return workflow.Event{
		ID:         ids.NewV7(),
		Type:       "lead.promoted",
		OccurredAt: time.Now(),
		Entity:     datasource.EntityRef{Type: "lead", ID: lead},
		Payload:    payload,
	}
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

// A lead DISQUALIFIED leaves the open pool without moving anything: it never
// becomes a person, so its follow-up task's link keeps its lead_id, and the
// resolver's original lead-keyed lookup still finds it.
func TestADisqualifiedLeadCompletesItsSystemTasks(t *testing.T) {
	e := setupResolve(t)
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	systemTask := e.seedTask(t, "system", "system")

	ctx := e.systemCtx()
	h := handlerFor(t, store, "lead.disqualified")
	fire(ctx, t, h, workflow.Event{
		ID:         ids.NewV7(),
		Type:       "lead.disqualified",
		OccurredAt: time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC),
		Entity:     datasource.EntityRef{Type: "lead", ID: e.lead},
	})
	if !e.isDone(t, systemTask) {
		t.Error("the disqualified lead's system follow-up is still open — a reminder chasing a closed loop")
	}
}

// A lead PROMOTED is a different shape. carryLeadActivities
// (people/promote.go) moves the follow-up task's link from the lead onto the
// person it became — entity_type 'person', person_id set, lead_id NULL —
// inside the SAME transaction that emits lead.promoted, so the resolver never
// sees a lead-linked row for a genuinely promoted lead. It has to complete the
// task through the person the event names instead.
func TestAPromotedLeadCompletesItsSystemTasksCarriedToThePerson(t *testing.T) {
	e := setupResolve(t)
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	person := e.seedPerson(t)
	systemTask := e.seedTaskLinkedToPerson(t, "system", "system", person)
	humanTask := e.seedTaskLinkedToPerson(t, "web", "human:"+e.rep.String(), person)

	ctx := e.systemCtx()
	h := handlerFor(t, store, "lead.promoted")
	result := fire(ctx, t, h, leadPromotedEvent(t, e.lead, person, "created"))

	if len(result.Applied) == 0 {
		t.Fatal("a promoted lead resolved nothing — the follow-up loop stays open forever")
	}
	if !e.isDone(t, systemTask) {
		t.Error("the promoted lead's system follow-up is still open — its task carried to the person, and the resolver looked it up by the lead id promotion just nulled")
	}
	if e.isDone(t, humanTask) {
		t.Error("the HUMAN's task was completed — the system claimed work a person may not consider done")
	}
}

// A lead promoted into an EXISTING person (dedupe_outcome "merged") must not
// claim work the person's own history already carries. promoteTarget only
// merges into a survivor that could easily have its own open system-minted
// reminder already (no_activity_reminder/check_in_cadence anchor on a person
// the same way) — completing every open system task on the person, rather
// than only the one this promotion carried, would tick off a reminder this
// promotion has nothing to do with, with an audit row claiming the follow-up
// happened. The resolver only completes the carried task on a genuinely NEW
// person, where nothing else could exist to collide with yet.
func TestAMergedPromotionDoesNotClaimThePersonsUnrelatedTasks(t *testing.T) {
	e := setupResolve(t)
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	person := e.seedPerson(t)
	// Pre-existing, unrelated to this promotion: a check-in reminder the
	// system minted against the SURVIVOR long before this lead ever promoted.
	unrelatedReminder := e.seedTaskLinkedToPerson(t, "system", "system", person)

	ctx := e.systemCtx()
	h := handlerFor(t, store, "lead.promoted")
	fire(ctx, t, h, leadPromotedEvent(t, e.lead, person, "merged"))

	if e.isDone(t, unrelatedReminder) {
		t.Error("a merge completed a person's pre-existing system reminder that this promotion never touched")
	}
}

// A person created by THIS promotion is not immune to the same collision:
// the resolver runs off the outbox, asynchronously, and anything that mints
// a system task against the freshly-created person in the window between the
// promotion committing and this handler actually running — no_activity_reminder,
// check_in_cadence — is not the follow-up this promotion carried. Bounding
// completion to the tasks that existed when the person did is what keeps "a
// fresh person cannot yet carry anything else" true instead of merely assumed.
func TestAPromotedLeadDoesNotCompleteATaskMintedAfterThePromotion(t *testing.T) {
	e := setupResolve(t)
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	person := e.seedPerson(t)
	carried := e.seedTaskLinkedToPerson(t, "system", "system", person)
	// A task minted a second AFTER the person the promotion created — the
	// exact shape of a sibling automation catching up before this handler runs.
	lateTask := e.seedTaskLinkedToPersonAt(t, "system", "system", person, time.Second)

	ctx := e.systemCtx()
	h := handlerFor(t, store, "lead.promoted")
	fire(ctx, t, h, leadPromotedEvent(t, e.lead, person, "created"))

	if !e.isDone(t, carried) {
		t.Error("the task actually carried by the promotion is still open")
	}
	if e.isDone(t, lateTask) {
		t.Error("a task minted AFTER the promotion was completed as if the promotion carried it")
	}
}

// The bound is read from Postgres on both sides, so the clock of the host
// running the handler cannot move it. That is the whole point: the API and
// worker hosts stamp workflow.Event.OccurredAt from their own time.Now(),
// activity.created_at is defaulted by the database, and the two drift
// independently.
//
// A host trailing the database by more than the gap between a follow-up
// task's creation and the promotion used to put the carried task on the far
// side of the bound. The resolver then completed nothing, returned no error
// and logged no anomaly, and the follow-up loop the system opened stayed open
// forever — the silent direction, which is why an hour of skew is asserted
// here rather than left to a deployment's luck.
func TestAPromotedLeadCompletesItsCarriedTaskWhenTheHostClockTrails(t *testing.T) {
	e := setupResolve(t)
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	person := e.seedPerson(t)
	// Carried by the promotion: minted against the lead a minute before the
	// person existed, well inside an hour of skew.
	carried := e.seedTaskLinkedToPersonAt(t, "system", "system", person, -time.Minute)

	ev := leadPromotedEvent(t, e.lead, person, "created")
	ev.OccurredAt = ev.OccurredAt.Add(-time.Hour)

	ctx := e.systemCtx()
	fire(ctx, t, handlerFor(t, store, "lead.promoted"), ev)

	if !e.isDone(t, carried) {
		t.Error("the carried follow-up is still open because the handler's host clock trails the database's — a loop the system opened and can no longer close")
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
// writes nothing whatever the completion path does.
func TestAFiringThatSelectedBeforeSomebodyElseFinishedWritesNothing(t *testing.T) {
	e := setupResolve(t)
	task := e.seedTask(t, "system", "system")

	firing := e.startBlockedFiring(t, task)
	// The winner: the task is completed and its version moves, under the lock
	// the firing is waiting on.
	firing.exec(`UPDATE activity SET is_done = true, done_at = now(), version = version + 1 WHERE id = $1`, task)
	got := firing.release()

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

// An ordinary edit that lands between the selection and the completion moves
// the version without finishing anything. The task still has to be completed:
// the loop that opened it has closed, and no later firing is promised for this
// lead — treating every version change as "somebody else did it" trades a noisy
// history for a follow-up that stays open forever.
func TestAnEditUnderTheFiringDoesNotLeaveTheTaskOpen(t *testing.T) {
	e := setupResolve(t)
	task := e.seedTask(t, "system", "system")

	firing := e.startBlockedFiring(t, task)
	// Not a completion: somebody retitled the task. The version moves and the
	// task stays open.
	firing.exec(`UPDATE activity SET subject = 'Follow up (renamed)', version = version + 1 WHERE id = $1`, task)
	got := firing.release()

	if got.err != nil {
		t.Fatalf("the firing failed instead of completing the task it selected: %v", got.err)
	}
	if got.completed != 1 {
		t.Errorf("the firing completed %d tasks, want the one it selected — an edit is not somebody else finishing it", got.completed)
	}
	if !e.isDone(t, task) {
		t.Fatal("the task is still open after the firing that was supposed to close it — a rename cost the follow-up its completion")
	}
}

// A task archived between the selection and the write is gone, not a failure.
// The row lock is taken live, so the completion answers not-found — and
// failing the firing on it would strand every OTHER task the same call
// selected, which is what the sibling below is here to catch.
func TestATaskArchivedUnderTheFiringIsNotAFailure(t *testing.T) {
	e := setupResolve(t)
	task := e.seedTask(t, "system", "system")
	sibling := e.seedTask(t, "system", "system")

	firing := e.startBlockedFiring(t, task)
	firing.exec(`UPDATE activity SET archived_at = now() WHERE id = $1`, task)
	got := firing.release()

	if got.err != nil {
		t.Fatalf("a task that went away failed the firing: %v", got.err)
	}
	if !e.isDone(t, sibling) {
		t.Fatal("the firing stopped at the archived task and left its sibling open")
	}
	// And the archived task itself is left alone. Completing it would write an
	// activity.updated event about a record that is no longer on any timeline,
	// which is the opposite of the skip this branch exists for.
	if e.isDone(t, task) {
		t.Error("the firing completed a task that had been archived under it")
	}
	if events := e.completionEvents(t, task); events != 0 {
		t.Errorf("the firing wrote %d activity.updated event(s) for an archived task", events)
	}
	if got.completed != 1 {
		t.Errorf("the firing reports %d completions, want only the sibling", got.completed)
	}
}

// blockedFiring is the shape all three race cases are: hold the task's row,
// start a firing that gets past its SELECT and stops at its write, act under
// the lock, then release and read what the firing did.
//
// One harness rather than three copies, because what makes these tests mean
// anything is the ORDERING — and the ordering is the part that took two
// corrections to get right (a waiter shows neither as an ungranted relation
// lock nor as an `active` backend). A change to it has to land in one place.
type blockedFiring struct {
	t        *testing.T
	blocking pgx.Tx
	done     chan firingResult
}

func (e *resolveEnv) startBlockedFiring(t *testing.T, task ids.UUID) *blockedFiring {
	t.Helper()
	blocking, err := e.owner.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := blocking.Exec(context.Background(),
		`SELECT id FROM activity WHERE id = $1 FOR UPDATE`, task); err != nil {
		t.Fatal(err)
	}

	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	ctx := e.systemCtx()
	done := make(chan firingResult, 1)
	go func() {
		completed, err := store.CompleteOpenSystemTasksForLead(ctx, ids.From[ids.LeadKind](e.lead))
		done <- firingResult{completed, err}
	}()

	b := &blockedFiring{t: t, blocking: blocking, done: done}
	// However the test ends, the lock is released and the firing is waited
	// for: a goroutine still holding a pooled connection outlives the test,
	// and the next one resets the database under it.
	t.Cleanup(func() {
		//craft:ignore swallowed-errors the lock is normally released by release() and this rollback is then a designed no-op; on the failure paths it is the release itself, and a failing release could tell this test nothing its own assertions have not
		_ = blocking.Rollback(context.Background())
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("the firing never returned after the lock was released")
		}
	})

	e.waitForBlockedWriter(t, done)
	return b
}

// exec runs one statement UNDER the held lock — what the winner of the race
// did before this firing could reach its write.
func (b *blockedFiring) exec(sql string, args ...any) {
	b.t.Helper()
	if _, err := b.blocking.Exec(context.Background(), sql, args...); err != nil {
		b.t.Fatal(err)
	}
}

// release commits the blocking transaction and answers what the firing then
// did. The result is put back for the cleanup above, so both can read it.
func (b *blockedFiring) release() firingResult {
	b.t.Helper()
	if err := b.blocking.Commit(context.Background()); err != nil {
		b.t.Fatal(err)
	}
	var got firingResult
	select {
	case got = <-b.done:
	case <-time.After(10 * time.Second):
		b.t.Fatal("the firing never returned after the lock was released")
	}
	b.done <- got
	return got
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
		// Blocked BY US specifically, through pg_blocking_pids: any other
		// waiter in this database would otherwise release the wait early and
		// the test would go on to assert about a race it never set up.
		//
		// A waiter on a ROW lock does not appear as an ungranted lock on the
		// relation — it waits on the holder's transaction id — and its
		// `state` reads "idle in transaction" rather than "active", so
		// neither of those is what identifies it.
		if err := e.owner.QueryRow(context.Background(),
			`SELECT count(*) FROM pg_stat_activity
			  WHERE datname = current_database()
			    AND pg_backend_pid() = ANY(pg_blocking_pids(pid))`).Scan(&blocked); err != nil {
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
