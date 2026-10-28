// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// recordKindColumns are the columns whose CHECK once listed the kinds of record
// and now also accepts `ext:<unit>:<kind>`. The schema is read back for each
// below, so a listed column that keeps a closed list fails.
var recordKindColumns = []struct{ table, column string }{
	{"activity_link", "entity_type"},
	{"attachment", "entity_type"},
	{"embedding", "entity_type"},
	{"field_provenance", "object_type"},
	{"list", "entity_type"},
	{"list_member", "entity_type"},
	{"list_live_member", "entity_type"},
	{"list_member_event", "entity_type"},
	{"taggable", "entity_type"},
	{"record_grant", "record_type"},
	{"dedupe_candidate", "entity_type"},
	{"user_record_view", "entity_type"},
	{"signal", "entity_type"},
	{"saved_view", "resource"},
}

func TestEveryKindColumnAcceptsAnExtensionKindAndRefusesAMalformedOne(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	ctx := context.Background()
	for _, c := range recordKindColumns {
		var def string
		var validated bool
		err := conn.QueryRow(ctx, `
			SELECT pg_get_constraintdef(con.oid), con.convalidated
			  FROM pg_constraint con
			  JOIN pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = ANY (con.conkey)
			 WHERE con.conrelid = $1::regclass AND con.contype = 'c' AND a.attname = $2
			   AND array_length(con.conkey, 1) = 1
			   AND pg_get_constraintdef(con.oid) LIKE '%ext:%'`, c.table, c.column).Scan(&def, &validated)
		if err != nil {
			t.Errorf("%s.%s: no CHECK names the extension form: %v", c.table, c.column, err)
			continue
		}
		if !validated {
			t.Errorf("%s.%s: the kind CHECK is still NOT VALID at head", c.table, c.column)
		}
		for _, bad := range []string{"ext:Unit:kind", "ext:unit:", "ext:unit:1kind", "ext:unit:a:b", "ext:-unit:kind", "xext:unit:kind"} {
			if kindCheckAccepts(t, conn, def, c.column, bad) {
				t.Errorf("%s.%s accepts the malformed kind %q", c.table, c.column, bad)
			}
		}
		for _, good := range []string{"ext:unit:kind", "ext:visit-log:visit_2"} {
			if !kindCheckAccepts(t, conn, def, c.column, good) {
				t.Errorf("%s.%s refuses the kind %q", c.table, c.column, good)
			}
		}
	}
}

// kindCheckAccepts evaluates the constraint's own expression with the column
// bound to value, so the test reads the pattern the schema holds rather than
// restating it. A constraint that reads other columns is evaluated with them
// null, which is why the kind CHECKs are kept free of them.
func kindCheckAccepts(t *testing.T, conn *pgx.Conn, def, column, value string) bool {
	t.Helper()
	expr := strings.TrimSuffix(strings.TrimPrefix(def, "CHECK "), " NOT VALID")
	var ok *bool
	query := fmt.Sprintf(`SELECT %s FROM (SELECT $1::text AS %s) AS v`, expr, column)
	if err := conn.QueryRow(context.Background(), query, value).Scan(&ok); err != nil {
		t.Fatalf("evaluating %s: %v", def, err)
	}
	return ok == nil || *ok
}

func seedActivityRow(t *testing.T, conn *pgx.Conn) string {
	t.Helper()
	var id string
	if err := conn.QueryRow(context.Background(), `
		INSERT INTO activity (kind, subject, occurred_at, source, captured_by)
		VALUES ('task', 'Kind link', now(), 'manual', 'test') RETURNING id::text`).Scan(&id); err != nil {
		t.Fatalf("seeding the activity: %v", err)
	}
	return id
}

func TestARecordKindLinkNeedsItsIdAndNoCoreForeignKey(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	ctx := context.Background()
	activity := seedActivityRow(t, conn)

	insert := func(entityType string, extID any) error {
		_, err := conn.Exec(ctx, `
			INSERT INTO activity_link (activity_id, entity_type, ext_entity_id) VALUES ($1, $2, $3)`,
			activity, entityType, extID)
		return err
	}
	if err := insert("ext:visit-log:visit", "0197a000-0000-7000-8000-000000000001"); err != nil {
		t.Fatalf("a link to an extension record was refused: %v", err)
	}
	if err := insert("ext:visit-log:visit", "0197a000-0000-7000-8000-000000000001"); err == nil {
		t.Error("the same extension record was linked to one activity twice")
	}
	if err := insert("ext:visit-log:visit", nil); err == nil {
		t.Error("an extension link with no record id was accepted")
	}
	if err := insert("deal", "0197a000-0000-7000-8000-000000000002"); err == nil {
		t.Error("a core link carrying an extension id was accepted")
	}
}

func TestADedupePairOfAnExtensionKindIsOrderedAndUnique(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	ctx := context.Background()
	insert := func(left, right string) error {
		_, err := conn.Exec(ctx, `
			INSERT INTO dedupe_candidate (entity_type, left_ext_id, right_ext_id, confidence, evidence, source, captured_by)
			VALUES ('ext:visit-log:visit', $1, $2, 0.9, '{}', 'test', 'system')`, left, right)
		return err
	}
	const low, high = "0197a000-0000-7000-8000-000000000001", "0197a000-0000-7000-8000-000000000002"
	if err := insert(low, high); err != nil {
		t.Fatalf("an ordered extension pair was refused: %v", err)
	}
	if err := insert(low, high); err == nil {
		t.Error("the same extension pair was recorded twice")
	}
	if err := insert(high, low); err == nil {
		t.Error("an unordered extension pair was accepted")
	}
}
