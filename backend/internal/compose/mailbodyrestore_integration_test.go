// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A mail an older parser stored with only its From/To lines gets its text
// back from its stored original. Nothing is guessed where no original
// survives, and a body a member edited is theirs.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type storedMail struct {
	body, completeness string
	occurredAt         time.Time
	ruleset            *string
}

func (o *owedEnv) stored(t *testing.T, message ids.UUID) storedMail {
	t.Helper()
	var got storedMail
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT body, body_completeness, occurred_at, owed_verdict_ruleset
			FROM activity WHERE id = $1`, message).Scan(&got.body, &got.completeness, &got.occurredAt, &got.ruleset)
	}); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestALegacyBodyIsRestoredFromItsOriginal(t *testing.T) {
	o := setupOwed(t)
	o.now = clocktest.Now(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	ask := "Could you send the signed NDA by Friday?"
	headerOnly := "From: pat@customer.example\nTo: " + o.seat

	lost := o.capture(t, owedMail{
		from: "pat@customer.example", to: o.seat, subject: "NDA", messageID: "restore-lost@customer.example",
		body: readInFull(ask), encoding: "base64", authenticated: true, at: o.now.Add(-48 * time.Hour),
	})
	edited := o.capture(t, owedMail{
		from: "pat@customer.example", to: o.seat, subject: "NDA again", messageID: "restore-edited@customer.example",
		body: readInFull(ask), encoding: "base64", authenticated: true, at: o.now.Add(-47 * time.Hour),
	})
	o.asLegacy(t, lost, headerOnly)
	o.asLegacy(t, edited, headerOnly)
	// A member's own edit, whatever it says, is never overwritten.
	if _, err := o.e.Activities.UpdateActivity(o.e.Admin(), ids.From[ids.ActivityKind](edited),
		activities.UpdateActivityInput{Body: new(headerOnly + "\n")}); err != nil {
		t.Fatal(err)
	}
	o.asLegacy(t, edited, headerOnly)
	var noOriginal ids.UUID
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `INSERT INTO activity (kind, subject, body, direction, occurred_at,
			source_system, source_id, source, captured_by, body_completeness)
			VALUES ('email', 'hi', $1, 'inbound', $2, 'email', 'restore-none@customer.example', 'gmail:seed',
			        'connector:gmail', 'unassessed') RETURNING id`, headerOnly, o.now).Scan(&noOriginal)
	}); err != nil {
		t.Fatal(err)
	}
	// A raw capture planted under that mail's Message-ID, with no link to it:
	// a sender types the Message-ID, so it is no proof of provenance.
	var planted ids.UUID
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			WITH rc AS (INSERT INTO raw_capture (source_system, source_id, payload)
			            VALUES ('email', 'restore-planted@customer.example', to_jsonb($2::text)) RETURNING id)
			INSERT INTO activity (kind, subject, body, direction, occurred_at, source_system, source_id, source,
			                      captured_by, body_completeness)
			VALUES ('email', 'hi', $1, 'inbound', now(), 'email', 'restore-planted@customer.example', 'gmail:seed',
			        'connector:gmail', 'unassessed') RETURNING id`,
			headerOnly, string(plantedOriginal(o.seat))).Scan(&planted)
	}); err != nil {
		t.Fatal(err)
	}
	_, readAt, err := activities.NewStore(o.e.DB()).OwedBacklog(o.judge(), "old-rules", o.now, 1, 400, 400)
	if err != nil {
		t.Fatal(err)
	}
	if applied, err := o.e.Activities.SetOwedVerdict(o.judge(), lost, activities.OwedVerdictInformsUs, "old-rules", readAt); err != nil || !applied {
		t.Fatalf("the verdict read from the short body: applied=%v err=%v", applied, err)
	}
	var thread string
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(context.Background(), `SELECT thread_key FROM activity WHERE id = $1`, lost).Scan(&thread); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `INSERT INTO signal_thread_scan (thread_key, last_activity_at, message_count)
			VALUES ($1, now(), 1)`, thread)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	before := o.stored(t, lost)

	// The preview: the registered task's own estimate, before anything runs.
	task := registeredMailBodyTask(t, o)
	ctx := repairOperator(t, o.e)
	var estimate int64
	if err := database.WithWorkspaceTx(ctx, o.e.Pool, func(tx pgx.Tx) error {
		var err error
		estimate, _, err = task.Steps[0].Estimate(ctx, tx, 100)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if estimate != 2 {
		t.Errorf("the preview counts %d mails to look at, want the 2 short ones with an original", estimate)
	}
	if got := o.stored(t, lost); got.body != before.body {
		t.Fatal("the preview changed a body")
	}

	o.runMailBodyTask(t)
	got := o.stored(t, lost)
	if got.body != headerOnly+"\n\n"+ask || got.completeness != "complete" {
		t.Errorf("restored body = %q (%q), want the original's whole text", got.body, got.completeness)
	}
	if !got.occurredAt.Equal(before.occurredAt) {
		t.Errorf("occurred_at moved from %v to %v; the mail keeps its age", before.occurredAt, got.occurredAt)
	}
	if got.ruleset != nil {
		t.Errorf("the verdict read from the short body is not marked for re-judgement (ruleset %q)", *got.ruleset)
	}
	if n := o.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND after->>'body_restored' = 'true'`, lost); n != 1 {
		t.Errorf("restorations audited = %d, want 1", n)
	}
	if n := o.e.WsCount(t, `SELECT count(*) FROM event_outbox o
		JOIN audit_log a ON a.id = (o.envelope->'trace'->>'audit_log_id')::uuid
		WHERE a.entity_id = $1 AND a.after->>'body_restored' = 'true'`, lost); n != 1 {
		t.Errorf("restoration events = %d, want 1", n)
	}
	if n := o.e.WsCount(t, `SELECT count(*) FROM signal_thread_scan WHERE thread_key = $1`, thread); n != 0 {
		t.Error("the conversation read from the short body is not due again")
	}
	if got := o.stored(t, edited); got.body != headerOnly {
		t.Errorf("a body a member edited was rewritten to %q", got.body)
	}
	if got := o.stored(t, noOriginal); got.body != headerOnly || got.completeness != "unverifiable" {
		t.Errorf("a mail with no original became %q (%q); nothing may be guessed", got.body, got.completeness)
	}
	if got := o.stored(t, planted); got.body != headerOnly || got.completeness != "unverifiable" {
		t.Errorf("a mail was rewritten from an original it does not link: %q (%q)", got.body, got.completeness)
	}
	assertMailBodyReplayChangesNothing(t, o, task)
}

// plantedOriginal is a readable mail a sender could store under any Message-ID.
func plantedOriginal(seat string) []byte {
	return owedMail{
		from: "pat@customer.example", to: seat, subject: "hi", messageID: "restore-planted@customer.example",
		body: "Planted words.", at: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}.raw()
}

// The restoration writer itself refuses a row that changed since it was read,
// and a held row. A refusal writes no body, no audit row and no event.
func TestTheRestoreWriterRefusesAStaleOrHeldRow(t *testing.T) {
	ask := "Could you send the signed NDA by Friday?"
	for _, tc := range []struct {
		name     string
		edit     func(r *activities.MailBodyRestoration)
		hold     bool
		restored bool
	}{
		{"as read", func(*activities.MailBodyRestoration) {}, false, true},
		{"a stale version", func(r *activities.MailBodyRestoration) { r.Version-- }, false, false},
		{"a body changed since it was read", func(r *activities.MailBodyRestoration) { r.StoredBody += " " }, false, false},
		{"a held row", func(*activities.MailBodyRestoration) {}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := setupOwed(t)
			o.now = clocktest.Now(t)
			o.contact(t, "Pat Buyer", "pat@customer.example")
			headerOnly := "From: pat@customer.example\nTo: " + o.seat
			message := o.capture(t, owedMail{
				from: "pat@customer.example", to: o.seat, subject: "NDA", messageID: "writer@customer.example",
				body: readInFull(ask), encoding: "base64", authenticated: true, at: o.now.Add(-time.Hour),
			})
			o.asLegacy(t, message, headerOnly)
			if tc.hold {
				placeStatutoryHold(t, message)
			}
			restoration := activities.MailBodyRestoration{
				Activity: message, StoredBody: headerOnly, Body: headerOnly + "\n\n" + ask, Completeness: "complete",
			}
			if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
				return tx.QueryRow(context.Background(), `SELECT version FROM activity WHERE id = $1`, message).Scan(&restoration.Version)
			}); err != nil {
				t.Fatal(err)
			}
			tc.edit(&restoration)
			system := principal.SystemActing(o.e.Admin(), "system:datatask-test")
			var restored bool
			if err := database.WithWorkspaceTx(system, o.e.Pool, func(tx pgx.Tx) error {
				var err error
				restored, _, err = activities.RestoreMailBodyTx(system, tx, restoration)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if restored != tc.restored {
				t.Fatalf("restored = %v, want %v", restored, tc.restored)
			}
			want, rows := headerOnly, 0
			if tc.restored {
				want, rows = restoration.Body, 1
			}
			if got := o.stored(t, message).body; got != want {
				t.Errorf("body = %q, want %q", got, want)
			}
			if n := o.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND after->>'body_restored' = 'true'`, message); n != rows {
				t.Errorf("restoration audit rows = %d, want %d", n, rows)
			}
			if n := o.e.WsCount(t, `SELECT count(*) FROM event_outbox o
				JOIN audit_log a ON a.id = (o.envelope->'trace'->>'audit_log_id')::uuid
				WHERE a.entity_id = $1 AND a.after->>'body_restored' = 'true'`, message); n != rows {
				t.Errorf("restoration events = %d, want %d", n, rows)
			}
		})
	}
}

// placeStatutoryHold restricts a row in the whole shape the database asks for.
func placeStatutoryHold(t *testing.T, message ids.UUID) {
	t.Helper()
	owner := integration.OwnerConn(t)
	ctx := context.Background()
	if _, err := owner.Exec(ctx, `
		INSERT INTO activity_retention_evidence (activity_id, basis, qualified_at, decided_by_name, reason)
		VALUES ($1, 'controller_pin', now(), 'Datenschutz', 'litigation hold')`, message); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `
		UPDATE activity SET restricted_at = now(), archived_at = now(), restricted_reason = 'litigation hold',
		       restricted_until = now() + interval '1 year',
		       retention_class = 'commercial_correspondence', retention_class_at = now()
		 WHERE id = $1`, message); err != nil {
		t.Fatal(err)
	}
}
