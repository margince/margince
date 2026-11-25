// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What one Telegram message persists once the ingest worker has filed it, and what
// a second delivery of the same message leaves behind. The natural key, the audit
// and outbox rows, the audience and the channel identity are read back from the
// database after the real worker has run, so a change to the shared filing code
// that moves any of them fails here by name.

import (
	"context"
	"testing"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// workTelegramIngestFixtureJob runs the fixture's raw update through the real
// worker, exactly as a queued job would.
func workTelegramIngestFixtureJob(t *testing.T, e *integration.Env, connID, rawID ids.UUID) {
	t.Helper()
	worker := newTelegramIngestWorker(e.Pool, CaptureConfig{}, quietTestLogger())
	err := worker.Work(context.Background(), &river.Job[TelegramIngestArgs]{
		Args: TelegramIngestArgs{Workspace: e.WS, ConnectionID: connID.String(), BotID: "42", RawCaptureID: rawID.String()},
	})
	if err != nil {
		t.Fatalf("ingest Work: %v", err)
	}
}

// telegramFixtureActivity reads the one activity the fixture's message became.
func telegramFixtureActivity(t *testing.T, e *integration.Env) ids.UUID {
	t.Helper()
	return ids.MustParse(e.WsScalar(t, `SELECT id::text FROM activity WHERE source_system = 'telegram'`))
}

func TestATelegramMessageIsFiledUnderItsBotChatMessageKeyAndWriteShape(t *testing.T) {
	e := integration.Setup(t)
	connID, rawID := telegramIngestFixture(t, e)

	workTelegramIngestFixtureJob(t, e, connID, rawID)

	id := telegramFixtureActivity(t, e)
	for column, want := range map[string]string{
		"source_system":    "telegram",
		"source_id":        "42:1001:7",
		"source":           "telegram:42:1001:7",
		"thread_key":       "telegram:42:1001",
		"kind":             "message",
		"channel_provider": "telegram",
		"direction":        "inbound",
		"audience":         "workspace",
		"captured_by":      "connector:telegram",
	} {
		got := e.WsScalar(t, `SELECT coalesce(`+column+`::text, '') FROM activity WHERE id = $1`, id)
		if got != want {
			t.Errorf("activity.%s = %q, want %q", column, got, want)
		}
	}

	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1`, id); n != 1 {
		t.Errorf("%d audit rows for the new activity, want 1", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->'entity'->>'id' = $1`, id.String()); n != 1 {
		t.Errorf("%d outbox events for the new activity, want 1", n)
	}
	if n := e.WsCount(t, `
		SELECT count(*) FROM contact_channel_identity
		 WHERE provider = 'telegram' AND channel_user_id = '555' AND archived_at IS NULL AND contact_id IS NOT NULL`); n != 1 {
		t.Errorf("%d live channel identities for telegram sender 555, want 1", n)
	}
	if n := e.WsCount(t, `
		SELECT count(*) FROM channel_connection
		 WHERE id = $1 AND provider = 'telegram' AND channel_id = '42' AND status = 'connected'`, connID); n != 1 {
		t.Errorf("%d intact connection rows after an ingest, want 1", n)
	}
}

func TestADeliveredTelegramMessageDeliveredAgainLandsNothingNew(t *testing.T) {
	e := integration.Setup(t)
	connID, rawID := telegramIngestFixture(t, e)

	workTelegramIngestFixtureJob(t, e, connID, rawID)
	id := telegramFixtureActivity(t, e)
	audits := e.WsCount(t, `SELECT count(*) FROM audit_log`)
	events := e.WsCount(t, `SELECT count(*) FROM event_outbox`)
	contacts := e.WsCount(t, `SELECT count(*) FROM contact`)
	identities := e.WsCount(t, `SELECT count(*) FROM contact_channel_identity`)

	workTelegramIngestFixtureJob(t, e, connID, rawID)

	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE source_system = 'telegram'`); n != 1 {
		t.Fatalf("%d telegram activities after a redelivery, want 1", n)
	}
	if again := telegramFixtureActivity(t, e); again != id {
		t.Errorf("the redelivery filed activity %s, want the original %s", again, id)
	}
	for table, before := range map[string]int{
		"audit_log": audits, "event_outbox": events, "contact": contacts, "contact_channel_identity": identities,
	} {
		if after := e.WsCount(t, `SELECT count(*) FROM `+table); after != before {
			t.Errorf("%s holds %d rows after a redelivery, want the %d it held before", table, after, before)
		}
	}
}
