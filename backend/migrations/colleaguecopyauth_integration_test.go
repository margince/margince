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

const colleagueCopyAuthUp = "core/1791786513_a_colleagues_copy_needs_an_authenticated_from.up.sql"

// rereadCopyWithOriginal seeds a copy migration 1791775182 re-read as the
// colleague's, with its stored original when authResults is not empty.
func rereadCopyWithOriginal(ctx context.Context, t *testing.T, conn *pgx.Conn, colleague, sourceID, connector string, authResults ...string) string {
	t.Helper()
	var raw *string
	if len(authResults) > 0 {
		original := strings.Join(append(append([]string{
			"Delivered-To: rep@auth.test",
			"Received: by 2002:a05:6504 with SMTP id l11;",
		}, authResults...), "From: Sam <sam@auth.test>", "To: dana@client.test", "", "hello", ""), "\r\n")
		raw = &original
	}
	var id string
	if err := conn.QueryRow(ctx, `
WITH rc AS (
  INSERT INTO raw_capture (source_system, source_id, payload)
  SELECT 'email', $2, to_jsonb($3::text) WHERE $3::text IS NOT NULL RETURNING id
)
INSERT INTO activity (kind, subject, direction, occurred_at, source_system, source_id, source, captured_by,
                      counterparty_email, sent_by_colleague, raw_capture_id)
VALUES ('email', 'Re: offer', 'outbound', now(), 'gmail', $2, 'gmail:seed', $4, 'dana@client.test', $1,
        (SELECT id FROM rc)) RETURNING id`, colleague, sourceID, raw, connector).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestACopyWithoutAnAuthenticatedFromTurnsBack(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()
	var colleague string
	if err := conn.QueryRow(ctx, `
		INSERT INTO app_user (email, display_name, status) VALUES ('Sam@auth.test', 'Sam', 'invited') RETURNING id`).Scan(&colleague); err != nil {
		t.Fatal(err)
	}
	const gmail = "connector:gmail:seat"
	pass := []string{
		"Authentication-Results: mx.google.com;",
		"       dkim=pass header.i=@auth.test;",
		"       dmarc=pass (p=QUARANTINE sp=QUARANTINE dis=NONE) header.from=auth.test",
	}
	authenticated := rereadCopyWithOriginal(ctx, t, conn, colleague, "auth-1", gmail, pass...)
	failed := rereadCopyWithOriginal(ctx, t, conn, colleague, "auth-2", gmail,
		append([]string{"Authentication-Results: mx.google.com; dmarc=fail header.from=auth.test"}, pass...)...)
	noOriginal := rereadCopyWithOriginal(ctx, t, conn, colleague, "auth-3", gmail)
	overIMAP := rereadCopyWithOriginal(ctx, t, conn, colleague, "auth-4", "connector:imap:seat", pass...)
	smuggled := rereadCopyWithOriginal(ctx, t, conn, colleague, "auth-5", gmail,
		`Authentication-Results: mx.google.com; spf=pass smtp.mailfrom="x; dmarc=pass header.from=auth.test y"@evil.test;`,
		"       dmarc=fail header.from=auth.test")

	attestedSince := rereadCopyWithOriginal(ctx, t, conn, colleague, "auth-6", "connector:imap:seat")
	if _, err := conn.Exec(ctx, `UPDATE activity SET counterparty_outbound_attested = true WHERE id = $1`, attestedSince); err != nil {
		t.Fatal(err)
	}
	wrapped := rereadCopyWithOriginal(ctx, t, conn, colleague, "auth-7", gmail, pass...)
	if _, err := conn.Exec(ctx, `
		UPDATE raw_capture SET payload = jsonb_build_object('encoding', 'base64',
		       'data', encode(convert_to(payload #>> '{}', 'UTF8') || '\x00ff'::bytea, 'base64'))
		 WHERE id = (SELECT raw_capture_id FROM activity WHERE id = $1)`, wrapped); err != nil {
		t.Fatal(err)
	}
	settledBy := settledThrough(ctx, t, conn, colleague, failed)

	execFile(ctx, t, conn, colleagueCopyAuthUp)

	for name, c := range map[string]struct {
		id, direction, counterparty string
	}{
		"Gmail passed DMARC for the From domain": {authenticated, "outbound", "dana@client.test"},
		"the topmost result failed":              {failed, "inbound", "sam@auth.test"},
		"no stored original":                     {noOriginal, "inbound", "sam@auth.test"},
		"captured over IMAP":                     {overIMAP, "inbound", "sam@auth.test"},
		"a pass smuggled in a quoted sender":     {smuggled, "inbound", "sam@auth.test"},
		"attested by the colleague's mailbox":    {attestedSince, "outbound", "dana@client.test"},
		"a base64-wrapped original that passed":  {wrapped, "outbound", "dana@client.test"},
	} {
		var direction, counterparty string
		var stillColleague bool
		if err := conn.QueryRow(ctx, `SELECT direction, counterparty_email, sent_by_colleague IS NOT NULL
			FROM activity WHERE id = $1`, c.id).Scan(&direction, &counterparty, &stillColleague); err != nil {
			t.Fatal(err)
		}
		if direction != c.direction || counterparty != c.counterparty || stillColleague != (c.direction == "outbound") {
			t.Errorf("%s: %s to %q, colleague kept %v; want %s to %q",
				name, direction, counterparty, stillColleague, c.direction, c.counterparty)
		}
	}
	settledBy.assertUndone(ctx, t, conn)
}

// settledByCopy is what a copy left behind while it read as the colleague's
// answer. The colleague is stamped as its sender, and a request was settled
// through it. That request's two reminders are done, one of them by a human.
type settledByCopy struct {
	copyID, colleague, request, machineReminder, humanReminder string
}

func settledThrough(ctx context.Context, t *testing.T, conn *pgx.Conn, colleague, copyID string) settledByCopy {
	t.Helper()
	c := settledByCopy{copyID: copyID, colleague: colleague}
	if _, err := conn.Exec(ctx, `INSERT INTO activity_participant (activity_id, role, user_id) VALUES ($1, 'from', $2)`,
		copyID, colleague); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(ctx, `
		INSERT INTO activity (kind, subject, direction, occurred_at, source, captured_by, counterparty_email)
		VALUES ('email', 'offer', 'inbound', now() - interval '1 hour', 'gmail:seed', 'connector:gmail:seat', 'dana@client.test')
		RETURNING id`).Scan(&c.request); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO activity_request_settlement (request_activity_id, judged_through_activity_id, verdict, confidence, decided_by)
		VALUES ($1, $2, 'settled', 1, 'test-model')`, c.request, copyID); err != nil {
		t.Fatal(err)
	}
	for _, reminder := range []*string{&c.machineReminder, &c.humanReminder} {
		if err := conn.QueryRow(ctx, `
			INSERT INTO activity (kind, subject, occurred_at, source, captured_by, source_system, source_activity_id, is_done, done_at)
			VALUES ('task', 'Reply', now(), 'system', 'system', 'email_request', $1, true, now()) RETURNING id`,
			c.request).Scan(reminder); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id)
		VALUES ('human', $1, 'update', 'activity', $2)`, colleague, c.humanReminder); err != nil {
		t.Fatal(err)
	}
	return c
}

func (c settledByCopy) assertUndone(ctx context.Context, t *testing.T, conn *pgx.Conn) {
	t.Helper()
	var senders, verdicts int
	var machineDone, humanDone bool
	if err := conn.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM activity_participant WHERE activity_id = $1 AND role = 'from' AND user_id = $2),
		       (SELECT count(*) FROM activity_request_settlement WHERE request_activity_id = $3),
		       (SELECT is_done FROM activity WHERE id = $4),
		       (SELECT is_done FROM activity WHERE id = $5)`,
		c.copyID, c.colleague, c.request, c.machineReminder, c.humanReminder).
		Scan(&senders, &verdicts, &machineDone, &humanDone); err != nil {
		t.Fatal(err)
	}
	if senders != 0 || verdicts != 0 {
		t.Errorf("the turned-back copy keeps %d colleague sender rows and its request %d verdicts; want 0 and 0", senders, verdicts)
	}
	if machineDone || !humanDone {
		t.Errorf("reminders done: system-completed %v, human-touched %v; want false and true", machineDone, humanDone)
	}
}
