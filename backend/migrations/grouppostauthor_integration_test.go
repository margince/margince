// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

const groupPostAuthorUp = "datafix/2026-10-03-5_a_group_post_is_its_authors_mail.up.sql"

// storedGroupPost seeds a group post captured the way capture did before it
// read the author: the group as counterparty and sender row, marked bulk.
func storedGroupPost(ctx context.Context, t *testing.T, conn *pgx.Conn, seat, msgID, author, unsubscribe string) string {
	t.Helper()
	return storedGroupPostAs(ctx, t, conn, seat, msgID, author, unsubscribe,
		`"'Author' via Info" <info@grouppost.test>`, "info@grouppost.test")
}

// storedGroupPostAs is storedGroupPost with the From header and the stored
// counterparty given.
func storedGroupPostAs(
	ctx context.Context, t *testing.T, conn *pgx.Conn, seat, msgID, author, unsubscribe, from, counterparty string,
) string {
	t.Helper()
	original := strings.Join([]string{
		"Delivered-To: rep@grouppost.test",
		"Received: by 2002:a05:7412:8082 with SMTP id f2",
		"From: " + from,
		"To: info@grouppost.test",
		`X-Original-From: "Author" <` + author + `>`,
		"X-Google-Group-Id: 217077506295",
		"List-ID: <info.grouppost.test>",
		"List-Unsubscribe: " + unsubscribe,
		"Message-ID: <" + msgID + ">",
		"", "hello", "",
	}, "\r\n")
	var id string
	if err := conn.QueryRow(ctx, `
WITH rc AS (
  INSERT INTO raw_capture (source_system, source_id, payload) VALUES ('email', $2, to_jsonb($3::text)) RETURNING id
), a AS (
  INSERT INTO activity (kind, subject, direction, occurred_at, source_system, source_id, source, captured_by,
                        counterparty_email, bulk_mail_attested, raw_capture_id)
  VALUES ('email', 'hi', 'inbound', now(), 'gmail', $2, 'gmail:seed', 'connector:gmail:' || $1,
          $4, true, (SELECT id FROM rc)) RETURNING id
), p AS (
  INSERT INTO activity_participant (activity_id, role, address) SELECT id, 'from', 'info@grouppost.test' FROM a
)
INSERT INTO capture_import (activity_id, user_id) SELECT id, $1::uuid FROM a RETURNING activity_id`,
		seat, msgID, original, counterparty).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestAGroupPostIsReReadAsItsAuthorsMail(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()
	var seat string
	if err := conn.QueryRow(ctx,
		`INSERT INTO app_user (email, display_name) VALUES ('rep@grouppost.test', 'Rep') RETURNING id`).Scan(&seat); err != nil {
		t.Fatal(err)
	}
	const google = "<mailto:googlegroups-manage+1+unsubscribe@googlegroups.com>, <https://groups.google.com/a/grouppost.test/group/info/subscribe>"

	authored := storedGroupPost(ctx, t, conn, seat, "g1@customer.test", "henry@customer.test", google)
	newsletter := storedGroupPost(ctx, t, conn, seat, "g2@esp.test", "news@esp.test",
		"<https://esp.test/u?r=groups.google.com>, "+google)

	execFile(ctx, t, conn, groupPostAuthorUp)
	execFile(ctx, t, conn, groupPostAuthorUp)

	for name, c := range map[string]struct {
		id, counterparty, sender string
		bulk                     bool
	}{
		"an author's post":        {authored, "henry@customer.test", "henry@customer.test", false},
		"a newsletter through it": {newsletter, "news@esp.test", "news@esp.test", true},
	} {
		var counterparty, sender string
		var bulk bool
		if err := conn.QueryRow(ctx, `
			SELECT a.counterparty_email, a.bulk_mail_attested, p.address
			  FROM activity a JOIN activity_participant p ON p.activity_id = a.id AND p.role = 'from'
			 WHERE a.id = $1`, c.id).Scan(&counterparty, &bulk, &sender); err != nil {
			t.Fatal(err)
		}
		if counterparty != c.counterparty || sender != c.sender || bulk != c.bulk {
			t.Errorf("%s: counterparty %q, sender %q, bulk %v; want %q, %q, %v",
				name, counterparty, sender, bulk, c.counterparty, c.sender, c.bulk)
		}
	}
}

// The shapes the SQL cannot read the way the Go parser does are left as they
// were captured, and a post is the seat's own when ANY importer wrote it.
func TestTheGroupPostFixLeavesWhatItCannotReadAlone(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()
	seat := func(email string) string {
		var id string
		if err := conn.QueryRow(ctx,
			`INSERT INTO app_user (email, display_name) VALUES ($1, 'Rep') RETURNING id`, email).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	rep, author := seat("rep@grouppost.test"), seat("author.seat@grouppost.test")
	const google = "<mailto:googlegroups-manage+1+unsubscribe@googlegroups.com>"
	const group = `"'Author' via Info" <info@grouppost.test>`

	bareLink := storedGroupPost(ctx, t, conn, rep, "s1@customer.test", "bare@customer.test", google+", https://esp.test/u")
	twoSenders := storedGroupPostAs(ctx, t, conn, rep, "s2@customer.test", "two@customer.test", google,
		group+", other@customer.test", "info@grouppost.test")
	corrected := storedGroupPostAs(ctx, t, conn, rep, "s3@customer.test", "third@customer.test", google,
		group, "someone@else.test")
	ownPost := storedGroupPost(ctx, t, conn, rep, "s4@customer.test", "author.seat@grouppost.test", google)
	if _, err := conn.Exec(ctx, `
		INSERT INTO capture_owner_identity (user_id, kind, value, created_by)
		VALUES ($1, 'address', 'author.seat@grouppost.test', 'test')`, author); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, ownPost, author); err != nil {
		t.Fatal(err)
	}
	// An author's sender row already there: the group's row goes, once.
	merged := storedGroupPost(ctx, t, conn, rep, "s5@customer.test", "merged@customer.test", google)
	if _, err := conn.Exec(ctx, `
		INSERT INTO activity_participant (activity_id, role, address) VALUES ($1, 'from', 'merged@customer.test')`,
		merged); err != nil {
		t.Fatal(err)
	}

	execFile(ctx, t, conn, groupPostAuthorUp)
	execFile(ctx, t, conn, groupPostAuthorUp)

	for name, c := range map[string]struct {
		id, counterparty string
		bulk             bool
	}{
		"a bare unsubscribe link keeps it bulk":   {bareLink, "bare@customer.test", true},
		"a From naming two senders":               {twoSenders, "info@grouppost.test", true},
		"a counterparty somebody already changed": {corrected, "someone@else.test", true},
		"a post one of its importers wrote":       {ownPost, "info@grouppost.test", true},
		"an author sender row already there":      {merged, "merged@customer.test", false},
	} {
		var counterparty string
		var bulk bool
		if err := conn.QueryRow(ctx,
			`SELECT counterparty_email, bulk_mail_attested FROM activity WHERE id = $1`, c.id).Scan(&counterparty, &bulk); err != nil {
			t.Fatal(err)
		}
		if counterparty != c.counterparty || bulk != c.bulk {
			t.Errorf("%s: counterparty %q, bulk %v; want %q, %v", name, counterparty, bulk, c.counterparty, c.bulk)
		}
	}
	var senders, audits int
	if err := conn.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM activity_participant WHERE activity_id = $1 AND role = 'from'),
		       (SELECT count(*) FROM audit_log WHERE entity_id = $1
		         AND after->>'datafix' = '2026-10-03-5_a_group_post_is_its_authors_mail')`,
		merged).Scan(&senders, &audits); err != nil {
		t.Fatal(err)
	}
	if senders != 1 || audits != 1 {
		t.Errorf("the merged post has %d sender rows and %d audit rows, want 1 and 1", senders, audits)
	}
}
