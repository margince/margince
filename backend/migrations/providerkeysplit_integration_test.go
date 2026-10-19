// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

const providerKeySplitMigration = "core/1792391226_each_provider_key_is_its_own_setting"

func providerKeyRows(ctx context.Context, t *testing.T, conn *pgx.Conn) map[string]string {
	t.Helper()
	rows, err := conn.Query(ctx, `SELECT key, value::text FROM setting WHERE key LIKE 'ai.provider_keys%'`)
	if err != nil {
		t.Fatalf("reading the provider key rows: %v", err)
	}
	got, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) ([2]string, error) {
		var kv [2]string
		err := row.Scan(&kv[0], &kv[1])
		return kv, err
	})
	if err != nil {
		t.Fatalf("reading the provider key rows: %v", err)
	}
	out := map[string]string{}
	for _, kv := range got {
		out[kv[0]] = kv[1]
	}
	return out
}

// Each provider's ref moves to its own row unchanged, so every sealed blob is
// still named. An empty ref names nothing and does not move.
func TestEachProviderKeyRefMovesToItsOwnRow(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()
	if _, err := conn.Exec(ctx, `INSERT INTO setting (key, value) VALUES
		('ai.provider_keys', '{"gemini":"mgv.1.ws.aaa","gemini_vertex":"mgv.1.ws.bbb","openai":""}')`); err != nil {
		t.Fatalf("seeding the map row: %v", err)
	}

	runFile(ctx, t, conn, providerKeySplitMigration+".up.sql")

	want := map[string]string{
		"ai.provider_keys_gemini":        `"mgv.1.ws.aaa"`,
		"ai.provider_keys_gemini_vertex": `"mgv.1.ws.bbb"`,
	}
	got := providerKeyRows(ctx, t, conn)
	if len(got) != len(want) {
		t.Fatalf("rows after the split = %v, want %v", got, want)
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("%s = %s, want %s", key, got[key], value)
		}
	}

	runFile(ctx, t, conn, providerKeySplitMigration+".down.sql")
	back := providerKeyRows(ctx, t, conn)
	if len(back) != 1 || back["ai.provider_keys"] != `{"gemini": "mgv.1.ws.aaa", "gemini_vertex": "mgv.1.ws.bbb"}` {
		t.Errorf("rows after the way down = %v, want the one map row", back)
	}
}

// An installation that never stored a key has no map row, and the migration
// touches nothing.
func TestTheProviderKeySplitWithNoMapRowDoesNothing(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()

	runFile(ctx, t, conn, providerKeySplitMigration+".up.sql")

	if got := providerKeyRows(ctx, t, conn); len(got) != 0 {
		t.Errorf("rows after a split of nothing = %v, want none", got)
	}
}

// The split applied twice, and a round trip down and up again, leave every
// ref as it was.
func TestTheProviderKeySplitIsRepeatableAndRoundTrips(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()
	if _, err := conn.Exec(ctx, `INSERT INTO setting (key, value) VALUES
		('ai.provider_keys', '{"anthropic":"mgv.1.ws.ccc","openai_compatible":"mgv.1.ws.ddd"}')`); err != nil {
		t.Fatalf("seeding the map row: %v", err)
	}
	want := map[string]string{
		"ai.provider_keys_anthropic":         `"mgv.1.ws.ccc"`,
		"ai.provider_keys_openai_compatible": `"mgv.1.ws.ddd"`,
	}
	same := func(step string) {
		t.Helper()
		got := providerKeyRows(ctx, t, conn)
		if len(got) != len(want) {
			t.Fatalf("rows after %s = %v, want %v", step, got, want)
		}
		for key, value := range want {
			if got[key] != value {
				t.Errorf("after %s, %s = %s, want %s", step, key, got[key], value)
			}
		}
	}

	runFile(ctx, t, conn, providerKeySplitMigration+".up.sql")
	runFile(ctx, t, conn, providerKeySplitMigration+".up.sql")
	same("the split applied twice")

	runFile(ctx, t, conn, providerKeySplitMigration+".down.sql")
	runFile(ctx, t, conn, providerKeySplitMigration+".up.sql")
	same("a round trip down and up")
}
