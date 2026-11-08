// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// agedEvent is the newest published event the contact's creation wrote, aged
// to days ago. The relay stamps published_at; the test moves it back in time.
func agedEvent(t *testing.T, owner *pgx.Conn, contact ids.UUID, days int, published bool) ids.UUID {
	t.Helper()
	var event ids.UUID
	if err := owner.QueryRow(context.Background(), `UPDATE event_outbox
		SET published_at = CASE WHEN $3 THEN now() - make_interval(days => $2) END,
		    created_at = now() - make_interval(days => $2)
		WHERE id = (SELECT id FROM event_outbox WHERE envelope->'entity'->>'id' = $1::text ORDER BY id DESC LIMIT 1)
		RETURNING id`, contact, days, published).Scan(&event); err != nil {
		t.Fatal(err)
	}
	return event
}

func eventKept(t *testing.T, e *integration.Env, event ids.UUID) bool {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE id = $1`, event) == 1
}

func TestPublishedEventsLeaveTheOutboxAfterTheirWindowUnlessSomethingStillNeedsThem(t *testing.T) {
	e := integration.Setup(t)
	owner := integration.OwnerConn(t)
	ordinary := agedEvent(t, owner, keyedRecipient(t, e, "Old News", "old@window.test"), 31, true)
	recent := agedEvent(t, owner, keyedRecipient(t, e, "Fresh News", "fresh@window.test"), 5, true)
	unpublished := agedEvent(t, owner, keyedRecipient(t, e, "Never Sent", "never@window.test"), 90, false)
	heldContact := keyedRecipient(t, e, "Held Party", "held@window.test")
	placeLegalHold(t, heldContact)
	held := agedEvent(t, owner, heldContact, 31, true)
	unread := agedEvent(t, owner, keyedRecipient(t, e, "Unread Input", "unread@window.test"), 31, true)
	if _, err := owner.Exec(context.Background(), `UPDATE event_outbox
		SET envelope = jsonb_set(envelope, '{type}', to_jsonb($2::text)), assurance_inputs_read = false WHERE id = $1`,
		unread, assurance.MarkingEventTypes()[0]); err != nil {
		t.Fatal(err)
	}

	if err := NewRetentionServiceFor(e.DB(), nil, slog.Default()).EvaluateInstallation(integration.RetentionPassCtx(e.WS)); err != nil {
		t.Fatal(err)
	}
	if eventKept(t, e, ordinary) {
		t.Error("a published event 31 days old stayed in the outbox")
	}
	for name, event := range map[string]ids.UUID{
		"an event inside its window": recent, "an unpublished event": unpublished,
		"an event about a held contact": held, "input the assurance check has not read": unread,
	} {
		if !eventKept(t, e, event) {
			t.Errorf("the pass removed %s", name)
		}
	}
}

// seedDelivery plants one delivery about contact whose state changed days ago.
func seedDelivery(t *testing.T, owner *pgx.Conn, sub, contact ids.UUID, status string, days int) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := owner.Exec(context.Background(), `INSERT INTO webhook_delivery
		(id, subscription_id, event_id, event_type, payload, status, attempts, entity_type, entity_id,
		 delivered_at, dead_lettered_at, created_at, updated_at)
		VALUES ($1, $2, $3, 'contact.created', '{"name":"Personal Name"}', $4, 1, 'contact', $5,
		 CASE WHEN $4 = 'delivered' THEN now() - make_interval(days => $6) END,
		 CASE WHEN $4 = 'dead_lettered' THEN now() - make_interval(days => $6) END,
		 now() - make_interval(days => $6), now() - make_interval(days => $6))`,
		id, sub, ids.NewV7(), status, contact, days); err != nil {
		t.Fatal(err)
	}
	return id
}

func seedSubscription(t *testing.T, e *integration.Env, owner *pgx.Conn) ids.UUID {
	t.Helper()
	sub := ids.NewV7()
	if _, err := owner.Exec(context.Background(), `INSERT INTO webhook_subscription
		(id, owner_id, target_url, event_types, signing_secret_ref, state)
		VALUES ($1, $2, 'https://hook.test/x', ARRAY['contact.created'], 'ref', 'active')`, sub, e.AdminUser); err != nil {
		t.Fatal(err)
	}
	return sub
}

func bodyKept(t *testing.T, e *integration.Env, delivery ids.UUID) bool {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM webhook_delivery WHERE id = $1 AND payload <> '' AND payload_pruned_at IS NULL`, delivery) == 1
}

func TestEndedWebhookBodiesAreClearedAfterTheirWindowAndWorkStillToSendKeepsItsBody(t *testing.T) {
	e := integration.Setup(t)
	owner := integration.OwnerConn(t)
	sub := seedSubscription(t, e, owner)
	subject := keyedRecipient(t, e, "Webhook Subject", "subject@window.test")
	heldContact := keyedRecipient(t, e, "Held Subject", "heldsubject@window.test")
	placeLegalHold(t, heldContact)
	cleared := map[string]ids.UUID{
		"a delivered body":     seedDelivery(t, owner, sub, subject, "delivered", 8),
		"a dead-lettered body": seedDelivery(t, owner, sub, subject, "dead_lettered", 8),
		"a cancelled body":     seedDelivery(t, owner, sub, subject, "visibility_revoked", 8),
	}
	kept := map[string]ids.UUID{
		"a body delivered two days ago": seedDelivery(t, owner, sub, subject, "delivered", 2),
		"a pending body":                seedDelivery(t, owner, sub, subject, "pending", 60),
		"a retrying body":               seedDelivery(t, owner, sub, subject, "retrying", 60),
		"a body about a held contact":   seedDelivery(t, owner, sub, heldContact, "delivered", 8),
	}

	if err := NewRetentionServiceFor(e.DB(), nil, slog.Default()).EvaluateInstallation(integration.RetentionPassCtx(e.WS)); err != nil {
		t.Fatal(err)
	}
	for name, delivery := range cleared {
		if bodyKept(t, e, delivery) {
			t.Errorf("the pass kept %s past its window", name)
		}
	}
	for name, delivery := range kept {
		if !bodyKept(t, e, delivery) {
			t.Errorf("the pass cleared %s", name)
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM webhook_delivery WHERE subscription_id = $1`, sub); n != len(cleared)+len(kept) {
		t.Fatalf("%d delivery rows remain, want every row kept for the log", n)
	}
}

func TestErasingASubjectClearsItsWebhookBodiesAndCancelsWhatIsStillToSend(t *testing.T) {
	e := integration.Setup(t)
	owner := integration.OwnerConn(t)
	sub := seedSubscription(t, e, owner)
	subject := keyedRecipient(t, e, "Erased Webhook Subject", "erased@webhook.test")
	delivered := seedDelivery(t, owner, sub, subject, "delivered", 1)
	retrying := seedDelivery(t, owner, sub, subject, "retrying", 1)

	if err := ComposedEraser(e.Pool, nil, nil).EraseContact(e.Admin(), subject, "test"); err != nil {
		t.Fatal(err)
	}
	if bodyKept(t, e, delivered) || bodyKept(t, e, retrying) {
		t.Fatal("the erasure left a webhook body rendered from the subject")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM webhook_delivery WHERE id = $1 AND status = 'visibility_revoked'
		AND next_retry_at IS NULL`, retrying); n != 1 {
		t.Fatal("a retry about the erased subject is still scheduled")
	}
}
