// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// telegramIngestWorker over real migrated Postgres: its contract is
// transactional (tenant context from job args, no swallowed Sink failure),
// which a mock connection cannot prove.

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// telegramIngestFixture seeds one connected channel_connection (bot "42") and
// one raw_capture row holding a text message from chat 1001, message id 7,
// sender 555 (the numbers normalize_test.go uses). Returns the job's ids.
func telegramIngestFixture(t *testing.T, e *integration.Env) (connID, rawID ids.UUID) {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	connID = ids.NewV7()
	err := database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO channel_connection
				(id, provider, channel_id, channel_label, credential_ref, status, connected_by)
			VALUES ($1, 'telegram', '42', 'acme_bot', 'cred-ref', 'connected', $2)`,
			connID, e.Rep1)
		return err
	})
	if err != nil {
		t.Fatalf("seeding the channel connection: %v", err)
	}
	return connID, e.SeedRawCapture(t, "telegram", "100", []byte(telegramIngestUpdateJSON))
}

const telegramIngestUpdateJSON = `{
	"update_id": 100,
	"message": {
		"message_id": 7,
		"chat": {"id": 1001, "type": "private", "username": "annlee"},
		"from": {"id": 555, "username": "annlee", "first_name": "Ann", "last_name": "Lee"},
		"date": 1690000000,
		"text": "hello"
	}
}`

// telegramIngestKickedJSON is what Telegram posts when customer 556 blocks bot 42.
// my_chat_member reports the bot's own membership, so both chat members are bot 42.
// The customer appears only as the private chat and as `from`.
// Putting 556 in new_chat_member.user would prove nothing about production.
const telegramIngestKickedJSON = `{
	"update_id": 200,
	"my_chat_member": {
		"chat": {"id": 556, "type": "private", "username": "blockeduser", "first_name": "Blocks"},
		"from": {"id": 556, "username": "blockeduser", "first_name": "Blocks"},
		"date": 1690000200,
		"old_chat_member": {"user": {"id": 42, "is_bot": true, "username": "acme_bot"}, "status": "member"},
		"new_chat_member": {"user": {"id": 42, "is_bot": true, "username": "acme_bot"}, "status": "kicked"}
	}
}`

// The job queue is not a request.
// The worker must build its tenant context from job.Args alone, or it lands the activity in the wrong tenant.
func TestIngestWorkerReestablishesWorkspaceContextFromArgs(t *testing.T) {
	e := integration.Setup(t)
	connID, rawID := telegramIngestFixture(t, e)

	// No principal.WithWorkspaceID on the context: the worker itself must resolve
	// the tenant from job.Args, not from whatever the caller had bound.
	workIngestArgs(t, newTelegramIngestWorker(e.Pool, CaptureConfig{}, quietTestLogger()),
		TelegramIngestArgs{ConnectionID: connID.String(), BotID: "42", RawCaptureID: rawID.String()})

	// Read back under the SAME workspace the args named — if the worker had
	// resolved the wrong tenant (or none), this activity would not be here.
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	var id ids.UUID
	err := database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id FROM activity WHERE source_system = 'telegram'`).Scan(&id)
	})
	if err != nil {
		t.Fatalf("reading back the captured activity: %v", err)
	}
	for column, want := range map[string]string{
		"source_id": "42:1001:7", "source": "capture", "thread_key": "telegram:42:1001",
		"kind": "message", "channel_provider": "telegram", "direction": "inbound",
		"audience": "workspace", "captured_by": "connector:telegram",
	} {
		if got := e.WsScalar(t, `SELECT coalesce(`+column+`::text, '') FROM activity WHERE id = $1`, id); got != want {
			t.Errorf("activity.%s = %q, want %q", column, got, want)
		}
	}
	counts := map[string]string{
		"audit_log":    `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1`,
		"event_outbox": `SELECT count(*) FROM event_outbox WHERE envelope->'entity'->>'id' = $1::text`,
	}
	for table, query := range counts {
		if n := e.WsCount(t, query, id); n != 1 {
			t.Errorf("%s holds %d rows for the new activity, want 1", table, n)
		}
	}

	// The one identity row is the live binding of the contact the activity is filed on.
	const identityOfActivity = `
		SELECT i.id::text FROM contact_channel_identity i
		  JOIN activity_link l ON l.activity_id = $1 AND l.entity_type = 'contact' AND l.contact_id = i.contact_id
		 WHERE i.provider = 'telegram' AND i.channel_user_id = '555' AND i.archived_at IS NULL`
	identity := e.WsScalar(t, identityOfActivity, id)
	if identity == "" {
		t.Fatal("no live telegram identity for sender 555 on the contact the activity is filed on")
	}
	const identityRows = `SELECT count(*) FROM contact_channel_identity`
	rowsBefore := e.WsCount(t, identityRows)
	if rowsBefore != 1 {
		t.Errorf("contact_channel_identity holds %d rows after one message, want 1", rowsBefore)
	}

	// A redelivery of the same raw update lands nothing new.
	workIngestArgs(t, newTelegramIngestWorker(e.Pool, CaptureConfig{}, quietTestLogger()),
		TelegramIngestArgs{ConnectionID: connID.String(), BotID: "42", RawCaptureID: rawID.String()})
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE source_system = 'telegram'`); n != 1 {
		t.Errorf("%d telegram activities after a redelivery, want 1", n)
	}
	for table, query := range counts {
		if n := e.WsCount(t, query, id); n != 1 {
			t.Errorf("%s holds %d rows after a redelivery, want still 1", table, n)
		}
	}
	if n := e.WsCount(t, identityRows); n != rowsBefore {
		t.Errorf("contact_channel_identity holds %d rows after a redelivery, want still %d", n, rowsBefore)
	}
	if got := e.WsScalar(t, identityOfActivity, id); got != identity {
		t.Errorf("live identity is %q after a redelivery, want the original %q", got, identity)
	}
}

// uniqueViolationSink is a connector.Sink whose Upsert always fails with the
// SQLSTATE a real dedupe race raises.
// That proves the worker propagates the error without winning a real race.
type uniqueViolationSink struct{}

func (uniqueViolationSink) Upsert(context.Context, connector.NormalizedRecord) (datasource.EntityRef, error) {
	return datasource.EntityRef{}, &pgconn.PgError{Code: "23505", ConstraintName: "uq_contact_channel_identity"}
}

// A unique violation during capture is retryable, never poison.
// The loser of two concurrent first messages must redeliver.
// Swallowing it would silently drop a customer's message.
func TestIngestWorkerTreatsAUniqueViolationAsRetryable(t *testing.T) {
	e := integration.Setup(t)
	connID, rawID := telegramIngestFixture(t, e)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))

	worker := &telegramIngestWorker{pool: e.Pool, sink: uniqueViolationSink{}, log: quiet}
	err := worker.Work(context.Background(), &river.Job[TelegramIngestArgs]{
		Args: TelegramIngestArgs{ConnectionID: connID.String(), BotID: "42", RawCaptureID: rawID.String()},
	})
	if err == nil {
		t.Fatal("Work returned nil — a unique violation must propagate so River redelivers, not be swallowed")
	}
	if strings.Contains(err.Error(), "42:1001:7") {
		t.Errorf("the persisted River error contains the Telegram natural key %q", "42:1001:7")
	}
	if !storekit.IsUniqueViolation(err) {
		t.Errorf("got %v, want an error still classifiable as a unique violation (the worker rewrapped rather than propagated it)", err)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != "uq_contact_channel_identity" {
		t.Errorf("got %v, want the original constraint name preserved", err)
	}
}

// TestIngestWorkerAppliesMembershipWithoutCapturingAnActivity is the wiring
// proof that a my_chat_member update reaches the same worker path a message does.
// It must land as a reachability change only.
func TestIngestWorkerAppliesMembershipWithoutCapturingAnActivity(t *testing.T) {
	e := integration.Setup(t)
	connID, _ := telegramIngestFixture(t, e)

	// A prior message already bound sender 556 to a contact — the
	// my_chat_member update below reports THAT account blocking the bot.
	contact := e.SeedContact(t, "Blocks The Bot", nil)
	e.WsExec(t, `
		INSERT INTO contact_channel_identity (contact_id, provider, channel_user_id, username, source, captured_by)
		VALUES (
		        $1, 'telegram', '556', 'blockeduser', 'telegram', 'connector:telegram')`,
		contact)

	rawID := e.SeedRawCapture(t, "telegram", "200", []byte(telegramIngestKickedJSON))
	workIngestArgs(t, newTelegramIngestWorker(e.Pool, CaptureConfig{}, quietTestLogger()),
		TelegramIngestArgs{ConnectionID: connID.String(), BotID: "42", RawCaptureID: rawID.String()})

	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE source_system = 'telegram'`); n != 0 {
		t.Errorf("%d activity rows after a my_chat_member update, want 0 — it is a reachability signal, never a message", n)
	}
	if n := e.WsCount(t, `
		SELECT count(*) FROM contact_channel_identity
		 WHERE channel_user_id = '556' AND archived_at IS NULL AND blocked_at IS NOT NULL`); n != 1 {
		t.Errorf("%d channel identity rows carry blocked_at after a kicked status, want 1 — "+
			"the update's identity is the private chat's, and the bot's id (42) matches no Contact", n)
	}
}

// telegramIngestGroupJSON is a message in a supergroup the bot was added to:
// Telegram delivers it under the same bare `message` update a private chat
// uses, which is why the exclusion cannot be expressed in allowed_updates.
const telegramIngestGroupJSON = `{
	"update_id": 300,
	"message": {
		"message_id": 12,
		"chat": {"id": -1001234567890, "type": "supergroup", "title": "Acme Support"},
		"from": {"id": 557, "username": "groupmember", "first_name": "Group"},
		"date": 1690000500,
		"text": "/help please"
	}
}`

// Group chats are out of scope.
// The worker must ack the delivery and capture nothing.
// Otherwise a rep's reply would route to the sender's private chat, not the group.
func TestIngestWorkerCapturesNothingFromAGroupChat(t *testing.T) {
	e := integration.Setup(t)
	connID, _ := telegramIngestFixture(t, e)
	rawID := e.SeedRawCapture(t, "telegram", "300", []byte(telegramIngestGroupJSON))
	// nil, not an error: a scope exclusion is a deliberate skip, and River
	// retrying it forever would be a fault report for working as designed.
	workIngestArgs(t, newTelegramIngestWorker(e.Pool, CaptureConfig{}, quietTestLogger()),
		TelegramIngestArgs{ConnectionID: connID.String(), BotID: "42", RawCaptureID: rawID.String()})

	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE source_system = 'telegram'`); n != 0 {
		t.Errorf("%d activity rows after a group-chat message, want 0", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM contact_channel_identity WHERE channel_user_id = '557'`); n != 0 {
		t.Errorf("%d channel identities minted for a group member, want 0", n)
	}
}
