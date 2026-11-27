// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What ends a cached Home answer, asked of the cache the server builds.

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/compose/homecache"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// keep stores answer as reader's, stamped now.
func keep(ctx context.Context, t *testing.T, c *homecache.Cache, reader ids.UUID, read homecache.Read, walk *ids.UUID, view string) {
	t.Helper()
	stamp, err := c.Stamp(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := homecache.Store(ctx, c, reader, read, "home", stamp, walk, crmcontracts.Worklist{View: new(crmcontracts.WorklistView(view))}); err != nil {
		t.Fatal(err)
	}
}

func served(ctx context.Context, t *testing.T, c *homecache.Cache, reader ids.UUID, read homecache.Read) bool {
	t.Helper()
	_, _, hit, err := homecache.Lookup[crmcontracts.Worklist](ctx, c, reader, read, "home")
	if err != nil {
		t.Fatal(err)
	}
	return hit
}

func TestACachedHomeAnswerIsServedToItsReaderUntilTheFenceMoves(t *testing.T) {
	e := Setup(t)
	c := homecache.New(e.Pool)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	contact := e.SeedContact(t, "Fenced", &e.Rep1)

	keep(ctx, t, c, e.Rep1, homecache.Worklist, nil, "decisions")
	if got, _, hit, err := homecache.Lookup[crmcontracts.Worklist](ctx, c, e.Rep1, homecache.Worklist, "home"); err != nil || !hit || got.View == nil || *got.View != "decisions" {
		t.Fatalf("lookup = %v %+v (err %v), want the kept answer", hit, got, err)
	}
	if served(e.As(e.Rep2, []ids.UUID{e.Team1}, RepPerms), t, c, e.Rep2, homecache.Worklist) {
		t.Error("another reader was served the answer")
	}

	e.WsExec(t, `UPDATE contact SET owner_id = $1 WHERE id = $2`, e.Rep2, contact)
	if served(ctx, t, c, e.Rep1, homecache.Worklist) {
		t.Error("an answer kept before an owner change was served after it")
	}
}

// A read that began before its reader's write and stores after it must not
// pass as current. It stays marked behind the write.
func TestAnAnswerComputedBeforeItsReadersWriteIsNeverServedAsCurrent(t *testing.T) {
	e := Setup(t)
	c := homecache.New(e.Pool)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)

	stamp, err := c.Stamp(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Forget(ctx, []ids.UUID{e.Rep1}); err != nil {
		t.Fatal(err)
	}
	if err := homecache.Store(ctx, c, e.Rep1, homecache.Worklist, "home", stamp, nil, crmcontracts.Worklist{}); err != nil {
		t.Fatal(err)
	}
	if _, standing, hit, err := homecache.Lookup[crmcontracts.Worklist](ctx, c, e.Rep1, homecache.Worklist, "home"); err != nil || !hit || !standing.Written {
		t.Errorf("an answer stamped before the reader's write: hit %v, written %v (err %v); want kept and marked behind the write", hit, standing.Written, err)
	}

	keep(ctx, t, c, e.Rep1, homecache.Worklist, nil, "today")
	if _, standing, hit, err := homecache.Lookup[crmcontracts.Worklist](ctx, c, e.Rep1, homecache.Worklist, "home"); err != nil || !hit || standing.Written {
		t.Errorf("an answer stamped after the write: hit %v, written %v (err %v); want kept and current", hit, standing.Written, err)
	}
}

// Expiry writes nothing, so it bounds the answer's life instead.
func TestAnAnswerLivesNoLongerThanTheNextGrantToExpire(t *testing.T) {
	e := Setup(t)
	c := homecache.New(e.Pool)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	contact := e.SeedContact(t, "Shared briefly", &e.Rep2)
	e.WsExec(t, `INSERT INTO record_grant
		(record_type, record_id, subject_type, subject_id, access, granted_by, expires_at)
		VALUES ('contact', $1, 'user', $2, 'read', $3, now() + interval '90 seconds')`, contact, e.Rep1, e.AdminUser)

	keep(ctx, t, c, e.Rep1, homecache.Worklist, nil, "today")
	if n := e.WsCount(t, `SELECT count(*) FROM home_read_cache c, record_grant g
		WHERE c.reader_id = $1 AND g.record_id = $2 AND c.valid_until <= g.expires_at`, e.Rep1, contact); n != 1 {
		t.Errorf("the kept answer outlives the grant it was assembled under (%d rows bounded)", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM home_read_cache
		WHERE reader_id = $1 AND valid_until <= computed_at + make_interval(secs => $2)`,
		e.Rep1, (homecache.Life + homecache.StaleGrace).Seconds()); n != 1 {
		t.Errorf("the kept answer outlives homecache.Life and its grace (%d rows bounded)", n)
	}
}

// A snooze runs out without a write, so an answer that hid the snoozed item is
// not served past the moment it comes back.
func TestAnAnswerLivesNoLongerThanTheNextSnoozeToEnd(t *testing.T) {
	e := Setup(t)
	c := homecache.New(e.Pool)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, AdminPerms)
	pipeline, open, _ := DealFixture(t, e)
	owner := OwnerConn(t)
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO deal (pipeline_id, stage_id, name, owner_id, status, source, captured_by, amount_minor, currency, expected_close_date)
		VALUES ($1, $2, 'Snoozed deal', $3, 'open', 'manual', 'test', 6000000, 'EUR', now() + interval '5 days')`,
		pipeline, open, e.Rep1); err != nil {
		t.Fatal(err)
	}
	engine := briefs.NewBriefEngine(e.Pool, e.Contacts)
	// The cache judges validity by the database clock, so the snooze is dated by it.
	var now time.Time
	if err := owner.QueryRow(context.Background(), `SELECT now()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	run, err := engine.SnapshotRun(ctx, now)
	if err != nil || len(run.Items) == 0 {
		t.Fatalf("a run to snooze from: items=%d err=%v", len(run.Items), err)
	}
	until := now.Add(90 * time.Second)
	if _, err := engine.MarkSnoozed(ctx, run.Items[0].ID, values.ReopenOnTime, &until, nil, now, nil); err != nil {
		t.Fatal(err)
	}

	keep(ctx, t, c, e.Rep1, homecache.Worklist, nil, "today")
	if n := e.WsCount(t, `SELECT count(*) FROM home_read_cache c, brief_item bi
		WHERE c.reader_id = $1 AND bi.id = $2 AND c.valid_until <= bi.snoozed_until`, e.Rep1, run.Items[0].ID); n != 1 {
		t.Errorf("the kept answer outlives the snooze it hid (%d rows bounded)", n)
	}
}

// startWalk records a walk for reader that lives worklistsnap's four hours.
func startWalk(t *testing.T, e *Env, reader ids.UUID) ids.UUID {
	t.Helper()
	walk := ids.NewV7()
	e.WsExec(t, `INSERT INTO worklist_snapshot (id, reader_id, params_fingerprint, as_of, created_at, expires_at, buckets, rows)
		VALUES ($1, $2, 'home', now(), now(), now() + interval '4 hours', '{}', '[]')`, walk, reader)
	return walk
}

func TestACachedFirstPageSaysWhenTheWalkItStartedHasEnded(t *testing.T) {
	e := Setup(t)
	c := homecache.New(e.Pool)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	walk := startWalk(t, e, e.Rep1)

	keep(ctx, t, c, e.Rep1, homecache.Worklist, &walk, "today")
	if _, standing, hit, err := homecache.Lookup[crmcontracts.Worklist](ctx, c, e.Rep1, homecache.Worklist, "home"); err != nil || !hit || standing.WalkEnded {
		t.Fatalf("a page whose walk stands: hit %v, walk ended %v (err %v); want served with its walk", hit, standing.WalkEnded, err)
	}
	e.WsExec(t, `DELETE FROM worklist_snapshot WHERE id = $1`, walk)
	if _, standing, hit, err := homecache.Lookup[crmcontracts.Worklist](ctx, c, e.Rep1, homecache.Worklist, "home"); err != nil || !hit || !standing.WalkEnded {
		t.Errorf("a page whose walk is gone: hit %v, walk ended %v (err %v); want served and its walk ended", hit, standing.WalkEnded, err)
	}
}

// A walk ends after four hours and a day answer is kept for the day. Back from
// a longer break, the reader sees the kept day at once, paging without the walk.
func TestAKeptDayOutlivesTheWalkItsFirstPageStarted(t *testing.T) {
	e := Setup(t)
	c := homecache.New(e.Pool)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	walk := startWalk(t, e, e.Rep1)
	keep(ctx, t, c, e.Rep1, homecache.Worklist, &walk, "decisions")
	e.WsExec(t, `UPDATE home_read_cache SET computed_at = computed_at - interval '5 hours',
	    valid_until = valid_until - interval '5 hours' WHERE reader_id = $1`, e.Rep1)
	e.WsExec(t, `UPDATE worklist_snapshot SET created_at = created_at - interval '5 hours',
	    expires_at = expires_at - interval '5 hours' WHERE id = $1`, walk)

	reads, ended := 0, 0
	fresh := func(context.Context) (crmcontracts.Worklist, error) {
		reads++
		return crmcontracts.Worklist{View: new(crmcontracts.WorklistViewToday)}, nil
	}
	homeWalk := homecache.NoWalk[crmcontracts.Worklist]()
	homeWalk.Ended = func(page crmcontracts.Worklist) (crmcontracts.Worklist, error) {
		ended++
		return page, nil
	}
	got, stale, err := homecache.Serve(ctx, c, e.Rep1, homecache.Worklist, "home", fresh, homeWalk)
	if err != nil || !stale || got.View == nil || *got.View != "decisions" || ended != 1 {
		t.Fatalf("serve five hours on = view %v (stale %v, walk dropped %d times, err %v), want the kept day, stale, its walk dropped once",
			got.View, stale, ended, err)
	}
	c.Settle()
	if reads != 1 {
		t.Errorf("read again %d times behind the kept day, want once", reads)
	}

	refused := homecache.NoWalk[crmcontracts.Worklist]()
	refused.Ended = func(page crmcontracts.Worklist) (crmcontracts.Worklist, error) {
		return page, errors.New("the cursor cannot page on")
	}
	keep(ctx, t, c, e.Rep1, homecache.Worklist, &walk, "decisions")
	if got, _, err := homecache.Serve(ctx, c, e.Rep1, homecache.Worklist, "home", fresh, refused); err != nil ||
		got.View == nil || *got.View != crmcontracts.WorklistViewToday || reads != 2 {
		t.Errorf("a kept day that cannot drop its walk = %+v (reads %d, err %v), want a fresh read", got, reads, err)
	}
}

// Home may lag background work by its five minutes, so a machine's write ends
// neither answer.
func TestAMachineWriteLeavesTheCachedReceiptAndWorklist(t *testing.T) {
	e := Setup(t)
	c := homecache.New(e.Pool)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	keep(ctx, t, c, e.Rep1, homecache.Magic, nil, "today")
	keep(ctx, t, c, e.Rep1, homecache.Worklist, nil, "today")

	e.WsExec(t, `INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id)
		VALUES ('agent', 'agent:enrich', 'update', 'deal', $1)`, ids.NewV7())
	for _, read := range []homecache.Read{homecache.Magic, homecache.Worklist} {
		if !served(ctx, t, c, e.Rep1, read) {
			t.Errorf("a machine write ended the kept %s", read)
		}
	}
}

func TestAnAnswerNearTheEndOfItsLifeIsServedAndReadAgainBehindIt(t *testing.T) {
	e := Setup(t)
	c := homecache.New(e.Pool)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	keep(ctx, t, c, e.Rep1, homecache.Worklist, nil, "decisions")
	e.WsExec(t, `UPDATE home_read_cache SET computed_at = computed_at - $1::interval WHERE reader_id = $2`,
		(homecache.Life - 30*time.Second).String(), e.Rep1)

	reads := 0
	fresh := func(context.Context) (crmcontracts.Worklist, error) {
		reads++
		return crmcontracts.Worklist{View: new(crmcontracts.WorklistViewToday)}, nil
	}
	noWalk := homecache.NoWalk[crmcontracts.Worklist]()
	got, stale, err := homecache.Serve(ctx, c, e.Rep1, homecache.Worklist, "home", fresh, noWalk)
	if err != nil || stale || got.View == nil || *got.View != "decisions" {
		t.Fatalf("serve near the end of life = %+v (stale %v, err %v), want the kept answer, fresh", got, stale, err)
	}
	c.Settle()
	if reads != 1 {
		t.Fatalf("read again %d times behind the served answer, want once", reads)
	}
	kept, standing, hit, err := homecache.Lookup[crmcontracts.Worklist](ctx, c, e.Rep1, homecache.Worklist, "home")
	if err != nil || !hit || kept.View == nil || *kept.View != crmcontracts.WorklistViewToday || standing.Age > time.Minute {
		t.Errorf("after the refresh the kept answer is %+v aged %s (hit %v, err %v), want the fresh one", kept, standing.Age, hit, err)
	}
}

func TestAnAnswerPastItsLifeIsServedStaleUntilItsGraceEnds(t *testing.T) {
	e := Setup(t)
	c := homecache.New(e.Pool)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	reads := 0
	fresh := func(context.Context) (crmcontracts.Worklist, error) {
		reads++
		return crmcontracts.Worklist{View: new(crmcontracts.WorklistViewToday)}, nil
	}
	noWalk := homecache.NoWalk[crmcontracts.Worklist]()
	age := func(by time.Duration) {
		e.WsExec(t, `UPDATE home_read_cache SET computed_at = computed_at - $1::interval,
		    valid_until = valid_until - $1::interval WHERE reader_id = $2`, by.String(), e.Rep1)
	}

	keep(ctx, t, c, e.Rep1, homecache.Worklist, nil, "decisions")
	age(homecache.Life + time.Minute)
	staleBefore, hitsBefore := homeCacheReads(t, homecache.Worklist, "stale"), homeCacheReads(t, homecache.Worklist, "hit")
	got, stale, err := homecache.Serve(ctx, c, e.Rep1, homecache.Worklist, "home", fresh, noWalk)
	if err != nil || !stale || got.View == nil || *got.View != "decisions" {
		t.Fatalf("serve past its life = %+v (stale %v, err %v), want the kept answer, stale", got, stale, err)
	}
	if grew, hits := homeCacheReads(t, homecache.Worklist, "stale")-staleBefore, homeCacheReads(t, homecache.Worklist, "hit")-hitsBefore; grew != 1 || hits != 0 {
		t.Errorf("an answer served past its life counted %d stale and %d hits, want 1 and 0", grew, hits)
	}
	c.Settle()
	if reads != 1 {
		t.Fatalf("read again %d times behind the stale answer, want once", reads)
	}

	// Back from a long break on the same day: the kept day shows at once.
	keep(ctx, t, c, e.Rep1, homecache.Worklist, nil, "decisions")
	age(6 * time.Hour)
	if got, stale, err := homecache.Serve(ctx, c, e.Rep1, homecache.Worklist, "home", fresh, noWalk); err != nil || !stale ||
		got.View == nil || *got.View != "decisions" {
		t.Fatalf("serve six hours on = %+v (stale %v, err %v), want the kept answer, stale", got, stale, err)
	}
	c.Settle()

	keep(ctx, t, c, e.Rep1, homecache.Worklist, nil, "decisions")
	age(homecache.Life + homecache.StaleGrace + time.Second)
	if got, stale, err := homecache.Serve(ctx, c, e.Rep1, homecache.Worklist, "home", fresh, noWalk); err != nil || stale ||
		got.View == nil || *got.View != crmcontracts.WorklistViewToday || reads != 3 {
		t.Errorf("serve past its grace = %+v (stale %v, reads %d, err %v), want a fresh read", got, stale, reads, err)
	}
}

func TestAReceiptPastItsLifeIsReadFresh(t *testing.T) {
	e := Setup(t)
	c := homecache.New(e.Pool)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	keep(ctx, t, c, e.Rep1, homecache.Magic, nil, "decisions")
	e.WsExec(t, `UPDATE home_read_cache SET computed_at = computed_at - $1::interval WHERE reader_id = $2`,
		(homecache.Life + time.Minute).String(), e.Rep1)
	reads := 0
	fresh := func(context.Context) (crmcontracts.Worklist, error) {
		reads++
		return crmcontracts.Worklist{}, nil
	}
	noWalk := homecache.NoWalk[crmcontracts.Worklist]()
	misses, stales := homeCacheReads(t, homecache.Magic, "miss"), homeCacheReads(t, homecache.Magic, "stale")
	if _, stale, err := homecache.Serve(ctx, c, e.Rep1, homecache.Magic, "home", fresh, noWalk); err != nil || stale || reads != 1 {
		t.Errorf("a receipt past its life: stale %v, %d fresh reads (err %v); want it read fresh once", stale, reads, err)
	}
	if grew, stale := homeCacheReads(t, homecache.Magic, "miss")-misses, homeCacheReads(t, homecache.Magic, "stale")-stales; grew != 1 || stale != 0 {
		t.Errorf("a receipt read fresh past its life counted %d misses and %d stale, want 1 and 0", grew, stale)
	}
}

// A background refresh that began before a reader's miss must not replace the
// newer answer that miss kept.
func TestAnOlderAnswerNeverReplacesANewerOne(t *testing.T) {
	e := Setup(t)
	c := homecache.New(e.Pool)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	older, err := c.Stamp(ctx)
	if err != nil {
		t.Fatal(err)
	}
	keep(ctx, t, c, e.Rep1, homecache.Worklist, nil, "decisions")
	if err := homecache.Store(ctx, c, e.Rep1, homecache.Worklist, "home", older, nil,
		crmcontracts.Worklist{View: new(crmcontracts.WorklistViewToday)}); err != nil {
		t.Fatal(err)
	}
	got, _, hit, err := homecache.Lookup[crmcontracts.Worklist](ctx, c, e.Rep1, homecache.Worklist, "home")
	if err != nil || !hit || got.View == nil || *got.View != "decisions" {
		t.Errorf("kept answer = %+v (hit %v, err %v), want the newer one", got, hit, err)
	}
}

// The cache only saves work. A kept answer this build cannot read, written by an
// older one, is passed over for the fresh read, which is kept in its place.
func TestAnUnreadableKeptAnswerIsServedFreshAndReplaced(t *testing.T) {
	e := Setup(t)
	c := homecache.New(e.Pool)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	keep(ctx, t, c, e.Rep1, homecache.Worklist, nil, "today")
	e.WsExec(t, `UPDATE home_read_cache SET payload = '"an older shape"' WHERE reader_id = $1`, e.Rep1)

	reads := 0
	fresh := func(context.Context) (crmcontracts.Worklist, error) {
		reads++
		return crmcontracts.Worklist{View: new(crmcontracts.WorklistViewToday)}, nil
	}
	noWalk := homecache.NoWalk[crmcontracts.Worklist]()
	misses := homeCacheReads(t, homecache.Worklist, "miss")
	if _, _, err := homecache.Serve(ctx, c, e.Rep1, homecache.Worklist, "home", fresh, noWalk); err != nil || reads != 1 {
		t.Fatalf("serve over an unreadable answer: err %v after %d fresh reads, want the fresh read", err, reads)
	}
	if grew := homeCacheReads(t, homecache.Worklist, "miss") - misses; grew != 1 {
		t.Errorf("a kept answer this build cannot read counted %d misses, want 1", grew)
	}
	if !served(ctx, t, c, e.Rep1, homecache.Worklist) {
		t.Error("the fresh answer was not kept in place of the unreadable one")
	}
}

// /metrics counts each Home read Serve answered, so the hit rate is measured.
func TestServeCountsItsHitsAndMissesOnMetrics(t *testing.T) {
	e := Setup(t)
	c := homecache.New(e.Pool)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	fresh := func(context.Context) (crmcontracts.MagicReceipt, error) { return crmcontracts.MagicReceipt{}, nil }
	noWalk := homecache.NoWalk[crmcontracts.MagicReceipt]()
	hits, firsts := homeCacheReads(t, homecache.Magic, "hit"), homeCacheReads(t, homecache.Magic, "first")

	for range 2 {
		if _, _, err := homecache.Serve(ctx, c, e.Rep1, homecache.Magic, "home", fresh, noWalk); err != nil {
			t.Fatal(err)
		}
	}
	if got := homeCacheReads(t, homecache.Magic, "first") - firsts; got != 1 {
		t.Errorf("first reads grew by %d, want 1: the first read found nothing kept", got)
	}
	if got := homeCacheReads(t, homecache.Magic, "hit") - hits; got != 1 {
		t.Errorf("hits grew by %d, want 1: the second read was served the kept answer", got)
	}
}

// homeCacheReads reads one series of the cache's counter from its /metrics lines.
func homeCacheReads(t *testing.T, read homecache.Read, outcome string) int {
	t.Helper()
	var out strings.Builder
	homecache.WriteMetrics(&out)
	prefix := `margince_home_cache_reads_total{read="` + string(read) + `",outcome="` + outcome + `"} `
	for line := range strings.Lines(out.String()) {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), prefix); ok {
			n, err := strconv.Atoi(value)
			if err != nil {
				t.Fatalf("reading %q: %v", line, err)
			}
			return n
		}
	}
	t.Fatalf("no %s series in:\n%s", prefix, out.String())
	return 0
}

// serveToday serves the Home worklist from c, counting how often it is read
// fresh. The fake page is "today", the kept one "decisions".
func serveToday(ctx context.Context, t *testing.T, c *homecache.Cache, reader ids.UUID, reads *int) (crmcontracts.Worklist, bool) {
	t.Helper()
	fresh := func(context.Context) (crmcontracts.Worklist, error) {
		*reads++
		return crmcontracts.Worklist{View: new(crmcontracts.WorklistViewToday)}, nil
	}
	noWalk := homecache.NoWalk[crmcontracts.Worklist]()
	got, stale, err := homecache.Serve(ctx, c, reader, homecache.Worklist, "home", fresh, noWalk)
	if err != nil {
		t.Fatal(err)
	}
	return got, stale
}

func TestAReadersWriteLeavesTheKeptDayServedStaleAndReadsItAgainBehind(t *testing.T) {
	e := Setup(t)
	c := homecache.New(e.Pool)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	keep(ctx, t, c, e.Rep1, homecache.Worklist, nil, "decisions")
	if err := c.Forget(ctx, []ids.UUID{e.Rep1}); err != nil {
		t.Fatal(err)
	}

	reads := 0
	got, stale := serveToday(ctx, t, c, e.Rep1, &reads)
	if !stale || got.View == nil || *got.View != "decisions" {
		t.Fatalf("after the reader's write Home served %+v (stale %v), want the kept day flagged stale", got, stale)
	}
	c.Settle()
	if reads != 1 {
		t.Fatalf("read again %d times behind the stale day, want once", reads)
	}
	got, stale = serveToday(ctx, t, c, e.Rep1, &reads)
	if stale || got.View == nil || *got.View != crmcontracts.WorklistViewToday || reads != 1 {
		t.Errorf("after the refresh Home served %+v (stale %v, %d reads), want the refreshed day, current, from the cache", got, stale, reads)
	}
}

func TestAReceiptBehindAReadersWriteIsReadFresh(t *testing.T) {
	e := Setup(t)
	c := homecache.New(e.Pool)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	keep(ctx, t, c, e.Rep1, homecache.Magic, nil, "decisions")
	if err := c.Forget(ctx, []ids.UUID{e.Rep1}); err != nil {
		t.Fatal(err)
	}
	reads := 0
	fresh := func(context.Context) (crmcontracts.Worklist, error) {
		reads++
		return crmcontracts.Worklist{}, nil
	}
	noWalk := homecache.NoWalk[crmcontracts.Worklist]()
	if _, stale, err := homecache.Serve(ctx, c, e.Rep1, homecache.Magic, "home", fresh, noWalk); err != nil || stale || reads != 1 {
		t.Errorf("a receipt behind a write: stale %v, %d fresh reads (err %v); want it read fresh once", stale, reads, err)
	}
}

// A stale day is a copy of what the reader could see before. When their reach
// narrows (a record moves to a colleague, a team membership ends) it is never
// shown, even marked stale.
func TestAStaleDayIsNeverServedAcrossAChangeInWhoMayReadWhat(t *testing.T) {
	changes := map[string]func(*testing.T, *Env, ids.UUID){
		"a record leaves the reader's reach": func(t *testing.T, e *Env, contact ids.UUID) {
			e.WsExec(t, `UPDATE contact SET owner_id = $1 WHERE id = $2`, e.Rep2, contact)
		},
		"the reader leaves their team": func(t *testing.T, e *Env, _ ids.UUID) {
			e.WsExec(t, `DELETE FROM team_membership WHERE user_id = $1`, e.Rep1)
		},
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			e := Setup(t)
			c := homecache.New(e.Pool)
			ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
			contact := e.SeedContact(t, "Moves away", &e.Rep1)
			keep(ctx, t, c, e.Rep1, homecache.Worklist, nil, "decisions")
			if err := c.Forget(ctx, []ids.UUID{e.Rep1}); err != nil {
				t.Fatal(err)
			}
			change(t, e, contact)

			reads := 0
			got, stale := serveToday(ctx, t, c, e.Rep1, &reads)
			if stale || reads != 1 || got.View == nil || *got.View != crmcontracts.WorklistViewToday {
				t.Errorf("%s: Home served %+v (stale %v, %d fresh reads), want one fresh read and no stale copy", name, got, stale, reads)
			}
		})
	}
}

// An administrator changes what a member may read through writes the member's own
// request never makes, so no per-writer mark reaches their cache. The access
// fence must end the kept day, fresh or marked behind a write.
func TestAMemberChangeByAnotherUserNeverLeavesTheKeptDayServed(t *testing.T) {
	changes := map[string]string{
		"their role assignment is removed":    `DELETE FROM role_assignment WHERE user_id = $1`,
		"a role's permissions are narrowed":   `UPDATE role SET permissions = '{}'::jsonb WHERE id IN (SELECT role_id FROM role_assignment WHERE user_id = $1)`,
		"they are removed from their team":    `DELETE FROM team_membership WHERE user_id = $1`,
		"their seat becomes read only":        `UPDATE app_user SET seat_type = 'read' WHERE id = $1`,
		"a field mask is added to their role": `INSERT INTO field_mask (role_key, object, field, condition) SELECT r.key, 'partner', 'margin_tier', 'always' FROM role r JOIN role_assignment a ON a.role_id = r.id WHERE a.user_id = $1 ON CONFLICT (role_key, object, field) DO UPDATE SET condition = 'always'`,
	}
	for name, change := range changes {
		for _, behind := range []bool{false, true} {
			t.Run(name+fmt.Sprintf(" (behind a write: %v)", behind), func(t *testing.T) {
				e := Setup(t)
				c := homecache.New(e.Pool)
				ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
				e.WsExec(t, `WITH r AS (INSERT INTO role (key, name, permissions) VALUES ('fence_probe', 'Fence probe', '{"contact": {"read": true}}') RETURNING id)
					INSERT INTO role_assignment (role_id, user_id) SELECT id, $1 FROM r`, e.Rep1)
				keep(ctx, t, c, e.Rep1, homecache.Worklist, nil, "decisions")
				if behind {
					if err := c.Forget(ctx, []ids.UUID{e.Rep1}); err != nil {
						t.Fatal(err)
					}
				}
				if n := e.WsCount(t, `WITH changed AS (`+strings.Replace(change, "$1", "'"+e.Rep1.String()+"'", 1)+` RETURNING 1) SELECT count(*) FROM changed`); n == 0 {
					t.Fatal("the change touched no row, so the case proves nothing")
				}
				reads := 0
				got, stale := serveToday(ctx, t, c, e.Rep1, &reads)
				if stale || reads != 1 || got.View == nil || *got.View != crmcontracts.WorklistViewToday {
					t.Errorf("Home served %+v (stale %v, %d fresh reads), want one fresh read and no kept copy", got, stale, reads)
				}
			})
		}
	}
}

// An answer under another day's key is never asked for again, and it holds
// display text, so the reader's next kept answer removes it.
func TestKeepingAnAnswerRemovesTheReadersAnswersThatCanNoLongerBeServed(t *testing.T) {
	e := Setup(t)
	c := homecache.New(e.Pool)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	other := e.As(e.Rep2, []ids.UUID{e.Team1}, RepPerms)
	stamp, err := c.Stamp(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"yesterday", "earlier today"} {
		if err := homecache.Store(ctx, c, e.Rep1, homecache.Worklist, key, stamp, nil, crmcontracts.Worklist{}); err != nil {
			t.Fatal(err)
		}
	}
	keep(other, t, c, e.Rep2, homecache.Worklist, nil, "decisions")
	e.WsExec(t, `UPDATE home_read_cache SET valid_until = now() - interval '1 second'
	              WHERE request_key = 'yesterday' OR reader_id = $1`, e.Rep2)

	keep(ctx, t, c, e.Rep1, homecache.Worklist, nil, "decisions")
	mine := e.WsScalar(t, `SELECT string_agg(request_key, ', ' ORDER BY request_key)
	                         FROM home_read_cache WHERE reader_id = $1`, e.Rep1)
	if mine != "earlier today, home" {
		t.Errorf("the reader's kept answers = %q, want the ended one gone and the rest kept", mine)
	}
	if e.WsCount(t, `SELECT count(*) FROM home_read_cache WHERE reader_id = $1`, e.Rep2) != 1 {
		t.Error("keeping one reader's answer removed another reader's")
	}
}
