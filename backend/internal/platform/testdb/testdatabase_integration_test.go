// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestASubtestReadsItsParentsDatabaseAndASecondFixtureGetsAFreshOne(t *testing.T) {
	seeded := New(t)
	t.Run("helper", func(t *testing.T) {
		if got := For(t); got != seeded {
			t.Errorf("For in a subtest answered %s, want the parent's %s", got.Name, seeded.Name)
		}
	})
	if again := New(t); again == seeded {
		t.Errorf("a second fixture in one test was handed the seeded %s again", seeded.Name)
	}
}

func TestAPoolOpenedBeforeTheFixtureSharesItsDatabase(t *testing.T) {
	early := For(t)
	if seeded := New(t); seeded != early {
		t.Errorf("the fixture seeded %s, but the pool opened before it is on %s", seeded.Name, early.Name)
	}
}

func TestATestsDatabaseIsAtHeadEmptyAndDroppedWhenItEnds(t *testing.T) {
	var name string
	t.Run("owner", func(t *testing.T) {
		db := New(t)
		name = db.Name
		conn := connect(t, db.OwnerDSN)
		if reason, err := schemaEmpty(t.Context(), conn); err != nil || reason != "" {
			t.Errorf("a fresh copy is not empty: %s %v", reason, err)
		}
		if reason, err := ledgerAtHead(t.Context(), conn); err != nil || reason != "" {
			t.Errorf("a fresh copy is not at head: %s %v", reason, err)
		}
	})
	maintenance, err := withDatabase(os.Getenv(ownerDSNEnv), "postgres")
	if err != nil {
		t.Fatal(err)
	}
	var exists bool
	if err := connect(t, maintenance).QueryRow(t.Context(),
		`SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`, name).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists || isCopy(name) {
		t.Errorf("%s outlived the test that owned it (on the server: %v)", name, exists)
	}
}

func connect(t *testing.T, dsn string) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(context.Background()); err != nil {
			t.Errorf("closing: %v", err)
		}
	})
	return conn
}
