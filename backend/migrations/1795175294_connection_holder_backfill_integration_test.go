// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

// A task handle and a retry key handed out before the change still answer after
// it. Live ones move to the agent's connection. A passport a human minted keeps
// what it had.
func TestTheConnectionHolderBackfillMovesOnlyLiveConnectedRows(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.CoreHistory()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	through := namespaceThroughVersion(t, core, "1795175294")
	if _, err := dbmigrate.Up(ctx, conn, through); err != nil {
		t.Fatalf("up: %v", err)
	}
	if _, err := dbmigrate.Down(ctx, conn, through, 1); err != nil {
		t.Fatalf("rolling the column back: %v", err)
	}

	user := seedAppUserForConnection(t, conn, "holder@backfill.example")
	grant := seedValue(t, conn, `WITH c AS (
		INSERT INTO oauth_client (client_id, client_name, redirect_uris) VALUES ('backfill', 'Claude', ARRAY['https://client.example/cb']))
		INSERT INTO oauth_grant (client_id, user_id, scopes) VALUES ('backfill', $1, ARRAY['read']) RETURNING id::text`, user)
	connected := seedHolderPassport(t, conn, user, &grant)
	minted := seedHolderPassport(t, conn, user, nil)
	approval := seedValue(t, conn, `INSERT INTO approval (kind, proposed_change, diff_hash, proposed_by, expires_at)
		VALUES ('enrich', '{}', 'h', 'agent', now() + interval '1 day') RETURNING id::text`)
	liveTask := seedValue(t, conn, `INSERT INTO agent_task (approval_id, passport_id, tool, expires_at, created_at)
		VALUES ($1, $2, 'enrich', now() + interval '1 hour', now()) RETURNING id::text`, approval, connected)
	for _, row := range []struct{ holder, key string }{
		{"agent:" + connected, "fresh"}, {"agent:" + minted, "minted"},
	} {
		if _, err := conn.Exec(ctx, `INSERT INTO idempotency_key (principal_id, key, endpoint, request_digest)
			VALUES ($1, $2, 'MCP create_record', 'd')`, row.holder, row.key); err != nil {
			t.Fatalf("seeding a claim: %v", err)
		}
	}
	if _, err := conn.Exec(ctx, `INSERT INTO idempotency_key (principal_id, key, endpoint, request_digest, created_at)
		VALUES ($1, 'stale', 'MCP create_record', 'd', now() - interval '2 days')`, "agent:"+connected); err != nil {
		t.Fatalf("seeding a stale claim: %v", err)
	}

	if _, err := dbmigrate.Up(ctx, conn, core); err != nil {
		t.Fatalf("migrating back up: %v", err)
	}
	if got := seedValue(t, conn, `SELECT COALESCE(connection_id::text, '') FROM agent_task WHERE id = $1`, liveTask); got != grant {
		t.Errorf("the live task names connection %q, want the grant %q", got, grant)
	}
	for key, want := range map[string]string{"fresh": "agent:" + grant, "minted": "agent:" + minted, "stale": "agent:" + connected} {
		if got := seedValue(t, conn, `SELECT principal_id FROM idempotency_key WHERE key = $1`, key); got != want {
			t.Errorf("claim %q is held by %q, want %q", key, got, want)
		}
	}
}

func seedHolderPassport(t *testing.T, conn *pgx.Conn, user string, grant *string) string {
	t.Helper()
	return seedValue(t, conn, `INSERT INTO passport (on_behalf_of, granted_by, label, scopes, token_hash, expires_at, oauth_grant_id)
		VALUES ($1, $1, 'backfill', ARRAY['read'], gen_random_uuid()::text, now() + interval '1 day', $2) RETURNING id::text`, user, grant)
}

// seedValue runs one statement and answers the single text value it returns.
func seedValue(t *testing.T, conn *pgx.Conn, sql string, args ...any) string {
	t.Helper()
	var value string
	if err := conn.QueryRow(context.Background(), sql, args...).Scan(&value); err != nil {
		t.Fatalf("running %q: %v", sql, err)
	}
	return value
}
