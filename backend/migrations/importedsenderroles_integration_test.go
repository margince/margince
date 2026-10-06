// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"fmt"
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

func importedActivity(ctx context.Context, t *testing.T, conn *pgx.Conn, seat, kind, sourceID string) string {
	t.Helper()
	var id string
	if err := conn.QueryRow(ctx, `
		INSERT INTO activity (kind, subject, direction, occurred_at, source_system, source_id, source, captured_by)
		VALUES ($1, 'hi', 'inbound', now(), 'mirror:importfix', $2, 'importfix:seed',
		        'connector:gmail:' || $3) RETURNING id`, kind, sourceID, seat).Scan(&id); err != nil {
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

type participantRow struct {
	role, contact, user, address string
}

func readParticipants(ctx context.Context, t *testing.T, conn *pgx.Conn, activity string) []participantRow {
	t.Helper()
	rows, err := conn.Query(ctx, `
		SELECT role, coalesce(contact_id::text, ''), coalesce(user_id::text, ''), coalesce(address, '')
		  FROM activity_participant WHERE activity_id = $1
		 ORDER BY role, contact_id::text, user_id::text, address`, activity)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []participantRow
	for rows.Next() {
		var r participantRow
		if err := rows.Scan(&r.role, &r.contact, &r.user, &r.address); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func assertParticipants(t *testing.T, name string, got, want []participantRow) {
	t.Helper()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("%s: participant rows\n got %v\nwant %v", name, got, want)
	}
}

func auditCount(ctx context.Context, t *testing.T, conn *pgx.Conn, activity string) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(ctx, `
		SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1`,
		activity).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// The pre-fix writer's damage: every linked contact a sender. The repair
// moves each contact to the role its address holds in the stated headers and
// deletes or moves NOTHING else — the bare header rows and the seat row are
// the fixed writer's own shape and stay exactly as written.
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
	damaged := importedActivity(ctx, t, conn, seat, "email", "damaged")
	for _, contact := range []string{sender, colleague, unnamed} {
		participant(ctx, t, conn, damaged, "from", nil, &contact, nil)
	}
	participant(ctx, t, conn, damaged, "from", nil, nil, s("sender@customer.test"))
	participant(ctx, t, conn, damaged, "to", nil, nil, s("rep@importfix.test"))
	participant(ctx, t, conn, damaged, "to", nil, nil, s("colleague@customer.test"))
	participant(ctx, t, conn, damaged, "bcc", nil, nil, s("quiet@customer.test"))
	participant(ctx, t, conn, damaged, "to", &seat, nil, nil)

	execFile(ctx, t, conn, importedSenderRolesUp)
	execFile(ctx, t, conn, importedSenderRolesUp)

	assertParticipants(t, "repaired", readParticipants(ctx, t, conn, damaged), []participantRow{
		{"bcc", "", "", "quiet@customer.test"},
		// The contact the headers do not name keeps the receiving side of an
		// inbound message without becoming the sender the headers deny.
		{"cc", unnamed, "", ""},
		{"from", sender, "", ""},
		{"from", "", "", "sender@customer.test"},
		{"to", colleague, "", ""},
		{"to", "", seat, ""},
		{"to", "", "", "colleague@customer.test"},
		{"to", "", "", "rep@importfix.test"},
	})
	if n := auditCount(ctx, t, conn, damaged); n != 1 {
		t.Errorf("audit rows for the repaired email: got %d, want 1 (two runs, one change)", n)
	}
}

// What the repair must not touch, each control one rule.
func TestTheSenderRoleFixLeavesTheRestAlone(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()

	var seat string
	if err := conn.QueryRow(ctx,
		`INSERT INTO app_user (email, display_name) VALUES ('rep@importcontrol.test', 'Rep') RETURNING id`).Scan(&seat); err != nil {
		t.Fatal(err)
	}
	s := func(v string) *string { return &v }
	seeded := map[string][]participantRow{}
	keep := func(name, activity string) {
		seeded[name] = readParticipants(ctx, t, conn, activity)
	}
	activities := map[string]string{}

	// An email stating no headers keeps the old behaviour, as the fixed
	// writer does.
	bare := namedContact(ctx, t, conn, "Bare", "bare@control.test")
	headerless := importedActivity(ctx, t, conn, seat, "email", "headerless")
	participant(ctx, t, conn, headerless, "from", nil, &bare, nil)
	activities["headerless"] = headerless

	// Mail the FIXED writer logged: the contact already sits on its header
	// role beside the bare rows. In scope, and a no-op.
	fixed := namedContact(ctx, t, conn, "Fixed", "fixed@control.test")
	correct := importedActivity(ctx, t, conn, seat, "email", "correct")
	participant(ctx, t, conn, correct, "from", nil, &fixed, nil)
	participant(ctx, t, conn, correct, "from", nil, nil, s("fixed@control.test"))
	participant(ctx, t, conn, correct, "to", nil, nil, s("rep@importcontrol.test"))
	activities["the fixed writer's mail"] = correct

	// An address that ever belonged to two contacts proves nothing about who
	// sent mail then, so the whole activity waits for a human.
	first := namedContact(ctx, t, conn, "First Holder", "moved@control.test")
	if _, err := conn.Exec(ctx,
		`UPDATE contact_email SET archived_at = now() WHERE contact_id = $1`, first); err != nil {
		t.Fatal(err)
	}
	second := namedContact(ctx, t, conn, "Second Holder", "moved@control.test")
	moved := importedActivity(ctx, t, conn, seat, "email", "moved")
	participant(ctx, t, conn, moved, "from", nil, &first, nil)
	participant(ctx, t, conn, moved, "from", nil, &second, nil)
	participant(ctx, t, conn, moved, "from", nil, nil, s("moved@control.test"))
	activities["a moved address"] = moved

	// A call is not this fix's kind, however its rows look.
	caller := namedContact(ctx, t, conn, "Caller", "caller@control.test")
	call := importedActivity(ctx, t, conn, seat, "call", "call")
	participant(ctx, t, conn, call, "from", nil, &caller, nil)
	participant(ctx, t, conn, call, "to", nil, nil, s("caller@control.test"))
	activities["a call"] = call

	for name, activity := range activities {
		keep(name, activity)
	}

	execFile(ctx, t, conn, importedSenderRolesUp)

	for name, activity := range activities {
		assertParticipants(t, name, readParticipants(ctx, t, conn, activity), seeded[name])
		if n := auditCount(ctx, t, conn, activity); n != 0 {
			t.Errorf("%s gained %d audit rows", name, n)
		}
	}
}
