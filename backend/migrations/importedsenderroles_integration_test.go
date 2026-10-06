// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

const importedSenderRolesUp = "datafix/2026-10-06_an_imported_emails_sender_is_who_its_headers_name.up.sql"

// namedContact seeds a contact holding one live address.
func namedContact(ctx context.Context, t *testing.T, conn *pgx.Conn, name, email string) string {
	t.Helper()
	var id string
	if err := conn.QueryRow(ctx, `
WITH c AS (
  INSERT INTO contact (full_name, source, captured_by) VALUES ($1, 'gmail', 'connector:gmail') RETURNING id
)
INSERT INTO contact_email (contact_id, email, source, captured_by)
SELECT id, $2, 'gmail', 'connector:gmail' FROM c RETURNING contact_id`, name, email).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// importedEmail seeds an inbound email the way the pre-fix logged writer and a
// mail capture together left one: every linked contact as a `from` row, the
// stated headers as bare address rows, and the mailbox owner as a user row.
func importedEmail(ctx context.Context, t *testing.T, conn *pgx.Conn, seat, sourceID string) string {
	t.Helper()
	var id string
	if err := conn.QueryRow(ctx, `
		INSERT INTO activity (kind, subject, direction, occurred_at, source_system, source_id, source, captured_by)
		VALUES ('email', 'hi', 'inbound', now(), 'mirror:importfix', $2, 'importfix:seed',
		        'connector:gmail:' || $1) RETURNING id`, seat, sourceID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func participant(ctx context.Context, t *testing.T, conn *pgx.Conn, activity, role string, userID, contactID, address *string) {
	t.Helper()
	if _, err := conn.Exec(ctx, `
		INSERT INTO activity_participant (activity_id, role, user_id, contact_id, address)
		VALUES ($1, $2, $3, $4, $5)`, activity, role, userID, contactID, address); err != nil {
		t.Fatal(err)
	}
}

func TestAnImportedEmailsSenderIsWhoItsHeadersName(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()

	var seat string
	if err := conn.QueryRow(ctx,
		`INSERT INTO app_user (email, display_name) VALUES ('rep@importfix.test', 'Rep') RETURNING id`).Scan(&seat); err != nil {
		t.Fatal(err)
	}
	sender := namedContact(ctx, t, conn, "Sender", "sender@customer.test")
	colleague := namedContact(ctx, t, conn, "Colleague", "colleague@customer.test")
	unnamed := namedContact(ctx, t, conn, "Unnamed", "unnamed@customer.test")

	s := func(v string) *string { return &v }
	damaged := importedEmail(ctx, t, conn, seat, "damaged")
	// The pre-fix writer's rows: every linked contact a sender.
	participant(ctx, t, conn, damaged, "from", nil, &sender, nil)
	participant(ctx, t, conn, damaged, "from", nil, &colleague, nil)
	participant(ctx, t, conn, damaged, "from", nil, &unnamed, nil)
	// The stated headers, as the importer supplied them.
	participant(ctx, t, conn, damaged, "from", nil, nil, s("sender@customer.test"))
	participant(ctx, t, conn, damaged, "to", nil, nil, s("rep@importfix.test"))
	participant(ctx, t, conn, damaged, "to", nil, nil, s("colleague@customer.test"))
	// The mailbox owner's own capture of it.
	participant(ctx, t, conn, damaged, "to", &seat, nil, nil)

	// An email stating no headers keeps the old behaviour, as the fixed
	// writer does.
	headerless := importedEmail(ctx, t, conn, seat, "headerless")
	participant(ctx, t, conn, headerless, "from", nil, &sender, nil)
	participant(ctx, t, conn, headerless, "from", nil, &colleague, nil)

	execFile(ctx, t, conn, importedSenderRolesUp)
	execFile(ctx, t, conn, importedSenderRolesUp)

	type row struct {
		role, contact, user, address string
	}
	read := func(activity string) []row {
		rows, err := conn.Query(ctx, `
			SELECT role, coalesce(contact_id::text, ''), coalesce(user_id::text, ''), coalesce(address, '')
			  FROM activity_participant WHERE activity_id = $1
			 ORDER BY role, contact_id::text, user_id::text, address`, activity)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.role, &r.contact, &r.user, &r.address); err != nil {
				t.Fatal(err)
			}
			out = append(out, r)
		}
		return out
	}

	want := []row{
		// The contact the headers do not name keeps the receiving side of an
		// inbound message without becoming a recipient the headers deny.
		{"cc", unnamed, "", ""},
		{"from", sender, "", "sender@customer.test"},
		{"to", colleague, "", "colleague@customer.test"},
		{"to", "", seat, ""},
	}
	got := read(damaged)
	if len(got) != len(want) {
		t.Fatalf("repaired rows: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("repaired row %d: got %v, want %v", i, got[i], want[i])
		}
	}

	untouched := read(headerless)
	if len(untouched) != 2 || untouched[0].role != "from" || untouched[1].role != "from" {
		t.Errorf("an email without stated headers was rewritten: %v", untouched)
	}

	var audited int
	if err := conn.QueryRow(ctx, `
		SELECT count(*) FROM audit_log
		 WHERE entity_type = 'activity' AND entity_id = $1
		   AND after->>'datafix' = '2026-10-06_an_imported_emails_sender_is_who_its_headers_name'`,
		damaged).Scan(&audited); err != nil {
		t.Fatal(err)
	}
	if audited != 1 {
		t.Errorf("audit rows for the repaired email: got %d, want 1 (two runs, one change)", audited)
	}
	if err := conn.QueryRow(ctx, `
		SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1`,
		headerless).Scan(&audited); err != nil {
		t.Fatal(err)
	}
	if audited != 0 {
		t.Errorf("the untouched email gained %d audit rows", audited)
	}
}
