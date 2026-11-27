// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package channels

// Messages that arrive through a Telegram Business link. The real poll, ingest
// worker, sink and send path run over the faked Bot API. A link row is written
// directly where a case needs a state or an owner the admin surface cannot
// produce.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// businessMessage is one business_message update. from is who typed it.
type businessMessage struct {
	updateID, messageID, chatID, fromID int64
	connection, chatType, text          string
}

func (m businessMessage) body(t *testing.T) json.RawMessage {
	t.Helper()
	if m.chatType == "" {
		m.chatType = "private"
	}
	raw, err := json.Marshal(map[string]any{
		"update_id": m.updateID,
		"business_message": map[string]any{
			"business_connection_id": m.connection,
			"message_id":             m.messageID,
			"chat":                   map[string]any{"id": m.chatID, "type": m.chatType, "first_name": "Cora", "username": "cora"},
			"from":                   map[string]any{"id": m.fromID, "first_name": "Cora", "username": "cora"},
			"date":                   telegramProviderDate,
			"text":                   m.text,
		},
	})
	if err != nil {
		t.Fatalf("rendering the business message: %v", err)
	}
	return raw
}

// sourceID is the key the activity must be filed under.
func (m businessMessage) sourceID(account int64) string {
	return fmt.Sprintf("%d:biz:%d:%d:%d", telegramBotID, account, m.chatID, m.messageID)
}

// link writes one business link row of this bot. A link an admin approved, now
// connected or switched off, has the seat approval gave it, shared. Cases that are not about a seat's posture
// then read as they would on an opened account.
func (c *telegramEnv) link(t *testing.T, account int64, connection, status string) {
	t.Helper()
	c.linkOnSeat(t, account, connection, status, "shared")
}

// linkOnSeat is link with the posture the connected link's seat starts at.
func (c *telegramEnv) linkOnSeat(t *testing.T, account int64, connection, status, posture string) {
	t.Helper()
	team := c.readerTeam(t)
	if err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		var seat *string
		if status == "connected" || status == "disconnected" {
			var id string
			if err := tx.QueryRow(context.Background(), `
				INSERT INTO capture_connection (provider, user_id, connected_by, reader_team_id, account_label, mail_posture, status)
				VALUES ('telegram_business', NULL, $1::uuid, $2, $3, $4, $5) RETURNING id::text`,
				c.admin, team, fmt.Sprintf("%d", account), posture, status).Scan(&id); err != nil {
				return err
			}
			seat = &id
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO channel_connection
			  (provider, channel_id, channel_label, credential_ref, status, connected_by, account_id,
			   parent_connection_id, ownership, approved_at, seat_id)
			VALUES ('telegram_business', $1, 'Mia', '', $2, $3, $4, $5, 'workspace', now(), $6::uuid)`,
			connection, status, c.admin, fmt.Sprintf("%d", account), c.conn.ID, seat)
		return err
	}); err != nil {
		t.Fatalf("writing the business link: %v", err)
	}
}

func (c *telegramEnv) deliverMessage(t *testing.T, sub <-chan *river.Event, m businessMessage) {
	t.Helper()
	c.api.hold(m.body(t))
	c.pollNow(t, sub, m.updateID+1)
}

type filedMessage struct {
	id, direction, audience, thread, contact string
}

func (c *telegramEnv) filed(t *testing.T, sourceID string) (filedMessage, bool) {
	t.Helper()
	var f filedMessage
	var found bool
	if err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			SELECT a.id::text, a.direction, a.audience, a.thread_key, coalesce(l.contact_id::text, '')
			  FROM activity a LEFT JOIN activity_link l ON l.activity_id = a.id AND l.entity_type = 'contact'
			 WHERE a.source_system = 'telegram' AND a.source_id = $1`, sourceID)
		if err != nil {
			return err
		}
		defer rows.Close()
		if rows.Next() {
			found = true
			return rows.Scan(&f.id, &f.direction, &f.audience, &f.thread, &f.contact)
		}
		return rows.Err()
	}); err != nil {
		t.Fatalf("reading the filed message: %v", err)
	}
	return f, found
}

func (c *telegramEnv) activitiesOnBot(t *testing.T) int {
	t.Helper()
	return c.count(t, `SELECT count(*) FROM activity WHERE source_system = 'telegram'`)
}

func (c *telegramEnv) assertNothingStored(t *testing.T, m businessMessage) {
	t.Helper()
	if n := c.rawCaptures(t, m.updateID); n != 0 {
		t.Errorf("%d raw captures for update %d, want none", n, m.updateID)
	}
	if n := c.ingestJobs(t); n != 0 {
		t.Errorf("%d ingest jobs, want none", n)
	}
	if n := c.activitiesOnBot(t); n != 0 {
		t.Errorf("%d activities, want none", n)
	}
}

func TestACustomerMessageAndTheOwnersReplyFileInOneThreadOnOneContact(t *testing.T) {
	t.Parallel()
	c := setupTelegramConnected(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)
	c.link(t, 880001, "bc-mia", "connected")
	in := businessMessage{updateID: 7001, messageID: 11, chatID: 990001, fromID: 990001, connection: "bc-mia", text: "Is it in stock?"}
	out := businessMessage{updateID: 7002, messageID: 12, chatID: 990001, fromID: 880001, connection: "bc-mia", text: "Yes, shipping Monday."}

	c.deliverMessage(t, sub, in)
	awaitJobKind(t, sub, compose.TelegramIngestArgs{}.Kind())
	c.deliverMessage(t, sub, out)
	awaitJobKind(t, sub, compose.TelegramIngestArgs{}.Kind())

	first, ok1 := c.filed(t, in.sourceID(880001))
	second, ok2 := c.filed(t, out.sourceID(880001))
	if !ok1 || !ok2 {
		t.Fatalf("filed customer message %v, owner reply %v, want both", ok1, ok2)
	}
	if first.direction != "inbound" || second.direction != "outbound" {
		t.Errorf("directions = %q and %q, want inbound then outbound", first.direction, second.direction)
	}
	if first.thread != "telegram:biz:880001:990001" || second.thread != first.thread {
		t.Errorf("threads = %q and %q, want one thread telegram:biz:880001:990001", first.thread, second.thread)
	}
	if first.contact == "" || second.contact != first.contact {
		t.Errorf("contacts = %q and %q, want the same customer contact", first.contact, second.contact)
	}
	if first.audience != "workspace" || second.audience != "workspace" {
		t.Errorf("audiences = %q and %q, want workspace", first.audience, second.audience)
	}
	if n := c.count(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1`, first.id); n != 1 {
		t.Errorf("%d audit rows for the customer message, want 1", n)
	}
	if n := c.count(t, `SELECT count(*) FROM event_outbox WHERE envelope::text LIKE '%' || $1::text || '%'`, first.id); n < 1 {
		t.Error("the customer message left no outbox event")
	}

	// Telegram sends both again after a lost cursor.
	c.rewindPollCursor(t, 0)
	c.pollNow(t, sub, out.updateID+1)
	if n := c.activitiesOnBot(t); n != 2 {
		t.Errorf("%d activities after a redelivery, want 2", n)
	}
}

func TestAMessageOnALinkThatIsNotConnectedStoresNothing(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"pending_approval", "disconnected"} {
		t.Run(status, func(t *testing.T) {
			c := setupTelegramConnected(t)
			runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
			startTelegramWorker(t, runner)
			c.link(t, 880002, "bc-"+status, status)
			m := businessMessage{updateID: 7101, messageID: 1, chatID: 990002, fromID: 990002, connection: "bc-" + status, text: "hello"}

			c.deliverMessage(t, sub, m)

			c.assertNothingStored(t, m)
		})
	}
}

func TestABusinessMessageNamingNoKnownLinkOrAGroupStoresNothing(t *testing.T) {
	t.Parallel()
	c := setupTelegramConnected(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)
	c.link(t, 880003, "bc-known", "connected")
	unknown := businessMessage{updateID: 7201, messageID: 1, chatID: 990003, fromID: 990003, connection: "bc-unknown", text: "hi"}
	group := businessMessage{updateID: 7202, messageID: 2, chatID: -100777, fromID: 990003, connection: "bc-known", chatType: "supergroup", text: "hi all"}

	c.deliverMessage(t, sub, unknown)
	c.deliverMessage(t, sub, group)

	c.assertNothingStored(t, unknown)
	c.assertNothingStored(t, group)
}

func TestTwoBusinessAccountsOnOneBotKeepTheSameChatAndMessageIDApart(t *testing.T) {
	t.Parallel()
	c := setupTelegramConnected(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)
	c.link(t, 880011, "bc-a", "connected")
	c.link(t, 880012, "bc-b", "connected")
	a := businessMessage{updateID: 7301, messageID: 5, chatID: 990004, fromID: 990004, connection: "bc-a", text: "to a"}
	b := businessMessage{updateID: 7302, messageID: 5, chatID: 990004, fromID: 990004, connection: "bc-b", text: "to b"}

	c.deliverMessage(t, sub, a)
	awaitJobKind(t, sub, compose.TelegramIngestArgs{}.Kind())
	c.deliverMessage(t, sub, b)
	awaitJobKind(t, sub, compose.TelegramIngestArgs{}.Kind())

	fa, okA := c.filed(t, a.sourceID(880011))
	fb, okB := c.filed(t, b.sourceID(880012))
	if !okA || !okB || fa.id == fb.id {
		t.Fatalf("account a %+v (%v), account b %+v (%v), want two distinct activities", fa, okA, fb, okB)
	}
	if fa.thread == fb.thread {
		t.Errorf("both accounts share thread %q", fa.thread)
	}
}

func TestAReconnectedAccountKeepsItsKeys(t *testing.T) {
	t.Parallel()
	c := setupTelegramConnected(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)
	c.link(t, 880021, "bc-old", "connected")
	m := businessMessage{updateID: 7401, messageID: 9, chatID: 990005, fromID: 990005, connection: "bc-old", text: "first"}
	c.deliverMessage(t, sub, m)
	awaitJobKind(t, sub, compose.TelegramIngestArgs{}.Kind())

	if err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE channel_connection SET channel_id = 'bc-new' WHERE channel_id = 'bc-old'`)
		return err
	}); err != nil {
		t.Fatalf("reconnecting the account: %v", err)
	}
	again := businessMessage{updateID: 7402, messageID: 10, chatID: 990005, fromID: 990005, connection: "bc-new", text: "second"}
	c.deliverMessage(t, sub, again)
	awaitJobKind(t, sub, compose.TelegramIngestArgs{}.Kind())

	first, _ := c.filed(t, m.sourceID(880021))
	second, ok := c.filed(t, again.sourceID(880021))
	if !ok || first.thread != second.thread || first.contact != second.contact {
		t.Errorf("after a reconnect: %+v then %+v (%v), want one thread and contact", first, second, ok)
	}
}

func TestABusinessThreadCannotBeRepliedToAndABotThreadCan(t *testing.T) {
	t.Parallel()
	c := setupTelegramConnected(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{SendRegistry: c.sendRegistry()})
	startTelegramWorker(t, runner)
	c.link(t, 880041, "bc-ro", "connected")
	biz := businessMessage{updateID: 7601, messageID: 4, chatID: 990007, fromID: 990007, connection: "bc-ro", text: "question"}
	bot := telegramUpdate{updateID: 7602, messageID: 5, senderID: 990008, username: "bob", firstName: "Bob", text: "hello bot"}

	c.deliverMessage(t, sub, biz)
	awaitJobKind(t, sub, compose.TelegramIngestArgs{}.Kind())
	c.arrive(t, sub, bot)
	awaitJobKind(t, sub, compose.TelegramIngestArgs{}.Kind())
	bizFiled, _ := c.filed(t, biz.sourceID(880041))
	botActivity, botContact := c.capturedMessage(t, bot)
	c.grantConsent(t, bizFiled.contact, "transactional")
	c.grantConsent(t, botContact, "transactional")

	var refusal struct {
		Code string `json:"code"`
	}
	status := c.Call(t, http.MethodPost, "/v1/activities/"+bizFiled.id+"/send-message", integration.AnyMap{
		"body": "reply from the CRM", "consent_purpose": "transactional",
	}, nil, &refusal)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("a reply into a business thread → %d, want 422", status)
	}
	if n := len(c.api.sentMessages()); n != 0 {
		t.Errorf("%d messages reached the Telegram API, want none", n)
	}
	if n := c.count(t, `SELECT count(*) FROM activity WHERE direction = 'outbound' AND thread_key = $1`, bizFiled.thread); n != 0 {
		t.Errorf("%d outbound activities staged on the business thread, want none", n)
	}

	if status := c.Call(t, http.MethodPost, "/v1/activities/"+botActivity+"/send-message", integration.AnyMap{
		"body": "hello Bob", "consent_purpose": "transactional",
	}, nil, nil); status != http.StatusAccepted {
		t.Fatalf("a reply into a bot thread → %d, want 202", status)
	}
	awaitJobKind(t, sub, compose.SendEmailArgs{}.Kind())
	if n := len(c.api.sentMessages()); n != 1 {
		t.Errorf("%d messages reached the Telegram API after the bot reply, want 1", n)
	}
}

func TestAMessageToTheBotStillFilesInTheBotsOwnNamespace(t *testing.T) {
	t.Parallel()
	c := setupTelegramConnected(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)
	u := telegramUpdate{updateID: 7701, messageID: 6, senderID: 990009, username: "eve", firstName: "Eve", text: "hi"}

	c.arrive(t, sub, u)
	awaitJobKind(t, sub, compose.TelegramIngestArgs{}.Kind())

	filed, ok := c.filed(t, u.naturalKey())
	if !ok || filed.audience != "workspace" || filed.direction != "inbound" || filed.thread != fmt.Sprintf("telegram:%d:%d", telegramBotID, u.senderID) {
		t.Errorf("bot message filed as %+v (%v), want an inbound workspace message in the bot's own thread", filed, ok)
	}
}

func TestALinkSwitchedOffAfterThePollAdmittedAMessageFilesNothingAndKeepsNoOriginal(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"owner turned it off": `UPDATE channel_connection SET status = 'disconnected' WHERE channel_id = $1`,
		"admin disconnected":  `UPDATE channel_connection SET status = 'disconnected', archived_at = now() WHERE channel_id = $1`,
	}
	for name, switchOff := range cases {
		t.Run(name, func(t *testing.T) {
			c := setupTelegramConnected(t)
			runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
			startTelegramWorker(t, runner)
			c.link(t, 880051, "bc-late", "connected")
			m := businessMessage{updateID: 7701, messageID: 1, chatID: 990051, fromID: 990051, connection: "bc-late", text: "too late"}

			// A connection of its own, because the suite's owner connection is one
			// session and a second transaction on it would share this one's state.
			// The uncommitted update holds the link row, so the ingest job the poll
			// queues waits at its lock until the switch-off commits.
			hold, err := c.Pool.Begin(context.Background())
			if err != nil {
				t.Fatalf("opening the switch-off transaction: %v", err)
			}
			defer func() {
				if err := hold.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
					t.Errorf("rolling back the switch-off: %v", err)
				}
			}()
			if _, err := hold.Exec(context.Background(), switchOff, "bc-late"); err != nil {
				t.Fatalf("switching the link off: %v", err)
			}
			c.api.hold(m.body(t))
			c.pollNow(t, sub, m.updateID+1)
			if n := c.rawCaptures(t, m.updateID); n != 1 {
				t.Errorf("%d raw captures once the poll admitted the message, want 1", n)
			}
			if err := hold.Commit(context.Background()); err != nil {
				t.Fatalf("committing the switch-off: %v", err)
			}
			awaitJobKind(t, sub, compose.TelegramIngestArgs{}.Kind())

			if n := c.activitiesOnBot(t); n != 0 {
				t.Errorf("%d activities, want none", n)
			}
			if n := c.rawCaptures(t, m.updateID); n != 0 {
				t.Errorf("%d raw captures left, want none", n)
			}
		})
	}
}

func TestABatchAppliesALinkChangeBeforeAdmittingTheMessagesAfterIt(t *testing.T) {
	t.Parallel()
	const account, connection = 880061, "bc-order"
	change := func(id int64, enabled bool) businessUpdate {
		return businessUpdate{updateID: id, connectionID: connection, ownerID: account, firstName: "Ola", enabled: enabled}
	}
	message := func(id int64) businessMessage {
		return businessMessage{updateID: id, messageID: id, chatID: 990061, fromID: 990061, connection: connection, text: "hi"}
	}
	cases := []struct {
		name   string
		status string
		batch  func(t *testing.T) []json.RawMessage
		last   int64
		stored bool
	}{
		{"a message after a disable", "connected", func(t *testing.T) []json.RawMessage {
			return []json.RawMessage{change(7801, false).body(t), message(7802).body(t)}
		}, 7802, false},
		{"a message before an enable", "disconnected", func(t *testing.T) []json.RawMessage {
			return []json.RawMessage{message(7811).body(t), change(7812, true).body(t)}
		}, 7812, false},
		{"a message after an enable", "disconnected", func(t *testing.T) []json.RawMessage {
			return []json.RawMessage{change(7821, true).body(t), message(7822).body(t)}
		}, 7822, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := setupTelegramConnected(t)
			runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
			startTelegramWorker(t, runner)
			c.link(t, account, connection, tc.status)

			for _, update := range tc.batch(t) {
				c.api.hold(update)
			}
			c.pollNow(t, sub, tc.last+1)

			if !tc.stored {
				if n := c.ingestJobs(t); n != 0 {
					t.Fatalf("%d ingest jobs, want none", n)
				}
				if n := c.rawCaptures(t, tc.last); n != 0 {
					t.Errorf("%d raw captures of the refused message, want none", n)
				}
				return
			}
			awaitJobKind(t, sub, compose.TelegramIngestArgs{}.Kind())
			if _, ok := c.filed(t, message(tc.last).sourceID(account)); !ok {
				t.Error("the message after an enable was not filed")
			}
		})
	}
}
