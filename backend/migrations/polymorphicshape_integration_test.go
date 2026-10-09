// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// A row named through a polymorphic pair goes with the record the pair names.
//
// Each table here keeps its type-and-id pair and carries one key column per type,
// generated from the pair. Deleting the record takes the row. A pair whose id is a
// record of ANOTHER type is refused. Both halves, since a key that refused every
// row would pass the cascade half by never holding one.

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestARecordGrantGoesWithItsRecord(t *testing.T) {
	owner := shapeConn(t)
	user := seedShapeUser(t, owner, "grant")
	contact := seedAppliedFieldContact(t, owner, "Grete Lund")

	insert := func(recordType string) error {
		_, err := owner.Exec(context.Background(), `
			INSERT INTO record_grant (record_type, record_id, subject_type, subject_id, access, granted_by)
			VALUES ($1, $2, 'user', $3, 'read', $3)`, recordType, contact, user)
		return err
	}
	expectRefusedAs(t, insert("deal"), "record_grant_deal_id_fkey")
	if err := insert("contact"); err != nil {
		t.Fatalf("granting a contact that exists was refused: %v", err)
	}
	deleteAndExpectGone(t, owner, `DELETE FROM contact WHERE id = $1`, contact,
		`SELECT count(*) FROM record_grant WHERE record_id = $1`)
}

func TestAForecastShareGoesWithItsScope(t *testing.T) {
	owner := shapeConn(t)
	user := seedShapeUser(t, owner, "share")
	var team string
	if err := owner.QueryRow(context.Background(),
		`INSERT INTO team (name) VALUES ('Nordics') RETURNING id::text`).Scan(&team); err != nil {
		t.Fatalf("seeding the team: %v", err)
	}

	insert := func(scopeKind string) error {
		_, err := owner.Exec(context.Background(), `
			INSERT INTO analytics_share
			  (kind, target, scope_kind, scope_id, token_hash, created_by, expires_at, captured_by)
			VALUES ('live', 'forecast', $1, $2, gen_random_uuid()::text, $3, now() + interval '1 day', 'human:seed')`,
			scopeKind, team, user)
		return err
	}
	expectRefusedAs(t, insert("owner"), "analytics_share_scope_user_id_fkey")
	if err := insert("team"); err != nil {
		t.Fatalf("sharing a team scope that exists was refused: %v", err)
	}
	deleteAndExpectGone(t, owner, `DELETE FROM team WHERE id = $1`, team,
		`SELECT count(*) FROM analytics_share WHERE scope_id = $1`)
}

func shapeConn(t *testing.T) *pgx.Conn {
	t.Helper()
	ownerDSN, _ := dsns(t)
	owner := connect(t, ownerDSN)
	headSchema(t, owner)
	return owner
}

func seedShapeUser(t *testing.T, conn *pgx.Conn, label string) string {
	t.Helper()
	var id string
	if err := conn.QueryRow(context.Background(), `
		INSERT INTO app_user (display_name, email)
		VALUES ($1, gen_random_uuid()::text || '@shape.test') RETURNING id::text`, label).Scan(&id); err != nil {
		t.Fatalf("seeding the user: %v", err)
	}
	return id
}

// expectRefusedAs holds a refusal to the key that made it. A row refused for some
// other column must not pass as the pair being checked.
func expectRefusedAs(t *testing.T, err error, constraint string) {
	t.Helper()
	if err == nil {
		t.Fatalf("a pair whose id is a record of another type was accepted; %s should refuse it", constraint)
	}
	if !strings.Contains(err.Error(), constraint) {
		t.Fatalf("refused by something other than %s: %v", constraint, err)
	}
}

func deleteAndExpectGone(t *testing.T, conn *pgx.Conn, deleteRecord, record, countRows string) {
	t.Helper()
	ctx := context.Background()
	var held, left int
	if err := conn.QueryRow(ctx, countRows, record).Scan(&held); err != nil {
		t.Fatalf("counting the rows that name it: %v", err)
	}
	if held != 1 {
		t.Fatalf("%d row(s) name the record before the delete, want the one just written", held)
	}
	if _, err := conn.Exec(ctx, deleteRecord, record); err != nil {
		t.Fatalf("deleting the record: %v", err)
	}
	if err := conn.QueryRow(ctx, countRows, record).Scan(&left); err != nil {
		t.Fatalf("counting the rows that named it: %v", err)
	}
	if left != 0 {
		t.Errorf("%d row(s) still name the deleted record", left)
	}
}
