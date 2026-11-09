// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

import (
	"context"
	"flag"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const schemaRefusalChild = "schema-refusal-child"

func TestEnsureSchemaRefusesAMismatchedCloneWithoutDroppingRiver(t *testing.T) {
	if args := flag.Args(); len(args) == 1 && args[0] == schemaRefusalChild {
		assertMismatchedCloneRefused(t)
		return
	}
	ctx := context.Background()
	admin := ownerConn(t)
	name := "margince_schema_refusal_" + strings.ReplaceAll(ids.NewV7().String(), "-", "")
	ident := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+ident); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "DROP DATABASE "+ident+" WITH (FORCE)"); err != nil {
			t.Errorf("dropping the refusal fixture: %v", err)
		}
	})
	cfg := admin.Config().Copy()
	cfg.Database = name
	owner, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing the refusal fixture: %v", err)
		}
	})
	if err := rebuildSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	poolCfg, err := pgxpool.ParseConfig(cfg.ConnString())
	if err != nil {
		t.Fatal(err)
	}
	poolCfg.ConnConfig.Database = name
	poolCfg.MaxConns = 2
	pool, err := OwnPoolFromConfig(ctx, poolCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := jobs.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	future := dbmigrate.Namespace{Name: "core", Migrations: []dbmigrate.Migration{{
		Version: "9999999999", Name: "future_schema", UpSQL: "SELECT 1", DownSQL: "SELECT 1",
	}}}
	if applied, err := dbmigrate.Up(ctx, owner, future); err != nil || applied != 1 {
		t.Fatalf("applying a newer source tree's migration: count=%d err=%v", applied, err)
	}
	// A child gives EnsureSchema its real first call without resetting global state.
	if err := gatekit.DeclareInputs(os.Getenv, ".", "../../../migrations"); err != nil {
		t.Fatal(err)
	}
	child := exec.Command(os.Args[0], "-test.run=^"+regexp.QuoteMeta(t.Name())+"$", "-test.v", "--", schemaRefusalChild)
	child.Env = append(os.Environ(), CloneDBEnv+"="+name)
	output, err := child.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "--- PASS: "+t.Name()+" (") {
		t.Fatalf("the first schema check did not refuse safely: %v\n%s", err, output)
	}
	var riverPresent, futurePresent bool
	if err := owner.QueryRow(ctx, `SELECT to_regclass('public.river_job') IS NOT NULL,
		EXISTS (SELECT 1 FROM schema_migrations_core WHERE version = @version)`,
		pgx.NamedArgs{"version": future.Migrations[0].Version}).Scan(&riverPresent, &futurePresent); err != nil {
		t.Fatal(err)
	}
	if !riverPresent || !futurePresent {
		t.Fatalf("a refused clone was changed: river present=%v, newer migration present=%v", riverPresent, futurePresent)
	}
}

func assertMismatchedCloneRefused(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	cfg, err := pgx.ParseConfig(os.Getenv("MARGINCE_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Database = os.Getenv(CloneDBEnv)
	owner, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing the child connection: %v", err)
		}
	}()
	err = EnsureSchema(ctx, owner)
	if err == nil || !strings.Contains(err.Error(), "refusing to rebuild a declared clone") || !strings.Contains(err.Error(), "records") || !strings.Contains(err.Error(), "make test-db-up") {
		t.Fatalf("schema check = %v, want the migration mismatch and rebuild command", err)
	}
	if schemaReady.Load() {
		t.Fatal("a refused clone was marked ready for application pools")
	}
}
