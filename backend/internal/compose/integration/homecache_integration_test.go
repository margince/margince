// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What ends a cached Home answer, asked of the cache the server builds.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/homecache"
	"github.com/margince/margince/backend/internal/compose/magic"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
	_, hit, err := homecache.Lookup[crmcontracts.Worklist](ctx, c, reader, read, "home")
	if err != nil {
		t.Fatal(err)
	}
	return hit
}

func TestACachedHomeAnswerIsServedToItsReaderUntilTheFenceMoves(t *testing.T) {
	e := Setup(t)
	c := homecache.New(e.Pool, magic.MachineActors())
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	contact := e.SeedContact(t, "Fenced", &e.Rep1)

	keep(ctx, t, c, e.Rep1, homecache.Worklist, nil, "decisions")
	if got, hit, err := homecache.Lookup[crmcontracts.Worklist](ctx, c, e.Rep1, homecache.Worklist, "home"); err != nil || !hit || got.View == nil || *got.View != "decisions" {
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

// A read that began before its reader's write and stores after it would hand
// the reader the day as it was before they acted.
func TestAnAnswerComputedBeforeItsReadersWriteIsNeverServed(t *testing.T) {
	e := Setup(t)
	c := homecache.New(e.Pool, magic.MachineActors())
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
	if served(ctx, t, c, e.Rep1, homecache.Worklist) {
		t.Error("an answer stamped before the reader's write was served")
	}

	keep(ctx, t, c, e.Rep1, homecache.Worklist, nil, "today")
	if !served(ctx, t, c, e.Rep1, homecache.Worklist) {
		t.Error("an answer stamped after the write was not served")
	}
}

// Expiry writes nothing, so it bounds the answer's life instead.
func TestAnAnswerLivesNoLongerThanTheNextGrantToExpire(t *testing.T) {
	e := Setup(t)
	c := homecache.New(e.Pool, magic.MachineActors())
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
		e.Rep1, homecache.Life.Seconds()); n != 1 {
		t.Errorf("the kept answer outlives homecache.Life (%d rows bounded)", n)
	}
}

func TestACachedFirstPageEndsWithTheWalkItStarted(t *testing.T) {
	e := Setup(t)
	c := homecache.New(e.Pool, magic.MachineActors())
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	walk := ids.NewV7()
	e.WsExec(t, `INSERT INTO worklist_snapshot (id, reader_id, params_fingerprint, as_of, created_at, expires_at, buckets, rows)
		VALUES ($1, $2, 'home', now(), now(), now() + interval '4 hours', '{}', '[]')`, walk, e.Rep1)

	keep(ctx, t, c, e.Rep1, homecache.Worklist, &walk, "today")
	if !served(ctx, t, c, e.Rep1, homecache.Worklist) {
		t.Fatal("a page whose walk stands was not served")
	}
	e.WsExec(t, `DELETE FROM worklist_snapshot WHERE id = $1`, walk)
	if served(ctx, t, c, e.Rep1, homecache.Worklist) {
		t.Error("a page was served after the walk its cursor resumes was gone")
	}
}

func TestAMachineWriteEndsTheCachedReceiptButNotTheWorklist(t *testing.T) {
	e := Setup(t)
	c := homecache.New(e.Pool, magic.MachineActors())
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	keep(ctx, t, c, e.Rep1, homecache.Magic, nil, "today")
	keep(ctx, t, c, e.Rep1, homecache.Worklist, nil, "today")

	e.WsExec(t, `INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id)
		VALUES ('agent', 'agent:enrich', 'update', 'deal', $1)`, ids.NewV7())
	if served(ctx, t, c, e.Rep1, homecache.Magic) {
		t.Error("the receipt was served after a machine wrote")
	}
	if !served(ctx, t, c, e.Rep1, homecache.Worklist) {
		t.Error("a machine write ended the worklist's answer, which it is not bound by")
	}
}
