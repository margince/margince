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

	execFile(ctx, t, conn, colleagueCopyAuthUp)

	for name, c := range map[string]struct {
		id, direction, counterparty string
	}{
		"Gmail passed DMARC for the From domain": {authenticated, "outbound", "dana@client.test"},
		"the topmost result failed":              {failed, "inbound", "sam@auth.test"},
		"no stored original":                     {noOriginal, "inbound", "sam@auth.test"},
		"captured over IMAP":                     {overIMAP, "inbound", "sam@auth.test"},
		"a pass smuggled in a quoted sender":     {smuggled, "inbound", "sam@auth.test"},
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
}
