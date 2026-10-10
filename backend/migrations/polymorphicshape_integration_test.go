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

func TestAnAssuranceTaskItemGoesWithItsSubject(t *testing.T) {
	owner := shapeConn(t)
	item := seedTaskItemFixture(t, owner)

	expectRefusedAs(t, item.insert("offer", false), "assurance_task_item_subject_offer_id_fkey")
	// The writer asks for an unkeyed row; the trigger keys it anyway, or the
	// delete below would leave it behind.
	if err := item.insert("deal", true); err != nil {
		t.Fatalf("a task item on a deal that exists was refused: %v", err)
	}
	deleteAndExpectGone(t, owner, `DELETE FROM deal WHERE id = $1`, item.deal,
		`SELECT count(*) FROM assurance_task_item WHERE subject_id = $1`)
}

// A row written before the keys existed has none, and must stay writable. The
// marker excuses it from the CHECKs, and an update does not key it.
func TestATaskItemWrittenBeforeTheKeysStaysWritable(t *testing.T) {
	owner := shapeConn(t)
	item := seedTaskItemFixture(t, owner)
	err := inTx(t, owner, func(tx pgx.Tx) error {
		ctx := context.Background()
		const trigger = `trg_assurance_task_item_subject_keys`
		if _, err := tx.Exec(ctx, `ALTER TABLE assurance_task_item DISABLE TRIGGER `+trigger); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO assurance_task_item
			  (cycle_id, exception_id, task_activity_id, subject_kind, subject_id, subject_unkeyed)
			VALUES ($1, $2, $3, 'deal', $4, true)`, item.cycle, item.exception, item.task, item.deal); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `ALTER TABLE assurance_task_item ENABLE TRIGGER `+trigger); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`UPDATE assurance_task_item SET state = 'resolved' WHERE subject_id = $1`, item.deal); err != nil {
			return err
		}
		var unkeyed bool
		var keyed *string
		if err := tx.QueryRow(ctx, `
			SELECT subject_unkeyed, subject_deal_id::text FROM assurance_task_item WHERE subject_id = $1`,
			item.deal).Scan(&unkeyed, &keyed); err != nil {
			return err
		}
		if !unkeyed || keyed != nil {
			t.Errorf("an update keyed a row written before the keys (unkeyed=%v, key=%v); "+
				"its subject may be gone, and the foreign key would then refuse the update", unkeyed, keyed)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("updating a task item written before the keys: %v", err)
	}
}

// taskItemFixture is a deal with an open finding about it, a cycle, and the
// task a bundle files under.
type taskItemFixture struct {
	conn                         *pgx.Conn
	deal, cycle, exception, task string
}

// insert files the finding under the task, naming the deal as a subject of
// the given kind.
func (f taskItemFixture) insert(subjectKind string, unkeyed bool) error {
	_, err := f.conn.Exec(context.Background(), `
		INSERT INTO assurance_task_item
		  (cycle_id, exception_id, task_activity_id, subject_kind, subject_id, subject_unkeyed)
		VALUES ($1, $2, $3, $4, $5, $6)`, f.cycle, f.exception, f.task, subjectKind, f.deal, unkeyed)
	return err
}

func seedTaskItemFixture(t *testing.T, conn *pgx.Conn) taskItemFixture {
	t.Helper()
	f := taskItemFixture{conn: conn}
	err := conn.QueryRow(context.Background(), `
WITH p AS (
  INSERT INTO pipeline (name) VALUES ('Shape') RETURNING id
), s AS (
  INSERT INTO stage (pipeline_id, name, position, semantic, win_probability)
  SELECT id, 'Open', 1, 'open', 10 FROM p RETURNING id, pipeline_id
), d AS (
  INSERT INTO deal (pipeline_id, stage_id, name, source, captured_by)
  SELECT s.pipeline_id, s.id, 'Shape deal', 'manual', 'test' FROM s RETURNING id
), c AS (
  INSERT INTO assurance_cycle (scope, opened_by) VALUES ('shape', 'human:seed') RETURNING id
), e AS (
  INSERT INTO assurance_exception (logical_key, type, subject_kind, subject_id, severity, captured_by)
  SELECT 'shape', 'stale_close_date', 'deal', d.id, 'low', 'human:seed' FROM d RETURNING id
), task AS (
  INSERT INTO activity (kind, subject, occurred_at, source, captured_by)
  VALUES ('task', 'Fix the deal', now(), 'manual', 'test') RETURNING id
)
SELECT d.id::text, c.id::text, e.id::text, task.id::text FROM d, c, e, task`).
		Scan(&f.deal, &f.cycle, &f.exception, &f.task)
	if err != nil {
		t.Fatalf("seeding the finding: %v", err)
	}
	return f
}
