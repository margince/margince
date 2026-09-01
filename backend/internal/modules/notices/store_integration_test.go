// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package notices

// The notice's whole life against a real database: created in the write
// shape (row + audit + outbox in one transaction), read only by its own
// recipient, settled once — because the row IS the transport, and every one
// of those properties is what "delivered" means here.

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type noticeEnv struct {
	owner     *pgx.Conn
	pool      *pgxpool.Pool
	store     *Store
	ws        ids.UUID
	recipient ids.UserID
	other     ids.UserID
}

func setupNotices(t *testing.T) *noticeEnv {
	t.Helper()
	ownerDSN := os.Getenv("MARGINCE_TEST_DSN")
	appDSN := os.Getenv("MARGINCE_TEST_APP_DSN")
	if ownerDSN == "" || appDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN / MARGINCE_TEST_APP_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
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
	if err := testdb.Reset(ctx, owner); err != nil {
		t.Fatal(err)
	}
	e := &noticeEnv{owner: owner, ws: ids.NewV7(), recipient: ids.New[ids.UserKind](), other: ids.New[ids.UserKind]()}
	if _, err := owner.Exec(ctx, `INSERT INTO workspace (id) VALUES ($1)`, e.ws); err != nil {
		t.Fatal(err)
	}
	for _, u := range []ids.UserID{e.recipient, e.other} {
		if _, err := owner.Exec(ctx,
			`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Rep')`,
			u, "rep-"+u.String()+"@notices.test"); err != nil {
			t.Fatal(err)
		}
	}
	pool, err := testdb.Pool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testdb.AssertPoolsQuiesced(t) })
	e.pool = pool
	e.store = NewStore(database.BindTo(pool, ids.From[ids.WorkspaceKind](e.ws)))
	return e
}

// engineCtx is who really creates a notice: the automation engine's system
// principal inside a correlation scope.
func (e *noticeEnv) engineCtx() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: "system:automation"})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

func (e *noticeEnv) asUser(u ids.UserID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + u.String(), UserID: u.UUID,
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

// THE BUS IS AT-LEAST-ONCE, so a handler that writes a notice for an event it
// may be handed twice has to land one line. workflow.Handler.Apply says as much
// in its own contract, and a handler that relies on the queue for that instead
// has put its correctness where no reader of this package can check it.
//
// Both directions, because a duplicate is not only a duplicate row: the audit
// trail and the announcement have to collapse with it, or a second event about
// a notice that already exists puts the same line on the same Worklist by
// another route.
// What a repeat ANSWERS WITH, which is a separate question from what it writes.
//
// A dedupe key names the EVENT rather than the text, so the two deliveries need
// not say the same thing: the second can carry a subject reworded since, or a
// kind a later version spells differently. Answering it with the stored id and
// this call's words describes a notice that exists nowhere — the reader cannot
// find it, and the database does not say it.
func TestARepeatAnswersWithTheStoredNoticeRatherThanTheReplay(t *testing.T) {
	e := setupNotices(t)
	key := "lead_sla:" + ids.NewV7().String()
	stored, err := e.store.insertNotice(e.engineCtx(), NewNotice{
		Recipient: e.recipient, Kind: "lead_sla", Subject: "SLA breach",
		Body: "overdue", DedupeKey: key,
	}, nil)
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}
	replay, err := e.store.insertNotice(e.engineCtx(), NewNotice{
		Recipient: e.recipient, Kind: "lead_sla_v2", Subject: "Response overdue",
		Body: "still overdue", DedupeKey: key,
	}, nil)
	if err != nil {
		t.Fatalf("second insert: %v", err)
	}
	if replay != stored {
		t.Errorf("the repeat answered %+v, want the notice that already stands %+v — every field, "+
			"or a caller renders a line nothing in the database says", replay, stored)
	}
}

func TestANoticeCarryingAKeyIsWrittenOncePerRecipient(t *testing.T) {
	e := setupNotices(t)
	again := NewNotice{
		Recipient: e.recipient, Kind: "lead_sla", Subject: "SLA breach",
		Body: "overdue", DedupeKey: "lead_sla:" + ids.NewV7().String(),
	}
	first, err := e.store.Create(e.engineCtx(), again)
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}
	second, err := e.store.Create(e.engineCtx(), again)
	if err != nil {
		t.Fatalf("second Create: %v", err)
	}
	if second != first {
		t.Errorf("the repeat answered %s, want the id of the notice that already stands (%s) — "+
			"a caller holding an id nothing has is worse than a duplicate", second, first)
	}

	var rows, audits, events int
	if err := e.owner.QueryRow(context.Background(), `
		SELECT (SELECT count(*) FROM notice WHERE dedupe_key = $1),
		       (SELECT count(*) FROM audit_log
		         WHERE entity_type = 'notice' AND entity_id = $2 AND action = 'create'),
		       (SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'notice.created')`,
		again.DedupeKey, first).Scan(&rows, &audits, &events); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if rows != 1 {
		t.Errorf("%d notices carry the key, want 1 — one breach delivered twice is two identical lines on one Worklist", rows)
	}
	if audits != 1 || events != 1 {
		t.Errorf("audits=%d events=%d, want 1 each — the second delivery announced a notice it did not write", audits, events)
	}

	// PER RECIPIENT, and this is where that claim is earned. One breach
	// escalated to two people is TWO notices and must stay two — a key scoped
	// to the event alone would put the line on whichever Worklist reached it
	// first and leave the other person told nothing.
	toSomebodyElse := again
	toSomebodyElse.Recipient = e.other
	other, err := e.store.Create(e.engineCtx(), toSomebodyElse)
	if err != nil {
		t.Fatalf("Create for the second recipient: %v", err)
	}
	if other == first {
		t.Fatal("the same key addressed to a second person answered the first person's notice — " +
			"one breach escalated to two people would tell only one of them")
	}
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM notice WHERE dedupe_key = $1`, again.DedupeKey).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 2 {
		t.Errorf("%d notices carry the key across two recipients, want 2", rows)
	}
}

// A notice with NO key dedupes against nothing, and that is the answer rather
// than an oversight: most notices are addressed by a human act and have no
// event to key on, and two of them are two real notices.
func TestANoticeWithoutAKeyIsWrittenEveryTime(t *testing.T) {
	e := setupNotices(t)
	unkeyed := NewNotice{Recipient: e.recipient, Kind: "automation", Subject: "Deal moved"}
	first, err := e.store.Create(e.engineCtx(), unkeyed)
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}
	second, err := e.store.Create(e.engineCtx(), unkeyed)
	if err != nil {
		t.Fatalf("second Create: %v", err)
	}
	if second == first {
		t.Fatal("two unkeyed notices collapsed into one — a caller with no natural key had one of their notices silently dropped")
	}
}

func TestANoticeIsCreatedInTheWriteShapeAndSettledOnce(t *testing.T) {
	e := setupNotices(t)
	id, err := e.store.Create(e.engineCtx(), NewNotice{
		Recipient: e.recipient, Kind: "automation", Subject: "Deal moved", Body: "The rule fired.",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// The write shape: the row, its audit entry and its announcement in one
	// committed state.
	var audits, events int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE entity_type = 'notice' AND entity_id = $1 AND action = 'create'`,
		id).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'notice.created'`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if audits != 1 || events != 1 {
		t.Fatalf("write shape: %d audit rows, %d events — want one of each", audits, events)
	}

	// Only its recipient reads it; another person's lane stays empty.
	unread, err := e.store.UnreadFor(e.asUser(e.recipient), 8)
	if err != nil {
		t.Fatalf("UnreadFor: %v", err)
	}
	if len(unread) != 1 || unread[0].Subject != "Deal moved" {
		t.Fatalf("recipient's unread = %+v, want the one notice", unread)
	}
	othersView, err := e.store.UnreadFor(e.asUser(e.other), 8)
	if err != nil {
		t.Fatalf("UnreadFor as another person: %v", err)
	}
	if len(othersView) != 0 {
		t.Fatalf("another person reads %+v, want nothing", othersView)
	}

	// Another person cannot settle it either — it reads as absent.
	if err := e.store.MarkRead(e.asUser(e.other), id); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("MarkRead by a stranger = %v, want not-found", err)
	}

	// The recipient settles it; it leaves the lane and a replay is the same
	// success with no second announcement.
	if err := e.store.MarkRead(e.asUser(e.recipient), id); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if err := e.store.MarkRead(e.asUser(e.recipient), id); err != nil {
		t.Fatalf("replayed MarkRead: %v", err)
	}
	var readEvents int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'notice.read'`).Scan(&readEvents); err != nil {
		t.Fatal(err)
	}
	if readEvents != 1 {
		t.Fatalf("notice.read announced %d times, want once", readEvents)
	}
	settled, err := e.store.UnreadFor(e.asUser(e.recipient), 8)
	if err != nil {
		t.Fatalf("UnreadFor after settle: %v", err)
	}
	if len(settled) != 0 {
		t.Fatalf("a settled notice still on the lane: %+v", settled)
	}
}

func TestANoticeToNobodyFailsLoudly(t *testing.T) {
	e := setupNotices(t)
	if _, err := e.store.Create(e.engineCtx(), NewNotice{
		Recipient: ids.New[ids.UserKind](), Kind: "automation", Subject: "orphan",
	}); err == nil {
		t.Fatal("a notice to a user who does not exist was recorded")
	}
}
