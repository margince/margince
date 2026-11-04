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

// A vouch recorded before vouches named an entity goes to the entity of the
// seat that first recorded it. That seat is read from the stamp the writer
// leaves: a human's user, or the user an agent's passport works for. A carried row
// follows its chain's first recorder. A row whose recorder nobody can name is
// revoked, never handed to the default entity.
func TestTheVouchEntityBackfillFollowsTheRecorderAndRevokesTheUnnamed(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.Core()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	through := namespaceThroughVersion(t, core, "1793823200")
	if _, err := dbmigrate.Up(ctx, conn, through); err != nil {
		t.Fatalf("up: %v", err)
	}
	if _, err := dbmigrate.Down(ctx, conn, through, 1); err != nil {
		t.Fatalf("rolling the entity column back: %v", err)
	}

	branchRep := seedAppUserForConnection(t, conn, "branch@vouch.example")
	defaultRep := seedAppUserForConnection(t, conn, "default@vouch.example")
	branch, defaultEntity := vouchEntities(t, conn, branchRep)
	passport := seedPassport(t, conn, branchRep)
	contact := scalar(t, conn, `INSERT INTO contact (full_name, source, captured_by)
		VALUES ('Dana', 'manual', 'human:x') RETURNING id`)

	byBranch := seedVouch(t, conn, contact, "human:"+branchRep, nil)
	byAgent := seedVouch(t, conn, contact, "agent:"+passport, nil)
	byDefault := seedVouch(t, conn, contact, "human:"+defaultRep, nil)
	carried := seedVouch(t, conn, contact, "human:"+defaultRep, &byBranch)
	unnamed := seedVouch(t, conn, contact, "system:import", nil)

	if _, err := dbmigrate.Up(ctx, conn, core); err != nil {
		t.Fatalf("migrating back up: %v", err)
	}
	for id, want := range map[string]string{byBranch: branch, byAgent: branch, byDefault: defaultEntity, carried: branch} {
		var entity string
		var revoked bool
		if err := conn.QueryRow(ctx, `SELECT legal_entity_id, revoked_at IS NOT NULL
			FROM communication_override WHERE id = $1`, id).Scan(&entity, &revoked); err != nil {
			t.Fatal(err)
		}
		if entity != want || revoked {
			t.Errorf("vouch %s: entity %s revoked %v, want entity %s and live", id, entity, revoked, want)
		}
	}
	var revoked bool
	if err := conn.QueryRow(ctx, `SELECT revoked_at IS NOT NULL FROM communication_override WHERE id = $1`,
		unnamed).Scan(&revoked); err != nil {
		t.Fatal(err)
	}
	if !revoked {
		t.Error("a vouch with no nameable recorder stayed live: it must be revoked, not given the default entity")
	}
}

func vouchEntities(t *testing.T, conn *pgx.Conn, branchRep string) (branch, defaultEntity string) {
	t.Helper()
	branch = scalar(t, conn, `INSERT INTO legal_entity (display_name, legal_name, country, captured_by)
		VALUES ('Branch', 'Branch Ltd', 'at', 'human:x') RETURNING id`)
	if _, err := conn.Exec(context.Background(), `INSERT INTO legal_entity_member (legal_entity_id, user_id, assigned_by)
		VALUES ($1, $2, 'human:x')`, branch, branchRep); err != nil {
		t.Fatalf("assigning the branch rep: %v", err)
	}
	return branch, scalar(t, conn, `SELECT id FROM legal_entity WHERE is_default`)
}

func seedPassport(t *testing.T, conn *pgx.Conn, user string) string {
	t.Helper()
	return scalar(t, conn, `INSERT INTO passport (granted_by, on_behalf_of, token_hash, scopes, expires_at)
		VALUES ($1, $1, 'hash-'||gen_random_uuid()::text, '{read}', now() + interval '1 day') RETURNING id`, user)
}

func seedVouch(t *testing.T, conn *pgx.Conn, contact, capturedBy string, carriedFrom *string) string {
	t.Helper()
	return scalar(t, conn, `INSERT INTO communication_override
		(contact_id, category, reason, decided_by_level, captured_by, carried_from)
		VALUES ($1, 'marketing', 'asked on the call', 'user', $2, $3) RETURNING id`, contact, capturedBy, carriedFrom)
}

func scalar(t *testing.T, conn *pgx.Conn, sql string, args ...any) string {
	t.Helper()
	var out string
	if err := conn.QueryRow(context.Background(), sql, args...).Scan(&out); err != nil {
		t.Fatalf("fixture %q: %v", sql, err)
	}
	return out
}
