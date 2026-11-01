// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build poc

package compose

// Proof of concept: two Worklist kinds pushed into work_item by a background
// producer that runs today's lane as the holder, then served from the store
// under a live read gate. It measures and compares; it changes no product path.
//
// Run against a seeded database (make bench-daily leaves one):
//   POC_OWNER_DSN=... POC_APP_DSN=... go test -tags poc -run TestWorklistFeedPoC -v ./internal/compose

import (
	"context"
	"fmt"
	"os"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/worklist"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

const pocDDL = `
CREATE TABLE IF NOT EXISTS work_item (
  kind text NOT NULL,
  subject_id uuid NOT NULL,
  holder_id uuid NOT NULL,
  sort_at timestamptz,
  version bigint NOT NULL,
  resolved_at timestamptz,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (kind, subject_id, holder_id));
CREATE INDEX IF NOT EXISTS work_item_open ON work_item (holder_id, kind, sort_at) WHERE resolved_at IS NULL;
GRANT SELECT, INSERT, UPDATE, DELETE ON work_item TO margince_app;
TRUNCATE work_item;`

type pocMember struct {
	id  ids.UUID
	ctx context.Context
}

type pocRow struct {
	member                 string
	laneTasks, laneWaiting int
	lane, read, count      time.Duration
	parity                 string
}

func pocPools(t *testing.T) (owner, app *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	open := func(env string) *pgxpool.Pool {
		dsn := os.Getenv(env)
		if dsn == "" {
			t.Fatalf("%s is not set", env)
		}
		pool, err := database.NewPool(ctx, dsn)
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
	rows, err := owner.Query(ctx, `SELECT id FROM app_user WHERE removed_at IS NULL ORDER BY id`)
	if err != nil {
		t.Fatalf("members: %v", err)
	}
	var userIDs []ids.UUID
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("member: %v", err)
		}
		userIDs = append(userIDs, id)
	}
	rows.Close()
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

// pocLanes reads the two kinds the way the Worklist reads them for "mine".
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

func (l pocLanes) read(ctx context.Context, me ids.UUID, asOf time.Time) (tasks, waiting []worklist.Item, err error) {
	open, err := l.tasks.OpenForViewer(ctx, asOf, asOf.Add(15*24*time.Hour), 500, attention.TasksMine, ids.Nil, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("tasks lane: %w", err)
	}
	for _, task := range open {
		tasks = append(tasks, worklist.Item{Kind: "task", Subject: task.ID, Holder: me, SortAt: task.DueAt})
	}
	waits, _, err := l.waiting.Unanswered(ctx, asOf, attention.WaitingFor{Owner: &me})
	if err != nil {
		return nil, nil, fmt.Errorf("waiting lane: %w", err)
	}
	for _, wait := range waits {
		asked := wait.FirstAskedAt
		waiting = append(waiting, worklist.Item{Kind: "customer_waiting", Subject: wait.ActivityID, Holder: me, SortAt: &asked})
	}
	return tasks, waiting, nil
}

func pocSubjects(items []worklist.Item) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Kind+":"+it.Subject.String())
	}
	sort.Strings(out)
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
	ctx := context.Background()
	if _, err := owner.Exec(ctx, pocDDL); err != nil {
		t.Fatalf("ddl: %v", err)
	}
	asOf := time.Now().UTC()
	lanes := newPocLanes(app, func() time.Time { return asOf })
	store := worklist.NewStore(app)
	members := pocMembers(t, owner, app)
	kinds := []string{"task", "customer_waiting"}

	// 1. Background producer: one run per holder, the lane rules unchanged.
	produced := map[ids.UUID][]worklist.Item{}
	var produceTotal time.Duration
	for _, m := range members {
		start := time.Now()
		tasks, waiting, err := lanes.read(m.ctx, m.id, asOf)
		if err != nil {
			t.Fatalf("producer for %s: %v", m.id, err)
		}
		if err := store.Replace(m.ctx, m.id, "task", tasks, 1); err != nil {
			t.Fatal(err)
		}
		if err := store.Replace(m.ctx, m.id, "customer_waiting", waiting, 1); err != nil {
			t.Fatal(err)
		}
		produceTotal += time.Since(start)
		produced[m.id] = append(tasks, waiting...)
	}
	t.Logf("producer: %d holders in %v (%v per holder)", len(members), produceTotal, produceTotal/time.Duration(max(1, len(members))))

	// 2. Serve: live lanes against the store read, per holder.
	var report []pocRow
	for _, m := range members {
		row := pocRow{member: m.id.String()[:8]}
		var laneTasks, laneWaiting []worklist.Item
		var err error
		row.lane, err = pocMedian(5, func() error {
			laneTasks, laneWaiting, err = lanes.read(m.ctx, m.id, asOf)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		row.laneTasks, row.laneWaiting = len(laneTasks), len(laneWaiting)
		var page worklist.Page
		row.read, err = pocMedian(5, func() error {
			page, err = store.Read(m.ctx, m.id, kinds, 1000)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		row.count, err = pocMedian(5, func() error {
			var n int
			return database.WithWorkspaceTx(m.ctx, app, func(tx pgx.Tx) error {
				return tx.QueryRow(m.ctx, `SELECT count(*) FROM work_item WHERE holder_id = $1 AND resolved_at IS NULL`, m.id).Scan(&n)
			})
		})
		if err != nil {
			t.Fatal(err)
		}
		want, got := pocSubjects(append(laneTasks, laneWaiting...)), pocSubjects(page.Items)
		row.parity = "same"
		if !slices.Equal(want, got) || page.Total != len(want) {
			row.parity = fmt.Sprintf("DIFF lane=%d store=%d total=%d", len(want), len(got), page.Total)
		}
		report = append(report, row)
	}
	t.Logf("%-9s %6s %8s %10s %10s %10s  %s", "member", "tasks", "waiting", "lane", "store", "count", "parity")
	for _, r := range report {
		t.Logf("%-9s %6d %8d %10v %10v %10v  %s", r.member, r.laneTasks, r.laneWaiting,
			r.lane.Round(time.Microsecond), r.read.Round(time.Microsecond), r.count.Round(time.Microsecond), r.parity)
	}

	// 3. A late producer run (older generation, empty answer) changes nothing.
	for _, m := range members {
		if len(produced[m.id]) == 0 {
			continue
		}
		if err := store.Replace(m.ctx, m.id, "task", nil, 0); err != nil {
			t.Fatal(err)
		}
		if err := store.Replace(m.ctx, m.id, "customer_waiting", nil, 0); err != nil {
			t.Fatal(err)
		}
		page, err := store.Read(m.ctx, m.id, kinds, 1000)
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != len(produced[m.id]) {
			t.Errorf("a stale run changed %s's list: %d items, want %d", m.id, page.Total, len(produced[m.id]))
		}
		t.Logf("version guard: stale empty run left %s with %d of %d items", m.id.String()[:8], page.Total, len(produced[m.id]))
		break
	}

	// 4. Size: other holders' rows must not slow one holder's read.
	if _, err := owner.Exec(ctx, `
		INSERT INTO work_item (kind, subject_id, holder_id, sort_at, version)
		SELECT 'task', a.id, gen_random_uuid(), a.due_at, 1
		  FROM activity a CROSS JOIN generate_series(1, 20)`); err != nil {
		t.Fatalf("inflating: %v", err)
	}
	if _, err := owner.Exec(ctx, `ANALYZE work_item`); err != nil {
		t.Fatal(err)
	}
	var rowsNow int
	if err := owner.QueryRow(ctx, `SELECT count(*) FROM work_item`).Scan(&rowsNow); err != nil {
		t.Fatal(err)
	}
	for _, m := range members[:min(3, len(members))] {
		took, err := pocMedian(5, func() error {
			_, err := store.Read(m.ctx, m.id, kinds, 50)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("store read with %d rows in work_item: %s %v", rowsNow, m.id.String()[:8], took.Round(time.Microsecond))
	}
}
