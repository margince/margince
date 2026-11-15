// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// owedSQL reads whether a human took the request once per message. Its rules
// must admit the same messages as with the EXISTS written into every rule.
// The matrix below varies everything those rules read.

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// seedOwedMatrix writes one inbound message per combination of verdict, taken
// request, machine sender, bulk mail, calendar reply, connector capture, label
// and reply.
func seedOwedMatrix(ctx context.Context, t *testing.T, tx pgx.Tx, base time.Time) {
	t.Helper()
	if _, err := tx.Exec(ctx, `
	CREATE TEMP TABLE owed_case AS
	  SELECT gen_random_uuid() AS id, row_number() OVER () AS n, *
	    FROM (VALUES (NULL::text), ('asks_us'), ('informs_us')) v(verdict)
	    CROSS JOIN (VALUES ('none'), ('human'), ('system')) k(taken)
	    CROSS JOIN (VALUES (false), (true)) m(machine)
	    CROSS JOIN (VALUES (false), (true)) b(bulk)
	    CROSS JOIN (VALUES (NULL::text), ('REPLY')) c(calendar)
	    CROSS JOIN (VALUES ('human:seed'), ('connector:seed')) p(captured_by)
	    CROSS JOIN (VALUES (NULL::text), ('noise'), ('commitment')) l(label)
	    CROSS JOIN (VALUES (false), (true)) r(replied)`); err != nil {
		t.Fatalf("building the owed matrix: %v", err)
	}
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by, thread_key,
		                      body_completeness, owed_verdict, owed_verdict_at, bulk_mail_attested,
		                      calendar_method, capture_label, capture_labeled_at)
		  SELECT id, 'email', 'Owed '||n, $1, 'inbound', 'manual', captured_by, 'owed-'||n,
		         'complete', verdict, CASE WHEN verdict IS NOT NULL THEN $1::timestamptz END, bulk,
		         calendar, label, CASE WHEN label IS NOT NULL THEN $1::timestamptz END
		    FROM owed_case`, []any{base}},
		{`INSERT INTO activity_participant (activity_id, address, role)
		  SELECT id, CASE WHEN machine THEN 'noreply' ELSE 'buyer' END||n||'@owed.example', 'from' FROM owed_case`, nil},
		{`INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by, source_system, source_activity_id)
		  SELECT gen_random_uuid(), 'task', 'Reply to Owed '||n, $1, 'manual', 'human:seed', $2, id
		    FROM owed_case WHERE taken <> 'none'`, []any{base, EmailRequestTaskSource}},
		{`INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id)
		  SELECT 'human', 'seed', 'create', 'activity', task.id
		    FROM activity task JOIN owed_case c ON c.id = task.source_activity_id WHERE c.taken = 'human'`, nil},
		{`INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by, thread_key)
		  SELECT gen_random_uuid(), 'email', 'Re: Owed '||n, $1::timestamptz + interval '1 hour', 'outbound', 'manual', 'human:seed', 'owed-'||n
		    FROM owed_case WHERE replied`, []any{base}},
		{`ANALYZE activity`, nil},
	} {
		if _, err := tx.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding the owed matrix: %v", err)
		}
	}
}

func owedIDs(ctx context.Context, t *testing.T, tx pgx.Tx, where string, asOf time.Time) []string {
	t.Helper()
	rows, err := tx.Query(ctx, `SELECT a.subject FROM activity a JOIN owed_case c ON c.id = a.id
	  WHERE `+where+` ORDER BY a.subject`, asOf)
	if err != nil {
		t.Fatalf("reading what is owed: %v", err)
	}
	subjects, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("collecting what is owed: %v", err)
	}
	return subjects
}

func TestOwedReadsATakenRequestOnceAndAdmitsWhatEveryRuleReadingItAdmits(t *testing.T) {
	e := setupPromises(t)
	ctx := context.Background()
	tx, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("rolling back the owed matrix: %v", err)
		}
	}()
	base := clocktest.Now(t).Add(-2 * time.Hour).Truncate(time.Second)
	seedOwedMatrix(ctx, t, tx, base)
	const asOf = "$1::timestamptz"

	if n := strings.Count(owedSQL(asOf, neverRelaxed, neverRelaxed, neverRelaxed), "FROM activity accepted"); n != 1 {
		t.Errorf("owedSQL reads a taken request %d times, want once", n)
	}
	relaxed := []string{neverRelaxed, "TRUE"}
	for _, dismissed := range relaxed {
		for _, informs := range relaxed {
			for _, unjudged := range relaxed {
				inline := `(NOT ` + ImportedHistorySQL + ` AND ` + requestOpenSQL + `
				  AND (` + dismissed + ` OR ` + notDismissedSQL + `)
				  AND ` + owedRulesSQL(asOf, informs, unjudged, acceptedRequestSQL) + `)`
				got := owedIDs(ctx, t, tx, owedSQL(asOf, dismissed, informs, unjudged), base)
				want := owedIDs(ctx, t, tx, inline, base)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("relaxed %s/%s/%s: read once owes %d messages, read in every rule %d:\n%v\n%v",
						dismissed, informs, unjudged, len(got), len(want), got, want)
				}
			}
		}
	}

	// The matrix must reach the rules a taken request changes, or the
	// comparison above holds over rows where it cannot differ.
	taken := owedIDs(ctx, t, tx, owedRulesSQL(asOf, neverRelaxed, neverRelaxed, "TRUE")+` AND `+requestOpenSQL, base)
	untaken := owedIDs(ctx, t, tx, owedRulesSQL(asOf, neverRelaxed, neverRelaxed, "FALSE")+` AND `+requestOpenSQL, base)
	if len(taken) <= len(untaken) || len(untaken) == 0 {
		t.Errorf("a taken request owes %d messages and an untaken one %d; the matrix does not exercise the rules it changes",
			len(taken), len(untaken))
	}
	if all := owedIDs(ctx, t, tx, owedSQL(asOf, neverRelaxed, neverRelaxed, neverRelaxed)+` AND c.taken = 'human'`, base); len(all) == 0 {
		t.Error("no message a human took is owed; the matrix never makes the taken-request subquery true")
	}
}
