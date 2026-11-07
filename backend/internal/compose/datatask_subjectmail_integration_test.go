// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Legacy rows are explicit fixtures because current admission no longer creates these false cases.
func seedDirectSenderDuty(t *testing.T, e *integration.Env, kind, state, actor string) ids.UUID {
	t.Helper()
	contact := e.SeedContact(t, "Inbound Sender", &e.Rep1)
	e.WsExec(t, `WITH address AS (
  INSERT INTO contact_email (contact_id, email, source, captured_by)
  VALUES (@contact, @email, 'capture', @actor)),
 acquisition AS (
  INSERT INTO contact_acquisition_evidence (contact_id, kind, captured_by)
  VALUES (@contact, @kind, @actor) RETURNING id),
 mail AS (
  INSERT INTO activity (kind, subject, direction, occurred_at, source, captured_by, bulk_mail_attested, inbound_sender_authenticated)
  VALUES ('email', 'Event sponsorship', 'inbound', @now, 'gmail', 'connector:gmail', true, true) RETURNING id),
 sender AS (
  INSERT INTO activity_participant (activity_id, role, address)
  SELECT id, 'from', @email FROM mail),
 delivery AS (
 INSERT INTO comms_outbound (id, activity_id, provider, message_id, recipients, subject, body, status, sender_kind, template_key, template_version)
 SELECT @delivery, id, 'controller', id::text, jsonb_build_array(@email::text), 'Privacy notice', 'body', 'pending', 'controller', 'record_confirmation', 1
 FROM mail WHERE @state = 'queued' RETURNING id)
 INSERT INTO privacy_notice_case (contact_id, acquisition_id, rule, due_at, state, blocked_reason, delivery_id)
 SELECT @contact, id, 'art14', @now, @state, CASE WHEN @state = 'blocked' THEN 'Missing route' END, (SELECT id FROM delivery) FROM acquisition`, pgx.NamedArgs{
		"delivery": ids.NewV7(), "contact": contact, "email": contact.String() + "@example.test", "actor": actor, "kind": kind, "state": state, "now": clocktest.Now(t),
	})
	return contact
}

func TestDirectSenderCorrectionPagesReplaysAndPreservesOtherSources(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	now := clocktest.Now(t)
	contacts := []ids.UUID{
		seedDirectSenderDuty(t, e, "unknown_legacy", "open", "connector:gmail"),
		seedDirectSenderDuty(t, e, "unknown_legacy", "blocked", "agent:capture_counterparty_verdict"),
		seedDirectSenderDuty(t, e, "referral", "open", "connector:gmail"),
		seedDirectSenderDuty(t, e, "unknown_legacy", "queued", "connector:gmail"),
		seedDirectSenderDuty(t, e, "unknown_legacy", "open", "human:seed"),
	}
	task := subjectMailDataTask(func() time.Time { return now })
	task.Steps[1].Limit, task.Steps[1].MinLimit = 1, 1
	var estimated int64
	var more bool
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		var err error
		estimated, more, err = task.Steps[1].Estimate(ctx, tx, 1)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if estimated != 1 || !more {
		t.Fatalf("bounded estimate = %d/%t", estimated, more)
	}
	registered := false
	for _, candidate := range dataTaskCatalog(e.Pool, func() time.Time { return now }) {
		if candidate.ID == task.ID {
			registered = true
		}
	}
	if !registered {
		t.Fatal("the correction is absent from the worker catalog")
	}
	runMachineAddressTask(ctx, t, e, task)
	runMachineAddressTask(ctx, t, e, task)
	for i, contact := range contacts {
		want := "open"
		if i < 2 {
			want = "exempt_with_reason"
		}
		if i == 3 {
			want = "queued"
		}
		var state string
		if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT state FROM privacy_notice_case WHERE contact_id = @id`, pgx.NamedArgs{"id": contact}).Scan(&state)
		}); err != nil {
			t.Fatal(err)
		}
		if state != want {
			t.Fatalf("case %d = %s, want %s", i, state, want)
		}
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'privacy_notice_case'`); got != 2 {
		t.Fatalf("settlement audit count = %d, want 2", got)
	}
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		count, _, err := task.Steps[1].Estimate(ctx, tx, 1)
		if err != nil {
			return err
		}
		if count != 0 {
			t.Fatalf("finished correction still estimates %d", count)
		}
		var delivery ids.UUID
		if err := tx.QueryRow(ctx, `SELECT delivery_id FROM privacy_notice_case WHERE contact_id = @contact`, pgx.NamedArgs{"contact": contacts[3]}).Scan(&delivery); err != nil {
			return err
		}
		_, err = consent.MarkNoticeDeliveryFailedTx(ctx, tx, delivery)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM privacy_notice_case WHERE state = 'delivery_failed'`); got != 0 {
		t.Fatal("a false duty returned after the queued delivery failed")
	}
}

func TestDirectSenderCorrectionRollsBackWithItsProgress(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	seedDirectSenderDuty(t, e, "unknown_legacy", "open", "connector:gmail")
	task := subjectMailDataTask(func() time.Time { return clocktest.Now(t) })
	batch := task.Steps[1].Batch
	interrupted := errors.New("interrupt after domain changes")
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		if _, err := batch(ctx, tx, datatask.BatchIn{Limit: 1}); err != nil {
			return err
		}
		return interrupted
	})
	if !errors.Is(err, interrupted) {
		t.Fatalf("rollback returned %v", err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM privacy_notice_case WHERE state = 'open'`); got != 1 {
		t.Fatal("case escaped rollback")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM contact_acquisition_evidence WHERE kind = 'subject_initiated'`); got != 0 {
		t.Fatal("acquisition escaped rollback")
	}
	err = database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := batch(ctx, tx, datatask.BatchIn{Limit: 1, Cursor: json.RawMessage(`"bad cursor"`)})
		return err
	})
	if err == nil {
		t.Fatal("malformed cursor was accepted")
	}
}

func TestDirectSenderCorrectionKeepsAQueuedSiblingAndCountsChangedContacts(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	now := clocktest.Now(t)
	contact := seedDirectSenderDuty(t, e, "unknown_legacy", "queued", "connector:gmail")
	for range 2 {
		e.WsExec(t, `WITH acquisition AS (INSERT INTO contact_acquisition_evidence (contact_id, kind, captured_by)
   VALUES (@contact, 'unknown_legacy', 'connector:gmail') RETURNING id)
   INSERT INTO privacy_notice_case (contact_id, acquisition_id, rule, due_at, state)
   SELECT @contact, id, 'art14', @now, 'open' FROM acquisition`, pgx.NamedArgs{"contact": contact, "now": now})
	}
	task := subjectMailDataTask(func() time.Time { return now })
	for i := range task.Steps {
		task.Steps[i].Limit, task.Steps[i].MinLimit = 1, 1
	}
	runMachineAddressTask(ctx, t, e, task)
	if got := e.WsCount(t, `SELECT count(*) FROM privacy_notice_case WHERE state = 'queued'`); got != 1 {
		t.Fatalf("queued sibling count = %d", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM privacy_notice_case WHERE state = 'exempt_with_reason'`); got != 2 {
		t.Fatalf("settled sibling count = %d", got)
	}
}
