// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build poc

package compose

// Proof of concept for pushed Worklist kinds, run against a seeded database
// (make bench-daily leaves one):
//
//	POC_OWNER_DSN=... POC_APP_DSN=... go test -tags poc -run TestWorklistFeedPoC -v ./internal/compose
//
// A holder's producer runs today's lane as the holder over the WHOLE set and
// writes work_item under a lock-ordered generation. Tasks are read back through
// the activities module's own rule restricted to the stored ids; waiting
// customers through the discover gate the lane uses. Each case below states
// what it proves.

import (
	"context"
	"fmt"
	"os"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/attention"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/worklist"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

const pocDDL = `
DROP TABLE IF EXISTS work_item;
DROP SEQUENCE IF EXISTS work_item_gen;
CREATE SEQUENCE work_item_gen;
CREATE TABLE work_item (
  kind text NOT NULL,
  subject_id uuid NOT NULL,
  holder_id uuid NOT NULL,
  sort_at timestamptz,
  gen bigint NOT NULL,
  open boolean NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (kind, subject_id, holder_id));
CREATE INDEX work_item_open ON work_item (holder_id, kind, sort_at) WHERE open;
CREATE INDEX work_item_subject ON work_item (kind, subject_id) WHERE open;
GRANT SELECT, INSERT, UPDATE, DELETE ON work_item TO margince_app;
GRANT USAGE ON SEQUENCE work_item_gen TO margince_app;`

const (
	kindTask    = "task"
	kindWaiting = "customer_waiting"
)

type pocMember struct {
	id  ids.UUID
	ctx context.Context
}

type pocEnv struct {
	owner, app *pgxpool.Pool
	asOf       time.Time
	window     time.Time
	lanes      pocLanes
	store      *worklist.Store
	members    []pocMember
}

func pocPools(t *testing.T) (owner, app *pgxpool.Pool) {
	t.Helper()
	open := func(env string) *pgxpool.Pool {
		dsn := os.Getenv(env)
		if dsn == "" {
			t.Fatalf("%s is not set", env)
		}
		pool, err := database.NewPool(context.Background(), dsn)
		if err != nil {
			t.Fatalf("opening %s: %v", env, err)
		}
		t.Cleanup(pool.Close)
		return pool
	}
	return open("POC_OWNER_DSN"), open("POC_APP_DSN")
}

func pocMembers(t *testing.T, owner, app *pgxpool.Pool) []pocMember {
	t.Helper()
	ctx := context.Background()
	var ws ids.UUID
	if err := owner.QueryRow(ctx, `SELECT id FROM workspace LIMIT 1`).Scan(&ws); err != nil {
		t.Fatalf("workspace: %v", err)
	}
	var userIDs []ids.UUID
	if err := owner.QueryRow(ctx, `SELECT array_agg(id ORDER BY id) FROM app_user WHERE removed_at IS NULL`).Scan(&userIDs); err != nil {
		t.Fatalf("members: %v", err)
	}
	authority := identity.NewService(app)
	base := principal.WithWorkspaceID(ctx, ws)
	var out []pocMember
	for _, id := range userIDs {
		p, err := authz.MemberPrincipal(base, authority, ws, id)
		if err != nil {
			continue // no seat: nobody whose list this proof can read
		}
		out = append(out, pocMember{id: id, ctx: principal.WithActor(base, p)})
	}
	return out
}

type pocLanes struct {
	tasks   attentionTasks
	waiting attentionWaiting
}

func newPocLanes(app *pgxpool.Pool, now attention.Clock) pocLanes {
	db := InstallationDB(app)
	return pocLanes{
		tasks: attentionTasks{store: activities.NewStore(db), claims: newContactsStore(db), db: db},
		waiting: attentionWaiting{
			store: activities.NewStore(db).WithOwnDomains(
				ownDomainReader{store: capture.NewOwnDomainStore(db)}),
			deals:    deals.NewStore(db, DealsInstallation()),
			contacts: newContactsStore(db),
			now:      now,
		},
	}
}

// pocAllTasks reads every task an input admits, page by page on the store's
// own keyset, because one read stops at the store's page cap.
func (e *pocEnv) pocAllTasks(ctx context.Context, in activities.ListActivitiesInput) ([]crmcontracts.Activity, error) {
	var out []crmcontracts.Activity
	limit := 200
	in.Limit = &limit
	for page := 0; page < 1000; page++ {
		var rows []crmcontracts.Activity
		var next storekit.Page
		err := database.ReuseGenericPlans(ctx, func(ctx context.Context) error {
			var err error
			rows, _, next, err = e.lanes.tasks.store.ListActivitiesPositioned(ctx, in)
			return err
		})
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
		if !next.HasMore || next.NextCursor == "" {
			return out, nil
		}
		in.Cursor = &next.NextCursor
	}
	return nil, fmt.Errorf("task read did not end")
}

// laneTasks is today's task lane for the reader, uncapped, due by until.
func (e *pocEnv) laneTasks(m pocMember, until time.Time) ([]ids.UUID, error) {
	in, ok := openTasksDueBy(m.ctx, e.asOf, until, attention.TasksMine, ids.Nil, nil)
	if !ok {
		return nil, fmt.Errorf("no task scope for %s", m.id)
	}
	rows, err := e.pocAllTasks(m.ctx, in)
	out := make([]ids.UUID, 0, len(rows))
	for _, row := range rows {
		out = append(out, ids.UUID(row.Id))
	}
	return out, err
}

// laneWaiting is today's waiting lane for the reader, and whether it cut.
func (e *pocEnv) laneWaiting(m pocMember) ([]worklist.Item, bool, error) {
	waits, cut, err := e.lanes.waiting.Unanswered(m.ctx, e.asOf, attention.WaitingFor{Owner: &m.id})
	out := make([]worklist.Item, 0, len(waits))
	for _, wait := range waits {
		asked := wait.FirstAskedAt
		out = append(out, worklist.Item{Kind: kindWaiting, Subject: wait.ActivityID, Holder: m.id, SortAt: &asked})
	}
	return out, cut, err
}

// produce rebuilds one holder: the whole open task set and the waiting lane.
func (e *pocEnv) produce(m pocMember) (bool, error) {
	in, ok := openTasksDueBy(m.ctx, e.asOf, e.asOf.AddDate(50, 0, 0), attention.TasksMine, ids.Nil, nil)
	if !ok {
		return false, fmt.Errorf("no task scope for %s", m.id)
	}
	open, err := e.pocAllTasks(m.ctx, in)
	if err != nil {
		return false, err
	}
	tasks := make([]worklist.Item, 0, len(open))
	for _, task := range open {
		tasks = append(tasks, worklist.Item{Kind: kindTask, Subject: ids.UUID(task.Id), Holder: m.id, SortAt: task.DueAt})
	}
	if _, err := e.store.Rebuild(m.ctx, m.id, kindTask, tasks); err != nil {
		return false, err
	}
	waits, cut, err := e.laneWaiting(m)
	if err != nil {
		return false, err
	}
	_, err = e.store.Rebuild(m.ctx, m.id, kindWaiting, waits)
	return cut, err
}

// storeTasks is the pushed read: the stored candidates, judged by the
// activities module's own task rule now.
func (e *pocEnv) storeTasks(m pocMember, until time.Time) ([]ids.UUID, error) {
	candidates, err := e.store.Candidates(m.ctx, m.id, kindTask)
	if err != nil || len(candidates) == 0 {
		return nil, err
	}
	in, ok := openTasksDueBy(m.ctx, e.asOf, until, attention.TasksMine, ids.Nil, nil)
	if !ok {
		return nil, fmt.Errorf("no task scope for %s", m.id)
	}
	in.AmongIDs = candidates
	rows, err := e.pocAllTasks(m.ctx, in)
	out := make([]ids.UUID, 0, len(rows))
	for _, row := range rows {
		out = append(out, ids.UUID(row.Id))
	}
	return out, err
}

func (e *pocEnv) storeWaiting(m pocMember) ([]worklist.Item, bool, error) {
	items, _, more, err := e.store.Gated(m.ctx, m.id, kindWaiting, 1000)
	return items, more, err
}

func pocSorted(in []ids.UUID) []string {
	out := make([]string, 0, len(in))
	for _, id := range in {
		out = append(out, id.String())
	}
	sort.Strings(out)
	return out
}

func pocSubjectsOf(items []worklist.Item) []ids.UUID {
	out := make([]ids.UUID, 0, len(items))
	for _, it := range items {
		out = append(out, it.Subject)
	}
	return out
}

func pocMedian(runs int, fn func() error) (time.Duration, error) {
	took := make([]time.Duration, 0, runs)
	for range runs {
		start := time.Now()
		if err := fn(); err != nil {
			return 0, err
		}
		took = append(took, time.Since(start))
	}
	slices.Sort(took)
	return took[len(took)/2], nil
}

func TestWorklistFeedPoC(t *testing.T) {
	owner, app := pocPools(t)
	if _, err := owner.Exec(context.Background(), pocDDL); err != nil {
		t.Fatalf("ddl: %v", err)
	}
	asOf := time.Now().UTC()
	e := &pocEnv{
		owner: owner, app: app, asOf: asOf, window: asOf.AddDate(0, 0, 15),
		lanes: newPocLanes(app, func() time.Time { return asOf }),
		store: worklist.NewStore(app),
	}
	e.members = pocMembers(t, owner, app)

	t.Run("producer writes the whole set", func(t *testing.T) { pocProduce(t, e) })
	t.Run("pushed reads equal the uncapped lanes", func(t *testing.T) { pocParity(t, e) })
	t.Run("every row the real Worklist shows is in the store", func(t *testing.T) { pocWorklistPages(t, e) })
	t.Run("a task done with no event leaves at once", func(t *testing.T) { pocDoneWithoutEvent(t, e) })
	t.Run("a reassigned task moves when the router rebuilds both holders", func(t *testing.T) { pocReassign(t, e) })
	t.Run("an older generation cannot reopen or re-add", func(t *testing.T) { pocStaleGeneration(t, e) })
	t.Run("a restriction with no event leaks nothing", func(t *testing.T) { pocRestriction(t, e) })
	t.Run("other holders' rows do not slow a read", func(t *testing.T) { pocSize(t, e) })
}

func pocProduce(t *testing.T, e *pocEnv) {
	start := time.Now()
	cuts := 0
	for _, m := range e.members {
		cut, err := e.produce(m)
		if err != nil {
			t.Fatalf("producer for %s: %v", m.id, err)
		}
		if cut {
			cuts++
		}
	}
	took := time.Since(start)
	t.Logf("%d holders rebuilt in %v (%v each); waiting lane cut for %d holders, so their waiting count is a floor",
		len(e.members), took.Round(time.Millisecond), (took / time.Duration(max(1, len(e.members)))).Round(time.Millisecond), cuts)
}

func pocParity(t *testing.T, e *pocEnv) {
	t.Logf("%-12s %6s %8s %10s %10s %10s %10s", "member", "tasks", "waiting", "lane task", "push task", "lane wait", "push wait")
	for _, m := range e.members {
		var lane, pushed []ids.UUID
		var laneW, pushedW []worklist.Item
		laneTook, err := pocMedian(3, func() (err error) { lane, err = e.laneTasks(m, e.window); return })
		if err != nil {
			t.Fatal(err)
		}
		pushTook, err := pocMedian(3, func() (err error) { pushed, err = e.storeTasks(m, e.window); return })
		if err != nil {
			t.Fatal(err)
		}
		laneWTook, err := pocMedian(3, func() (err error) { laneW, _, err = e.laneWaiting(m); return })
		if err != nil {
			t.Fatal(err)
		}
		pushWTook, err := pocMedian(3, func() (err error) { pushedW, _, err = e.storeWaiting(m); return })
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(pocSorted(lane), pocSorted(pushed)) {
			t.Errorf("%s tasks: lane %d, pushed %d", m.id, len(lane), len(pushed))
		}
		if !slices.Equal(pocSorted(pocSubjectsOf(laneW)), pocSorted(pocSubjectsOf(pushedW))) {
			t.Errorf("%s waiting: lane %d, pushed %d", m.id, len(laneW), len(pushedW))
		}
		t.Logf("%-12s %6d %8d %10v %10v %10v %10v", m.id.String()[24:], len(lane), len(laneW),
			laneTook.Round(time.Millisecond), pushTook.Round(time.Millisecond),
			laneWTook.Round(time.Millisecond), pushWTook.Round(time.Millisecond))
	}
	t.Log("task columns: both return hydrated activities under the same module rule; waiting columns: the lane hydrates cards, the pushed read returns gated ids only")
}

func pocWorklistPages(t *testing.T, e *pocEnv) {
	service := newAttentionService(e.app, NewApprovalsService(InstallationDB(e.app)), func() time.Time { return e.asOf })
	checked, missing, skipped := 0, 0, map[string]int{}
	for _, m := range e.members[:min(5, len(e.members))] {
		tasks, err := e.store.Candidates(m.ctx, m.id, kindTask)
		if err != nil {
			t.Fatal(err)
		}
		waits, err := e.store.Candidates(m.ctx, m.id, kindWaiting)
		if err != nil {
			t.Fatal(err)
		}
		held := map[string]bool{}
		for _, id := range append(tasks, waits...) {
			held[id.String()] = true
		}
		for _, filter := range []string{"tasks", "upcoming", "customer_waiting"} {
			cursor := ""
			for page := 0; page < 50; page++ {
				reading := principal.WithCorrelationID(m.ctx, ids.UUID(uuid.Must(uuid.NewV7())))
				out, err := service.WorklistView(reading, "today", "mine", filter, ids.Nil, 100, cursor)
				if err != nil {
					t.Fatalf("%s %s: %v", m.id, filter, err)
				}
				for _, row := range out.Queue {
					if row.Source != kindTask && row.Source != kindWaiting {
						skipped[string(row.Source)]++
						continue
					}
					checked++
					if !held[row.Id] {
						missing++
						t.Errorf("%s: the Worklist shows %s %s, the store does not hold it", m.id, row.Source, row.Id)
					}
				}
				if out.NextCursor == nil || *out.NextCursor == "" {
					break
				}
				cursor = *out.NextCursor
			}
		}
	}
	t.Logf("walked every page of tasks, upcoming and customer_waiting for 5 members: %d task/waiting rows checked, %d missing from the store; other row sources not compared: %v",
		checked, missing, skipped)
}

// pocOwnerExec runs one statement as the owner with the user triggers paused,
// because this throwaway corpus lacks the evidence real writers record.
func pocOwnerExec(t *testing.T, e *pocEnv, statement string, args ...any) {
	t.Helper()
	ctx := context.Background()
	tx, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }() //craft:ignore T2 rollback after commit is a no-op
	for i, sql := range []string{`ALTER TABLE activity DISABLE TRIGGER USER`, statement, `ALTER TABLE activity ENABLE TRIGGER USER`} {
		var bound []any
		if i == 1 {
			bound = args
		}
		if _, err := tx.Exec(ctx, sql, bound...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func pocFirstTaskHolder(t *testing.T, e *pocEnv) (pocMember, ids.UUID) {
	t.Helper()
	for _, m := range e.members {
		tasks, err := e.storeTasks(m, e.window)
		if err != nil {
			t.Fatal(err)
		}
		if len(tasks) > 0 {
			return m, tasks[0]
		}
	}
	t.Fatal("no member holds a task in the window")
	return pocMember{}, ids.Nil
}

func pocDoneWithoutEvent(t *testing.T, e *pocEnv) {
	m, task := pocFirstTaskHolder(t, e)
	pocOwnerExec(t, e, `UPDATE activity SET is_done = true, done_at = now() WHERE id = $1`, task)
	t.Cleanup(func() { pocOwnerExec(t, e, `UPDATE activity SET is_done = false, done_at = NULL WHERE id = $1`, task) })
	pushed, err := e.storeTasks(m, e.window)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(pushed, task) {
		t.Errorf("the done task is still listed with no rebuild")
	}
	candidates, err := e.store.Candidates(m.ctx, m.id, kindTask)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("done with no event and no rebuild: still a stored candidate %v, listed %v", slices.Contains(candidates, task), slices.Contains(pushed, task))
}

func pocReassign(t *testing.T, e *pocEnv) {
	from, task := pocFirstTaskHolder(t, e)
	var to pocMember
	for _, m := range e.members {
		if m.id != from.id {
			to = m
			break
		}
	}
	pocOwnerExec(t, e, `UPDATE activity SET assignee_id = $2 WHERE id = $1`, task, to.id)
	t.Cleanup(func() {
		pocOwnerExec(t, e, `UPDATE activity SET assignee_id = $2 WHERE id = $1`, task, from.id)
		for _, m := range []pocMember{from, to} {
			if _, err := e.produce(m); err != nil {
				t.Error(err)
			}
		}
	})
	listed := func(m pocMember) bool {
		pushed, err := e.storeTasks(m, e.window)
		if err != nil {
			t.Fatal(err)
		}
		return slices.Contains(pushed, task)
	}
	beforeFrom, beforeTo := listed(from), listed(to)
	// The router: activity.updated names the record, never who held it, so the
	// holders it is leaving come from the store and the one it joins from live.
	holders, err := e.store.HoldersOf(from.ctx, kindTask, []ids.UUID{task})
	if err != nil {
		t.Fatal(err)
	}
	holders = append(holders, to.id)
	for _, m := range e.members {
		if slices.Contains(holders, m.id) {
			if _, err := e.produce(m); err != nil {
				t.Fatal(err)
			}
		}
	}
	afterFrom, afterTo := listed(from), listed(to)
	if afterFrom || !afterTo {
		t.Errorf("after the rebuild: old holder lists it %v, new holder %v", afterFrom, afterTo)
	}
	t.Logf("reassigned, before any rebuild: old holder lists it %v, new holder %v; after rebuilding the %d routed holders: old %v, new %v",
		beforeFrom, beforeTo, len(holders), afterFrom, afterTo)
}

func pocStaleGeneration(t *testing.T, e *pocEnv) {
	m, task := pocFirstTaskHolder(t, e)
	before, err := e.store.Candidates(m.ctx, m.id, kindTask)
	if err != nil {
		t.Fatal(err)
	}
	// A run that drew generation 1 and lost the race answers "nothing open".
	if err := e.store.RebuildAt(m.ctx, m.id, kindTask, nil, 1); err != nil {
		t.Fatal(err)
	}
	after, err := e.store.Candidates(m.ctx, m.id, kindTask)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Errorf("a stale empty run changed %d candidates to %d", len(before), len(after))
	}
	// A newer run settles everything; a stale run that still carries the task
	// must not bring it back, because the settled row stays as a tombstone.
	if _, err := e.store.Rebuild(m.ctx, m.id, kindTask, nil); err != nil {
		t.Fatal(err)
	}
	if err := e.store.RebuildAt(m.ctx, m.id, kindTask, []worklist.Item{{Kind: kindTask, Subject: task, Holder: m.id}}, 2); err != nil {
		t.Fatal(err)
	}
	reopened, err := e.store.Candidates(m.ctx, m.id, kindTask)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(reopened, task) {
		t.Errorf("a stale run re-added a settled task")
	}
	if _, err := e.produce(m); err != nil {
		t.Fatal(err)
	}
	t.Logf("stale empty run kept %d of %d candidates; stale run carrying a settled task re-added it: %v",
		len(after), len(before), slices.Contains(reopened, task))
}

func pocRestriction(t *testing.T, e *pocEnv) {
	var restricted []ids.UUID
	for _, m := range e.members {
		tasks, err := e.store.Candidates(m.ctx, m.id, kindTask)
		if err != nil {
			t.Fatal(err)
		}
		waits, err := e.store.Candidates(m.ctx, m.id, kindWaiting)
		if err != nil {
			t.Fatal(err)
		}
		for i, id := range append(tasks, waits...) {
			if i%3 == 0 {
				restricted = append(restricted, id)
			}
		}
	}
	var classless []ids.UUID
	if err := e.owner.QueryRow(context.Background(),
		`SELECT coalesce(array_agg(id), '{}') FROM activity WHERE id = ANY($1) AND retention_class IS NULL`, restricted).Scan(&classless); err != nil {
		t.Fatal(err)
	}
	pocOwnerExec(t, e, `UPDATE activity SET restricted_at = now(), restricted_reason = 'poc', archived_at = now(),
		restricted_until = now() + interval '30 days',
		retention_class = coalesce(retention_class, 'commercial_correspondence'),
		retention_class_at = coalesce(retention_class_at, now()) WHERE id = ANY($1)`, restricted)
	t.Cleanup(func() {
		pocOwnerExec(t, e, `UPDATE activity SET restricted_at = NULL, restricted_reason = NULL, restricted_until = NULL, archived_at = NULL WHERE id = ANY($1)`, restricted)
		pocOwnerExec(t, e, `UPDATE activity SET retention_class = NULL, retention_class_at = NULL WHERE id = ANY($1)`, classless)
	})
	leaked, agreedTasks, agreedWaiting, extraWaiting := 0, 0, 0, 0
	for _, m := range e.members {
		lane, err := e.laneTasks(m, e.window)
		if err != nil {
			t.Fatal(err)
		}
		pushed, err := e.storeTasks(m, e.window)
		if err != nil {
			t.Fatal(err)
		}
		laneW, _, err := e.laneWaiting(m)
		if err != nil {
			t.Fatal(err)
		}
		pushedW, _, err := e.storeWaiting(m)
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range append(pushed, pocSubjectsOf(pushedW)...) {
			if slices.Contains(restricted, id) {
				leaked++
			}
		}
		if slices.Equal(pocSorted(lane), pocSorted(pushed)) {
			agreedTasks++
		}
		if slices.Equal(pocSorted(pocSubjectsOf(laneW)), pocSorted(pocSubjectsOf(pushedW))) {
			agreedWaiting++
		}
		laneHas := map[ids.UUID]bool{}
		for _, id := range pocSubjectsOf(laneW) {
			laneHas[id] = true
		}
		for _, id := range pocSubjectsOf(pushedW) {
			if !laneHas[id] {
				extraWaiting++ // pushed shows a wait the lane no longer does
			}
		}
	}
	if leaked > 0 || extraWaiting > 0 || agreedTasks != len(e.members) {
		t.Errorf("after restricting: %d leaked, %d waits the lane dropped, tasks equal for %d of %d", leaked, extraWaiting, agreedTasks, len(e.members))
	}
	t.Logf("restricted %d listed records, no event, no rebuild: %d leaked, %d waits shown that the lane dropped; tasks equal the lane for %d of %d members, waiting for %d of %d (the rest are waits new to the lane, which a rebuild adds)",
		len(restricted), leaked, extraWaiting, agreedTasks, len(e.members), agreedWaiting, len(e.members))
}

func pocSize(t *testing.T, e *pocEnv) {
	ctx := context.Background()
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO work_item (kind, subject_id, holder_id, sort_at, gen, open)
		SELECT 'task', a.id, gen_random_uuid(), a.due_at, 1, true FROM activity a CROSS JOIN generate_series(1, 20)`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(ctx, `ANALYZE work_item`); err != nil {
		t.Fatal(err)
	}
	var total int
	if err := e.owner.QueryRow(ctx, `SELECT count(*) FROM work_item`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	for _, m := range e.members[:min(3, len(e.members))] {
		took, err := pocMedian(5, func() error { _, err := e.storeTasks(m, e.window); return err })
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("pushed task read with %d rows in work_item: %v", total, took.Round(time.Millisecond))
	}
}
