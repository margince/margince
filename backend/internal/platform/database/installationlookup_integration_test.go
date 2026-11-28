// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package database_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func installationEnv(t *testing.T) (*pgx.Conn, *pgxpool.Pool) {
	t.Helper()
	ownerDSN, appDSN := os.Getenv("MARGINCE_TEST_DSN"), os.Getenv("MARGINCE_TEST_APP_DSN")
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
	cfg, err := pgxpool.ParseConfig(appDSN)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := testdb.OwnPoolFromConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return owner, pool
}

func addWorkspace(t *testing.T, owner *pgx.Conn) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := owner.Exec(context.Background(), `INSERT INTO workspace (id) VALUES ($1)`, id); err != nil {
		t.Fatal(err)
	}
	return id
}

// One handle follows the installation: live A, then A archived with none live,
// then live B. A cached answer would keep admitting transactions after the
// archive and keep returning A after the replacement.
func TestAHandleFollowsTheInstallationThroughAnArchiveAndAReplacement(t *testing.T) {
	owner, pool := installationEnv(t)
	ctx := context.Background()
	a := addWorkspace(t, owner)
	db := database.ForInstallation(pool)
	ran := 0
	body := func(pgx.Tx) error { ran++; return nil }

	got, err := db.Workspace(ctx)
	if err != nil || got.UUID != a {
		t.Fatalf("with A live, Workspace = %v, %v; want %v", got, err, a)
	}
	if err := db.Tx(ctx, body); err != nil || ran != 1 {
		t.Fatalf("with A live, Tx = %v (body ran %d times)", err, ran)
	}

	if _, err := owner.Exec(ctx, `UPDATE workspace SET archived_at = now() WHERE id = $1`, a); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Workspace(ctx); !errors.Is(err, database.ErrNotBootstrapped) {
		t.Fatalf("with A archived and none live, Workspace = %v, want ErrNotBootstrapped", err)
	}
	if err := db.Tx(ctx, body); !errors.Is(err, database.ErrNotBootstrapped) || ran != 1 {
		t.Fatalf("with none live, Tx = %v (body ran %d times), want ErrNotBootstrapped and no run", err, ran)
	}

	b := addWorkspace(t, owner)
	got, err = db.Workspace(ctx)
	if err != nil || got.UUID != b {
		t.Fatalf("with B live, Workspace = %v, %v; want %v", got, err, b)
	}
	if err := db.Tx(ctx, body); err != nil || ran != 2 {
		t.Fatalf("with B live, Tx = %v (body ran %d times)", err, ran)
	}
}

// Many first lookups on one fresh handle all agree on the one live workspace.
func TestConcurrentFirstResolutionsOnOneHandleAgree(t *testing.T) {
	owner, pool := installationEnv(t)
	ctx := context.Background()
	a := addWorkspace(t, owner)
	db := database.ForInstallation(pool)

	const callers = 16
	answers := make([]ids.UUID, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Go(func() {
			got, err := db.Workspace(ctx)
			answers[i], errs[i] = got.UUID, err
			if err == nil {
				errs[i] = db.Tx(ctx, func(pgx.Tx) error { return nil })
			}
		})
	}
	wg.Wait()
	for i := range callers {
		if errs[i] != nil || answers[i] != a {
			t.Errorf("caller %d got %v, %v; want %v", i, answers[i], errs[i], a)
		}
	}
}
