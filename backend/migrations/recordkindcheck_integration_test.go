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
// and now also accepts `ext:<unit>:<kind>`, because a writer puts one there.
// The schema is read back for each below, so a listed column that keeps a
// closed list fails.
var recordKindColumns = []struct{ table, column string }{
	{"list", "entity_type"},
	{"list_member", "entity_type"},
	{"list_live_member", "entity_type"},
	{"list_member_event", "entity_type"},
	{"taggable", "entity_type"},
	{"record_grant", "record_type"},
	{"saved_view", "resource"},
}

// coreOnlyKindColumns name a kind of record that no writer fills with an
// extension's kind, so their CHECK keeps core's closed list.
var coreOnlyKindColumns = []struct{ table, column, core string }{
	{"attachment", "entity_type", "contact"},
	{"embedding", "entity_type", "contact"},
	{"field_provenance", "object_type", "contact"},
	{"dedupe_candidate", "entity_type", "contact"},
	{"user_record_view", "entity_type", "contact"},
	{"signal", "entity_type", "contact"},
}

func TestEveryKindColumnAcceptsAnExtensionKindAndRefusesAMalformedOne(t *testing.T) {
	t.Parallel()
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

func TestACoreOnlyKindColumnRefusesAnExtensionKind(t *testing.T) {
	t.Parallel()
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	ctx := context.Background()
	for _, c := range coreOnlyKindColumns {
		var def string
		var validated bool
		err := conn.QueryRow(ctx, `
			SELECT pg_get_constraintdef(con.oid), con.convalidated
			  FROM pg_constraint con
			  JOIN pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = ANY (con.conkey)
			 WHERE con.conrelid = $1::regclass AND con.contype = 'c' AND a.attname = $2
			   AND con.conname = $3`, c.table, c.column, c.table+"_"+c.column+"_check").Scan(&def, &validated)
		if err != nil {
			t.Errorf("%s.%s: no kind CHECK: %v", c.table, c.column, err)
			continue
		}
		if !validated {
			t.Errorf("%s.%s: the kind CHECK is still NOT VALID at head", c.table, c.column)
		}
		if kindCheckAccepts(t, conn, def, c.column, "ext:demo:thing") {
			t.Errorf("%s.%s accepts the extension kind ext:demo:thing, which no writer puts there", c.table, c.column)
		}
		if !kindCheckAccepts(t, conn, def, c.column, c.core) {
			t.Errorf("%s.%s refuses the core kind %q", c.table, c.column, c.core)
		}
	}
}

func TestADedupeCandidateHasNoExtensionPairColumns(t *testing.T) {
	t.Parallel()
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	var left int
	if err := conn.QueryRow(context.Background(), `
		SELECT count(*) FROM information_schema.columns
		 WHERE table_schema = 'public' AND table_name = 'dedupe_candidate'
		   AND column_name IN ('left_ext_id', 'right_ext_id')`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Errorf("dedupe_candidate still has %d extension pair column(s)", left)
	}
}
