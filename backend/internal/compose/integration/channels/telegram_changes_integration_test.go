// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package channels

// An edit or a deletion that arrives through the Telegram poll acts on the
// message the poll filed. The real poll, ingest worker and sink run over the
// faked Bot API.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// editedBusinessMessage is an edited_business_message of a message of this chat.
func (m businessMessage) edited(t *testing.T, updateID, editDate int64, text string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"update_id": updateID,
		"edited_business_message": map[string]any{
			"business_connection_id": m.connection,
			"message_id":             m.messageID,
			"chat":                   map[string]any{"id": m.chatID, "type": "private", "first_name": "Cora", "username": "cora"},
			"from":                   map[string]any{"id": m.fromID, "first_name": "Cora", "username": "cora"},
			"date":                   telegramProviderDate,
			"edit_date":              editDate,
			"text":                   text,
		},
	})
	if err != nil {
		t.Fatalf("rendering the edit: %v", err)
	}
	return raw
}

// deleted is a deleted_business_messages update for this message.
func (m businessMessage) deleted(t *testing.T, updateID int64) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"update_id": updateID,
		"deleted_business_messages": map[string]any{
			"business_connection_id": m.connection,
			"chat":                   map[string]any{"id": m.chatID, "type": "private", "first_name": "Cora"},
			"message_ids":            []int64{m.messageID},
		},
	})
	if err != nil {
		t.Fatalf("rendering the deletion: %v", err)
	}
	return raw
}

func (c *telegramEnv) filedBody(t *testing.T, sourceID string) string {
	t.Helper()
	var body string
	if err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT coalesce(body, '') FROM activity WHERE source_system = 'telegram' AND source_id = $1`, sourceID).Scan(&body)
	}); err != nil {
		t.Fatalf("reading the filed body: %v", err)
	}
	return body
}

func TestAnEditAndADeletionReachTheMessageThePollFiled(t *testing.T) {
	c := setupTelegramConnected(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)
	c.link(t, 880031, "bc-edit", "connected")
	m := businessMessage{updateID: 7801, messageID: 21, chatID: 990021, fromID: 990021, connection: "bc-edit", text: "Is it in stock?"}
	key := m.sourceID(880031)

	c.deliverMessage(t, sub, m)
	awaitJobKind(t, sub, compose.TelegramIngestArgs{}.Kind())
	filed, ok := c.filed(t, key)
	if !ok {
		t.Fatal("the message was not filed")
	}
	audits := c.count(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1`, filed.id)

	c.api.hold(m.edited(t, 7802, telegramProviderDate+60, "Is it in stock today?"))
	c.pollNow(t, sub, 7803)
	awaitJobKind(t, sub, compose.TelegramIngestArgs{}.Kind())
	if got := c.filedBody(t, key); got != "Is it in stock today?" {
		t.Fatalf("body %q after the edit, want the edited text", got)
	}
	if n := c.activitiesOnBot(t); n != 1 {
		t.Fatalf("%d activities after the edit, want the one message", n)
	}
	if got := c.count(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1`, filed.id); got != audits+1 {
		t.Fatalf("%d audit rows after the edit, want %d", got, audits+1)
	}

	// A company account's deletion removes nothing: anyone may delete there.
	c.api.hold(m.deleted(t, 7804))
	c.pollNow(t, sub, 7805)
	awaitJobKind(t, sub, compose.TelegramIngestArgs{}.Kind())
	if got := c.filedBody(t, key); got != "Is it in stock today?" {
		t.Fatalf("body %q after a deletion on a company account, want the team's copy kept", got)
	}
}

func TestAnEditOnALinkThatIsNotConnectedStoresNothing(t *testing.T) {
	for _, status := range []string{"pending_approval", "disconnected"} {
		t.Run(status, func(t *testing.T) {
			c := setupTelegramConnected(t)
			runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
			startTelegramWorker(t, runner)
			c.link(t, 880032, "bc-"+status, status)
			m := businessMessage{updateID: 7901, messageID: 1, chatID: 990022, fromID: 990022, connection: "bc-" + status}

			c.api.hold(m.edited(t, 7901, telegramProviderDate+60, "edited"))
			c.pollNow(t, sub, 7902)
			c.api.hold(m.deleted(t, 7903))
			c.pollNow(t, sub, 7904)

			if n := c.rawCaptures(t, 7901) + c.rawCaptures(t, 7903); n != 0 {
				t.Errorf("%d raw captures, want none", n)
			}
			if n := c.ingestJobs(t); n != 0 {
				t.Errorf("%d ingest jobs, want none", n)
			}
			if n := c.activitiesOnBot(t); n != 0 {
				t.Errorf("%d activities, want none", n)
			}
		})
	}
}
