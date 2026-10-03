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
	original := strings.Join([]string{
		"Delivered-To: rep@grouppost.test",
		"Received: by 2002:a05:7412:8082 with SMTP id f2",
		`From: "'Author' via Info" <info@grouppost.test>`,
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
          'info@grouppost.test', true, (SELECT id FROM rc)) RETURNING id
), p AS (
  INSERT INTO activity_participant (activity_id, role, address) SELECT id, 'from', 'info@grouppost.test' FROM a
)
INSERT INTO capture_import (activity_id, user_id) SELECT id, $1::uuid FROM a RETURNING activity_id`,
		seat, msgID, original).Scan(&id); err != nil {
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
