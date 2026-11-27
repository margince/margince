// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Catalogs must exist before a connector boots. Unregistered kinds are refused
// by their database foreign key.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/compose/integration"
)

func TestActivityKindAndChannelProviderAreSeededByMigration(t *testing.T) {
	integration.Setup(t) // triggers EnsureSchema — the migration this test asserts on
	owner := integration.OwnerConn(t)
	ctx := context.Background()

	rows, err := owner.Query(ctx, `SELECT kind FROM activity_kind ORDER BY kind`)
	if err != nil {
		t.Fatalf("querying activity_kind: %v", err)
	}
	kinds, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("reading activity kinds: %v", err)
	}
	// Interaction kinds stay separate from transport registrations.
	want := []string{"call", "email", "meeting", "message", "note", "task"}
	if len(kinds) != len(want) {
		t.Fatalf("activity_kind seeded %v, want %v", kinds, want)
	}
	for i, k := range want {
		if kinds[i] != k {
			t.Fatalf("activity_kind seeded %v, want %v", kinds, want)
		}
	}

	provRows, err := owner.Query(ctx, `SELECT provider || '/' || transport FROM channel_provider ORDER BY provider`)
	if err != nil {
		t.Fatalf("querying channel_provider: %v", err)
	}
	providers, err := pgx.CollectRows(provRows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("reading channel providers: %v", err)
	}
	// Core transports remain seed obligations; the registry also admits later
	// migrations and unit-owned transports. Core display names must resolve too.
	required := map[string]bool{"dispact/core": true, "telegram/core": true, "whatsapp/core": true, "zalo_oa/core": true}
	for provider := range coreProviderLabels {
		required[provider+"/core"] = true
	}
	for _, provider := range providers {
		delete(required, provider)
	}
	if len(required) != 0 {
		t.Fatalf("channel_provider is missing core transports: %v (seeded %v)", required, providers)
	}
}

// The FK is what does the real work: an unregistered kind is refused by the
// database, not by an application-side list somebody has to remember.
func TestActivityKindFKRefusesAnUnregisteredKind(t *testing.T) {
	ctx := context.Background()

	_, err := integration.OwnerConn(t).Exec(ctx, `
		INSERT INTO activity (kind, source, captured_by)
		VALUES ('dispact', 'manual', 'test')`)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != "activity_kind_fkey" {
		t.Fatalf("insert failed with %v, want a foreign_key_violation on activity_kind_fkey specifically — "+
			"any other failure (a bad column, an RLS refusal) would pass this test for the wrong reason", err)
	}
}
