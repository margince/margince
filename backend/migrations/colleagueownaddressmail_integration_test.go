// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

const (
	colleagueMailUnderOwnAddress     = "core/1791782100_a_colleagues_mail_is_ours_under_an_own_address.up.sql"
	colleagueMailUnderOwnAddressDown = "core/1791782100_a_colleagues_mail_is_ours_under_an_own_address.down.sql"
)

// A colleague's copy whose address the capturing seat also claims was stored
// with no `from` participant, so 1791775182 left it inbound. This migration
// re-reads it from counterparty_email, as capture does, and down restores it.
func TestAColleaguesMailUnderAnOwnAddressIsOurs(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()
	var seat, colleague, copied, reminder string
	if err := conn.QueryRow(ctx, `
		INSERT INTO app_user (email, display_name) VALUES ('rep@colleague.test', 'Rep') RETURNING id`).Scan(&seat); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(ctx, `
		INSERT INTO app_user (email, display_name, status) VALUES ('sam@colleague.test', 'Sam', 'invited') RETURNING id`).Scan(&colleague); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO capture_owner_identity (user_id, kind, value, created_by)
		VALUES ($1, 'address', 'sam@colleague.test', 'test')`, seat); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(ctx, `
		INSERT INTO activity (kind, subject, direction, occurred_at, source_system, source_id, source, captured_by,
		                      counterparty_email, owed_verdict, owed_verdict_at)
		VALUES ('email', 'Re: offer', 'inbound', now(), 'gmail', 'alias-copy-1', 'gmail:seed', 'connector:gmail:' || $1,
		        'sam@colleague.test', 'asks_us', now()) RETURNING id`, seat).Scan(&copied); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO activity_participant (activity_id, role, user_id) VALUES ($1, 'to', $2)`, copied, seat); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO activity_participant (activity_id, role, address) VALUES ($1, 'to', 'dana@client.test')`, copied); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(ctx, `
		INSERT INTO activity (kind, subject, occurred_at, source, captured_by, source_system, source_activity_id)
		VALUES ('task', 'Reply', now(), 'system', 'system', 'email_request', $1) RETURNING id`, copied).Scan(&reminder); err != nil {
		t.Fatal(err)
	}

	// A customer's question, captured before the customer was invited as a seat.
	var question, invitedLater string
	if err := conn.QueryRow(ctx, `
		INSERT INTO activity (kind, subject, direction, occurred_at, source_system, source_id, source, captured_by,
		                      counterparty_email)
		VALUES ('email', 'Question', 'inbound', now(), 'gmail', 'before-invite-1', 'gmail:seed', 'connector:gmail:' || $1,
		        'alice@client.test') RETURNING id`, seat).Scan(&question); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO activity_participant (activity_id, role, address) VALUES ($1, 'to', 'dana@client.test')`, question); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(ctx, `
		INSERT INTO app_user (email, display_name, status, created_at) VALUES ('alice@client.test', 'Alice', 'invited', now() + interval '1 minute')
		RETURNING id`).Scan(&invitedLater); err != nil {
		t.Fatal(err)
	}

	// Left alone: a copy naming no outside recipient, and the colleague's mail
	// captured by their own mailbox.
	untouched := map[string]string{}
	for sourceID, capturer := range map[string]string{"internal-only-1": seat, "own-mailbox-1": colleague} {
		var id string
		if err := conn.QueryRow(ctx, `
			INSERT INTO activity (kind, subject, direction, occurred_at, source_system, source_id, source, captured_by,
			                      counterparty_email)
			VALUES ('email', 'Lunch', 'inbound', now(), 'gmail', $1, 'gmail:seed', 'connector:gmail:' || $2,
			        'sam@colleague.test') RETURNING id`, sourceID, capturer).Scan(&id); err != nil {
			t.Fatal(err)
		}
		untouched[sourceID] = id
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO activity_participant (activity_id, role, address) VALUES ($1, 'to', 'dana@client.test')`,
		untouched["own-mailbox-1"]); err != nil {
		t.Fatal(err)
	}

	execFile(ctx, t, conn, colleagueMailUnderOwnAddress)
	execFile(ctx, t, conn, colleagueMailUnderOwnAddress)

	for sourceID, id := range untouched {
		if left := colleagueCopyState(ctx, t, conn, id, colleague); left.direction != "inbound" || left.byColleague {
			t.Errorf("%s is %+v, want the inbound mail it was", sourceID, left)
		}
	}

	if later := colleagueCopyState(ctx, t, conn, question, invitedLater); later.direction != "inbound" || later.byColleague {
		t.Errorf("a question from a seat invited after it was captured is %+v, want the inbound mail it was", later)
	}

	got := colleagueCopyState(ctx, t, conn, copied, colleague)
	if got != (copyState{direction: "outbound", counterparty: "dana@client.test", byColleague: true, fromColleague: true, rereads: 1}) {
		t.Errorf("after two runs the copy is %+v, want outbound to dana by the colleague, re-read once", got)
	}
	if !archivedNow(ctx, t, conn, reminder) {
		t.Error("the reminder the owed pass filed while the copy read as a question is still open")
	}

	execFile(ctx, t, conn, colleagueMailUnderOwnAddressDown)
	got = colleagueCopyState(ctx, t, conn, copied, colleague)
	if got.direction != "inbound" || got.counterparty != "sam@colleague.test" || got.byColleague || got.fromColleague {
		t.Errorf("after down the copy is %+v, want inbound from the colleague as capture filed it", got)
	}
	if archivedNow(ctx, t, conn, reminder) {
		t.Error("the down migration left the reminder it archived archived")
	}
}

type copyState struct {
	direction, counterparty    string
	byColleague, fromColleague bool
	rereads                    int
}

func colleagueCopyState(ctx context.Context, t *testing.T, conn *pgx.Conn, id, colleague string) copyState {
	t.Helper()
	var got copyState
	if err := conn.QueryRow(ctx, `
		SELECT a.direction, a.counterparty_email, a.sent_by_colleague IS NOT NULL,
		       EXISTS (SELECT 1 FROM activity_participant p WHERE p.activity_id = a.id AND p.role = 'from' AND p.user_id = $2),
		       (SELECT count(*) FROM audit_log m WHERE m.entity_id = a.id AND m.action = 'update'
		         AND m.after->>'migration' = '1791782100_a_colleagues_mail_is_ours_under_an_own_address')
		  FROM activity a WHERE a.id = $1`, id, colleague).
		Scan(&got.direction, &got.counterparty, &got.byColleague, &got.fromColleague, &got.rereads); err != nil {
		t.Fatal(err)
	}
	return got
}
