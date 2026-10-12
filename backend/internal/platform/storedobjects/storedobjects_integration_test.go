// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package storedobjects_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/storedobjects"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A retry restarts the grace period, because the window it measures restarts.
//
// This is the one that can delete a contact's file. A key recorded again must not
// inherit an earlier intent's age: inheriting it makes the key eligible for the
// reaper the moment it is recorded, and a sweep running between this put and its row
// commit then deletes the bytes just written.
func TestARetryRestartsTheGracePeriod(t *testing.T) {
	ctx, pool := ledgerEnv(t)
	ledger := storedobjects.NewLedger(database.BindTo(pool, workspaceOf(ctx)))
	const key = "ws/attachment/retried"

	if err := ledger.Record(ctx, storedobjects.KindAttachment, key); err != nil {
		t.Fatalf("recording the first intent: %v", err)
	}
	// Aged past the grace, which is the state a retry must not inherit.
	age(ctx, t, pool, key, 2*storedobjects.ProvisionalObjectGrace)
	before := provisionalSince(ctx, t, pool, key)

	if err := ledger.Record(ctx, storedobjects.KindAttachment, key); err != nil {
		t.Fatalf("recording the retry: %v", err)
	}

	after := provisionalSince(ctx, t, pool, key)
	if !after.After(before) {
		t.Fatalf("recorded_at stayed at %s after a retry, so the key is already past its grace: "+
			"a sweep between this put and its row would delete the bytes just written", before)
	}
	if time.Since(after) > time.Minute {
		t.Errorf("recorded_at is %s, which is not this retry's clock", after)
	}
}

// A kind outside the vocabulary is refused where it is written.
//
// It reaches the sweep as a key nobody owns, and the sweep cannot adjudicate one —
// so a typo in a single writer would strand every other writer's orphans.
func TestAnUndeclaredKindIsRefused(t *testing.T) {
	ctx, pool := ledgerEnv(t)
	ledger := storedobjects.NewLedger(database.BindTo(pool, workspaceOf(ctx)))

	err := ledger.Record(ctx, storedobjects.Kind("knowledge-typo"), "ws/knowledge-typo/x")
	if err == nil {
		t.Fatal("an undeclared kind was recorded, so the sweep inherits a key it cannot adjudicate")
	}
	if holds(ctx, t, pool, "ws/knowledge-typo/x") {
		t.Error("a key with a refused kind was left in the ledger")
	}
}

// What is declared is what comes back, and clearing it on the caller's transaction
// takes it away.
func TestADeclarationSurvivesUntilItIsCleared(t *testing.T) {
	ctx, pool := ledgerEnv(t)
	db := database.BindTo(pool, workspaceOf(ctx))
	ledger := storedobjects.NewLedger(db)
	const key = "ws/knowledge/declared"

	if err := ledger.Record(ctx, storedobjects.KindKnowledge, key); err != nil {
		t.Fatalf("recording: %v", err)
	}
	age(ctx, t, pool, key, 2*storedobjects.ProvisionalObjectGrace)

	held, err := ledger.ListProvisional(systemCtx(ctx), time.Now().UTC(), 500)
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	// Found among whatever else the installation holds, never asserted to be alone:
	// this table carries no workspace column — the key's own prefix is the tenant —
	// so every test in this lane shares one ledger.
	var mine *storedobjects.Provisional
	for i := range held {
		if held[i].Key == key {
			mine = &held[i]
		}
	}
	if mine == nil {
		t.Fatalf("the declared key is not among the %d provisional keys", len(held))
	}
	if mine.Kind != storedobjects.KindKnowledge {
		t.Errorf("the key came back as kind %q, want %q", mine.Kind, storedobjects.KindKnowledge)
	}

	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		return storedobjects.Clear(ctx, tx, key)
	}); err != nil {
		t.Fatalf("clearing: %v", err)
	}
	if holds(ctx, t, pool, key) {
		t.Error("the key is still declared after its clear committed")
	}
}

// The sweep's two reads refuse a caller that is not the system.
//
// A seat asking what is provisional is asking which objects it could orphan, and
// retiring one is the reaper's act — neither is a seat's to take.
func TestTheSweepsReadsRefuseASeat(t *testing.T) {
	ctx, pool := ledgerEnv(t)
	ledger := storedobjects.NewLedger(database.BindTo(pool, workspaceOf(ctx)))

	if _, err := ledger.ListProvisional(ctx, time.Now().UTC(), 10); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("ListProvisional as a seat = %v, want permission denied", err)
	}
	if err := ledger.Retire(ctx, "ws/attachment/x"); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("Retire as a seat = %v, want permission denied", err)
	}
}

func ledgerEnv(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("MARGINCE_TEST_DSN")
	if dsn == "" {
		t.Fatal("MARGINCE_TEST_DSN is not set — run `make db-up` and try again (integration tests fail loudly, they never skip)")
	}
	pool, err := testdb.OwnPool(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:ledger-test",
	}), pool
}

// systemCtx is the principal the sweep runs under.
func systemCtx(ctx context.Context) context.Context {
	return principal.SystemActing(ctx, "stored_object_reap_worker")
}

func workspaceOf(ctx context.Context) ids.WorkspaceID {
	ws, _ := principal.WorkspaceID(ctx)
	return ids.From[ids.WorkspaceKind](ws)
}

// age backdates one declaration, so a test can reach the far side of the grace
// without waiting a day for it.
func age(ctx context.Context, t *testing.T, pool *pgxpool.Pool, key string, by time.Duration) {
	t.Helper()
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE stored_object_intent SET recorded_at = now() - $2::interval
			 WHERE storage_key = $1`, key, by.String())
		return err
	}); err != nil {
		t.Fatalf("backdating %s: %v", key, err)
	}
}

func provisionalSince(ctx context.Context, t *testing.T, pool *pgxpool.Pool, key string) time.Time {
	t.Helper()
	var at time.Time
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT recorded_at FROM stored_object_intent WHERE storage_key = $1`, key).Scan(&at)
	}); err != nil {
		t.Fatalf("reading %s back: %v", key, err)
	}
	return at
}

// holds answers whether ONE key is declared, which is all a test in a shared lane
// may ask: the ledger is installation-wide, so counting its rows counts everybody's.
func holds(ctx context.Context, t *testing.T, pool *pgxpool.Pool, key string) bool {
	t.Helper()
	var found bool
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM stored_object_intent WHERE storage_key = $1)`, key).Scan(&found)
	}); err != nil {
		t.Fatalf("asking whether %s is declared: %v", key, err)
	}
	return found
}
