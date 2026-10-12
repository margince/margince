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
	colleagueCopiedMailOwesNothing     = "core/1791778974_a_colleagues_copied_mail_owes_nothing.up.sql"
	colleagueCopiedMailOwesNothingDown = "core/1791778974_a_colleagues_copied_mail_owes_nothing.down.sql"
)

// rereadColleagueCopy seeds a colleague's copied mail as migration 1791775182
// left it: outbound to the customer, by the colleague.
func rereadColleagueCopy(ctx context.Context, t *testing.T, conn *pgx.Conn, seat, colleague string) string {
	t.Helper()
	var id string
	if err := conn.QueryRow(ctx, `
		INSERT INTO activity (kind, subject, direction, occurred_at, source_system, source_id, source, captured_by,
		                      counterparty_email, sent_by_colleague)
		VALUES ('email', 'Re: offer', 'outbound', now(), 'gmail', 'copy-1', 'gmail:seed', 'connector:gmail:' || $1,
		        'dana@client.test', $2) RETURNING id`, seat, colleague).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestAColleaguesCopiedMailOwesNothing(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()
	var seat, colleague string
	if err := conn.QueryRow(ctx, `
		INSERT INTO app_user (email, display_name) VALUES ('rep@colleague.test', 'Rep') RETURNING id`).Scan(&seat); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(ctx, `
		INSERT INTO app_user (email, display_name, status) VALUES ('sam@colleague.test', 'Sam', 'invited') RETURNING id`).Scan(&colleague); err != nil {
		t.Fatal(err)
	}
	copied := rereadColleagueCopy(ctx, t, conn, seat, colleague)
	// What the owed pass left on the copy while it read as a question: a
	// verdict, a reminder nobody touched, and one a human edited.
	var untouched, edited string
	if err := conn.QueryRow(ctx, `
		INSERT INTO activity_request_settlement (request_activity_id, judged_through_activity_id, verdict, confidence, decided_by)
		VALUES ($1, $1, 'still_owed', 1, 'test')
		RETURNING request_activity_id`, copied).Scan(new(string)); err != nil {
		t.Fatal(err)
	}
	for _, reminder := range []*string{&untouched, &edited} {
		if err := conn.QueryRow(ctx, `
			INSERT INTO activity (kind, subject, occurred_at, source, captured_by, source_system, source_activity_id)
			VALUES ('task', 'Reply', now(), 'system', 'system', 'email_request', $1) RETURNING id`, copied).Scan(reminder); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id)
		VALUES ('human', $1, 'update', 'activity', $2)`, seat, edited); err != nil {
		t.Fatal(err)
	}

	execFile(ctx, t, conn, colleagueCopiedMailOwesNothing)
	execFile(ctx, t, conn, colleagueCopiedMailOwesNothing)

	var verdicts, audits int
	var untouchedArchived, editedArchived bool
	if err := conn.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM activity_request_settlement WHERE request_activity_id = a.id),
		       (SELECT count(*) FROM audit_log WHERE entity_id = $2
		         AND after->>'migration' = '1791778974_a_colleagues_copied_mail_owes_nothing'),
		       (SELECT archived_at IS NOT NULL FROM activity WHERE id = $2),
		       (SELECT archived_at IS NOT NULL FROM activity WHERE id = $3)
		  FROM activity a WHERE a.id = $1`, copied, untouched, edited).
		Scan(&verdicts, &audits, &untouchedArchived, &editedArchived); err != nil {
		t.Fatal(err)
	}
	if verdicts != 0 || audits != 1 {
		t.Errorf("%d settlement verdicts remain and the archived reminder has %d audit rows; want 0 and 1 across two runs",
			verdicts, audits)
	}
	if !untouchedArchived || editedArchived {
		t.Errorf("reminder archived: untouched %v, human-edited %v; want true and false", untouchedArchived, editedArchived)
	}

	// Down restores the archive it made, and not one made after it.
	execFile(ctx, t, conn, colleagueCopiedMailOwesNothingDown)
	if archivedNow(ctx, t, conn, untouched) {
		t.Error("the down migration left the reminder it archived archived")
	}
	if _, err := conn.Exec(ctx, `UPDATE activity SET archived_at = clock_timestamp() WHERE id = $1`, untouched); err != nil {
		t.Fatal(err)
	}
	execFile(ctx, t, conn, colleagueCopiedMailOwesNothingDown)
	if !archivedNow(ctx, t, conn, untouched) {
		t.Error("the down migration restored a reminder archived after it ran")
	}
}

func archivedNow(ctx context.Context, t *testing.T, conn *pgx.Conn, id string) bool {
	t.Helper()
	var archived bool
	if err := conn.QueryRow(ctx, `SELECT archived_at IS NOT NULL FROM activity WHERE id = $1`, id).Scan(&archived); err != nil {
		t.Fatal(err)
	}
	return archived
}
