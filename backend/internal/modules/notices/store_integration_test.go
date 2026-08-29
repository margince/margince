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

func TestANoticeIsCreatedInTheWriteShapeAndSettledOnce(t *testing.T) {
	e := setupNotices(t)
	id, err := e.store.Create(e.engineCtx(), e.recipient, "automation", "Deal moved", "The rule fired.")
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
	if _, err := e.store.Create(e.engineCtx(), ids.New[ids.UserKind](), "automation", "orphan", ""); err == nil {
		t.Fatal("a notice to a user who does not exist was recorded")
	}
}
