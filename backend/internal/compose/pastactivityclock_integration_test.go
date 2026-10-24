// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// last_activity_at counts only what has happened. A booked meeting ahead does
// not move the deal, contact or company clock until its start passes. Then the
// past_activity_clock_refold worker moves it, however long it was down.
// Activities land through LogActivity. Time passes on a pool whose now() is
// shifted, so the clock functions and the refold window see one instant.

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// shiftedPool answers an app-role pool whose now() is the real one plus shift.
// Its own schema holds the shifted now() ahead of pg_catalog's on this pool's
// search path only, so no other connection's clock moves.
func shiftedPool(t *testing.T, shift time.Duration) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(ctx, `
		CREATE SCHEMA IF NOT EXISTS clockshift_test;
		GRANT USAGE ON SCHEMA clockshift_test TO PUBLIC;
		CREATE OR REPLACE FUNCTION clockshift_test.now() RETURNS timestamptz LANGUAGE sql STABLE AS $$
			SELECT pg_catalog.now() + coalesce(nullif(current_setting('margince_test.clock_shift', true), ''), '0')::interval
		$$;`); err != nil {
		t.Fatalf("installing the shifted clock: %v", err)
	}
	cfg, err := database.PoolConfig(os.Getenv("MARGINCE_TEST_APP_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = `"$user", public, clockshift_test, pg_catalog`
	cfg.ConnConfig.RuntimeParams["margince_test.clock_shift"] = shift.String()
	pool, err := testdb.OwnPoolFromConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var moved time.Time
	if err := pool.QueryRow(ctx, `SELECT now() - pg_catalog.now()`).Scan(new(time.Duration)); err != nil {
		t.Fatalf("the shifted pool cannot read its clock: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT now()`).Scan(&moved); err != nil || moved.Before(clocktest.Now(t).Add(shift-time.Minute)) {
		t.Fatalf("the shifted pool reads now() = %v (%v), want about %v ahead", moved, err, shift)
	}
	return pool
}

// runRefold runs the production worker once, on the given pool.
func runRefold(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	worker := &pastActivityClockWorker{pool: pool, identity: identity.NewService(pool), log: slog.New(slog.DiscardHandler)}
	if err := worker.Work(context.Background(), &river.Job[PastActivityClockRefoldArgs]{}); err != nil {
		t.Fatalf("running the past-activity clock refold: %v", err)
	}
}

// accountFixture is a company, one contact it employs and one of its deals.
type accountFixture struct {
	clockFixture
	company, contact, deal ids.UUID
}

func newAccountFixture(t *testing.T) accountFixture {
	t.Helper()
	f := accountFixture{clockFixture: newClockFixture(t)}
	f.company = f.e.SeedCompany(t, "Past Clock Co", nil)
	companyID := ids.From[ids.CompanyKind](f.company)
	f.contact = employedAt(t, f.e, f.company, "Pia Past")
	f.deal = f.newDeal(t, "Past clock deal", &companyID)
	return f
}

// log files one activity on the contact and the deal.
func (f accountFixture) log(t *testing.T, kind string, at time.Time) {
	t.Helper()
	subject := "Past clock " + kind
	in := activities.LogActivityInput{
		Kind: kind, Subject: &subject, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{onRecord("contact", f.contact), onRecord("deal", f.deal)},
	}
	if kind == "meeting" {
		in.MeetingStatus = new("booked")
	}
	if kind == "email" {
		in.Direction = new("inbound")
	}
	if _, _, err := f.e.Activities.LogActivity(f.e.Admin(), in); err != nil {
		t.Fatalf("logging the %s: %v", kind, err)
	}
}

// assertClocks holds the contact, company and deal clock to want.
func (f accountFixture) assertClocks(t *testing.T, want time.Time, why string) {
	t.Helper()
	contact, err := f.e.Contacts.GetContact(f.e.Admin(), ids.From[ids.ContactKind](f.contact), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	company, err := f.e.Contacts.GetCompany(f.e.Admin(), ids.From[ids.CompanyKind](f.company), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, clock := range []struct {
		record string
		at     *time.Time
	}{{"contact", contact.LastActivityAt}, {"company", company.LastActivityAt}, {"deal", f.dealClock(t, f.deal)}} {
		if clock.at == nil || !clock.at.Equal(want) {
			t.Errorf("%s last_activity_at = %v, want %v: %s", clock.record, clock.at, want, why)
		}
	}
}

func TestAFutureMeetingIsNotTheLastActivity(t *testing.T) {
	f := newAccountFixture(t)
	mailed, meetsAt := f.now.AddDate(0, 0, -3), f.now.AddDate(0, 0, 7)
	f.log(t, "email", mailed)
	f.log(t, "meeting", meetsAt)
	f.assertClocks(t, mailed, "a meeting next week has not happened")

	runRefold(t, f.e.Pool)
	f.assertClocks(t, mailed, "a refold before the meeting starts must not count it")

	runRefold(t, shiftedPool(t, 8*24*time.Hour))
	f.assertClocks(t, meetsAt, "once the meeting has started, the refold counts it")
}

// The worker did not run for days around the meeting's start. The mark is
// where the last run left it, so the first run after the outage still reaches it.
func TestAMeetingThatStartedDuringAnOutageStillMovesTheClock(t *testing.T) {
	f := newAccountFixture(t)
	mailed, meetsAt := f.now.AddDate(0, 0, -3), f.now.AddDate(0, 0, 1)
	f.log(t, "email", mailed)
	f.log(t, "meeting", meetsAt)

	runRefold(t, shiftedPool(t, 4*24*time.Hour))
	f.assertClocks(t, meetsAt, "a meeting that started three days before the next run")
}

// clockRow is what a rewrite would change on a record even when its value stands.
type clockRow struct {
	xmin, version int64
	updatedAt     time.Time
}

func readClockRow(t *testing.T, e *integration.Env, table string, id ids.UUID) clockRow {
	t.Helper()
	var row clockRow
	if err := e.Pool.QueryRow(context.Background(),
		`SELECT xmin::text::bigint, version, updated_at FROM `+pgx.Identifier{table}.Sanitize()+` WHERE id = $1`, id).
		Scan(&row.xmin, &row.version, &row.updatedAt); err != nil {
		t.Fatal(err)
	}
	return row
}

func TestASecondRefoldWritesNothing(t *testing.T) {
	f := newAccountFixture(t)
	f.log(t, "email", f.now.AddDate(0, 0, -3))
	meetsAt := f.now.AddDate(0, 0, 2)
	f.log(t, "meeting", meetsAt)
	later := shiftedPool(t, 3*24*time.Hour)
	runRefold(t, later)
	f.assertClocks(t, meetsAt, "the first refold moves the clocks")

	before := map[string]clockRow{}
	for table, id := range map[string]ids.UUID{"contact": f.contact, "company": f.company, "deal": f.deal} {
		before[table] = readClockRow(t, f.e, table, id)
	}
	// The mark goes back a day, so this run reads the meeting again. A second
	// pass over the same records must rewrite none of them.
	f.e.WsExec(t, `UPDATE activity_clock_refold_mark SET passed_up_to = now() - interval '1 day'`)
	runRefold(t, later)
	for table, id := range map[string]ids.UUID{"contact": f.contact, "company": f.company, "deal": f.deal} {
		if after := readClockRow(t, f.e, table, id); after != before[table] {
			t.Errorf("%s: the second refold rewrote the row (%+v → %+v), want it untouched", table, before[table], after)
		}
	}
}

// Two refolds that reach the same records, and a write filed under them, run
// at once. Each locks its whole set first, in the order every writer shares.
func TestOverlappingRefoldsAndAWriteDoNotDeadlock(t *testing.T) {
	f := newAccountFixture(t)
	second := employedAt(t, f.e, f.company, "Quinn Past")
	ctx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), f.e.WS), pastActivityClockActor)
	sets := []storekit.LastActivityTargets{
		{Contacts: []ids.UUID{f.contact, second}, Deals: []ids.UUID{f.deal}},
		{Contacts: []ids.UUID{second, f.contact}, Companies: []ids.UUID{f.company}, Deals: []ids.UUID{f.deal}},
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2*len(sets)*10+10)
	for round := range 10 {
		for _, set := range sets {
			wg.Go(func() {
				errs <- f.e.DB().Tx(ctx, func(tx pgx.Tx) error { return refoldClocksTx(ctx, tx, set) })
			})
		}
		wg.Go(func() {
			at := f.now.AddDate(0, 0, -round)
			subject := "Concurrent note"
			_, _, err := f.e.Activities.LogActivity(f.e.Admin(), activities.LogActivityInput{
				Kind: "note", Subject: &subject, OccurredAt: &at, Source: "manual",
				Links: []activities.ActivityLinkInput{onRecord("contact", second), onRecord("deal", f.deal)},
			})
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("a refold or write running alongside the others failed: %v", err)
		}
	}
}

func TestTheDataTaskMovesBackAClockTheOldRuleLeftInTheFuture(t *testing.T) {
	f := newAccountFixture(t)
	mailed := f.now.AddDate(0, 0, -3)
	f.log(t, "email", mailed)
	f.log(t, "meeting", f.now.AddDate(0, 0, 9))
	// The rule before 1792656701 stored the meeting's start; this is that row.
	for _, table := range []string{"contact", "company", "deal"} {
		f.e.WsExec(t, `UPDATE `+pgx.Identifier{table}.Sanitize()+` SET last_activity_at = now() + interval '9 days'
			WHERE id = ANY($1)`, []ids.UUID{f.contact, f.company, f.deal})
	}

	task := pastActivityClocksDataTask()
	ctx := datatask.Acting(principal.WithWorkspaceID(context.Background(), f.e.WS), task.ID)
	runTask := func() int {
		changed := 0
		for _, step := range task.Steps {
			err := f.e.DB().Tx(ctx, func(tx pgx.Tx) error {
				out, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: step.Limit})
				changed += out.Changed
				return err
			})
			if err != nil {
				t.Fatalf("running step %s: %v", step.Name, err)
			}
		}
		return changed
	}
	if got := runTask(); got == 0 {
		t.Error("the data task changed no record, want the clocks dated in the future moved")
	}
	f.assertClocks(t, mailed, "a stored clock in the future goes back to what has happened")
	if got := runTask(); got != 0 {
		t.Errorf("a second run changed %d records, want none", got)
	}
}
