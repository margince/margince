// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

const replyTopicRegroupUp = "datafix/2026-11-03_a_reply_on_a_new_topic_is_its_own_conversation.up.sql"

// storedMail seeds one captured email under threadKey that references the given ids,
// the rows live capture writes: the activity, its mail identity and its references.
func storedMail(ctx context.Context, t *testing.T, conn *pgx.Conn, messageID, subject, threadKey, at string, references ...string) string {
	t.Helper()
	var id string
	if err := conn.QueryRow(ctx, `
		INSERT INTO activity (kind, subject, direction, occurred_at, source_system, source_id, source, captured_by, thread_key)
		VALUES ('email', $2, 'inbound', $4::timestamptz, 'email', $1, 'gmail', 'connector:gmail', $3) RETURNING id`,
		messageID, subject, threadKey, at).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO activity_identity (activity_id, identity_kind, identity_key, attested_by)
		VALUES ($1, 'mail', $2, 'connector:gmail')`, id, messageID); err != nil {
		t.Fatal(err)
	}
	for _, ref := range references {
		if _, err := conn.Exec(ctx, `INSERT INTO activity_mail_reference (activity_id, referenced_id) VALUES ($1, $2)`, id, ref); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func storedThreadKey(ctx context.Context, t *testing.T, conn *pgx.Conn, messageID string) string {
	t.Helper()
	var key string
	if err := conn.QueryRow(ctx, `SELECT thread_key FROM activity WHERE source_id = $1`, messageID).Scan(&key); err != nil {
		t.Fatal(err)
	}
	return key
}

func TestHistoryRegroupsAReplyOnANewTopicIntoItsOwnConversation(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()

	var seat string
	if err := conn.QueryRow(ctx,
		`INSERT INTO app_user (email, display_name) VALUES ('rep@regroup.test', 'Rep') RETURNING id`).Scan(&seat); err != nil {
		t.Fatal(err)
	}
	const root = "opener@x.test"
	storedMail(ctx, t, conn, root, "Angebot", root, "2026-06-01T08:00:00Z")
	storedMail(ctx, t, conn, "answer@x.test", "Re: Angebot", root, "2026-06-02T08:00:00Z", root)
	storedMail(ctx, t, conn, "invoice@x.test", "Rechnung März", root, "2026-06-03T08:00:00Z", root, "answer@x.test")
	storedMail(ctx, t, conn, "paid@x.test", "RE:  rechnung märz", root, "2026-06-04T08:00:00Z", root, "answer@x.test", "invoice@x.test")
	storedMail(ctx, t, conn, "blank@x.test", "Re:", root, "2026-06-05T08:00:00Z", root, "invoice@x.test", "paid@x.test")
	// A reply naming a message of another conversation is not that conversation's to split.
	storedMail(ctx, t, conn, "elsewhere@x.test", "Ganz anders", "other-root@x.test", "2026-06-06T08:00:00Z", "answer@x.test")
	// A reply whose sender named it after another conversation's key stays with its parent.
	storedMail(ctx, t, conn, "member@x.test", "Re: Vertrag", "ghost@x.test", "2026-06-02T09:00:00Z", "ghost@x.test")
	storedMail(ctx, t, conn, "ghost@x.test", "Re: Etwas", root, "2026-06-07T08:00:00Z", root, "answer@x.test")
	if _, err := conn.Exec(ctx, `INSERT INTO capture_thread_verdict (thread_key, user_id, status, kind)
		VALUES ($1, $2, 'held_by_owner', 'personal')`, root, seat); err != nil {
		t.Fatal(err)
	}

	execFile(ctx, t, conn, replyTopicRegroupUp)

	want := map[string]string{
		root: root, "answer@x.test": root,
		"invoice@x.test": "invoice@x.test", "paid@x.test": "invoice@x.test", "blank@x.test": "invoice@x.test",
		"elsewhere@x.test": "other-root@x.test", "member@x.test": "ghost@x.test", "ghost@x.test": root,
	}
	for message, key := range want {
		if got := storedThreadKey(ctx, t, conn, message); got != key {
			t.Errorf("%s is in conversation %q, want %q", message, got, key)
		}
	}
	var status string
	if err := conn.QueryRow(ctx, `SELECT status FROM capture_thread_verdict WHERE thread_key = 'invoice@x.test' AND user_id = $1`,
		seat).Scan(&status); err != nil || status != "held_by_owner" {
		t.Errorf("the split conversation's verdict is %q (%v), want the held verdict carried", status, err)
	}
	countAudits := func() int {
		var n int
		if err := conn.QueryRow(ctx, `SELECT count(*) FROM audit_log
			WHERE after->>'datafix' = '2026-11-03_a_reply_on_a_new_topic_is_its_own_conversation'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := countAudits(); n != 3 {
		t.Errorf("%d audit rows, want one per moved message (3)", n)
	}

	execFile(ctx, t, conn, replyTopicRegroupUp)
	if n := countAudits(); n != 3 {
		t.Errorf("a second run wrote %d audit rows in total, want the first run's 3 and nothing more", n)
	}
}
