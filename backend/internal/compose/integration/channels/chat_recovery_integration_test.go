// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package channels

// The delivery cases a chat network must survive, run against WhatsApp and Zalo.
// They use the real route, the real ingest worker and the shared filing path.
// Telegram Business has its own poll, so its two cases are written out at the
// end of this file.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/jobs"
)

// chatAccount is one connected provider account as a test drives it.
type chatAccount struct {
	id         string
	in, out    func(t *testing.T, msgID, text string)
	disconnect func(t *testing.T)
	reconnect  func(t *testing.T) chatAccount
}

// webhookNetwork is what differs between the networks the cases below share.
type webhookNetwork struct {
	name, job  string
	setup      func(t *testing.T) *telegramEnv
	connect    func(t *testing.T, c *telegramEnv, account string) chatAccount
	enqueue    func(ctx context.Context, in *jobs.Runner, args []byte) error
	filed      func(c *telegramEnv, t *testing.T, msgID string) (filedMessage, bool)
	activities func(c *telegramEnv, t *testing.T) int
	raw        func(c *telegramEnv, t *testing.T) int
	await      func(t *testing.T, sub <-chan *river.Event)
	thread     func(account string) string
}

func whatsAppAccount(t *testing.T, c *telegramEnv, n waNumber) chatAccount {
	t.Helper()
	deliver := func(t *testing.T, body string) {
		t.Helper()
		if status := c.deliverWA(t, n, body); status != http.StatusOK {
			t.Fatalf("delivery → %d, want 200", status)
		}
	}
	return chatAccount{
		id:         n.phoneID,
		in:         func(t *testing.T, id, text string) { deliver(t, waInbound(n.phoneID, id, text)) },
		out:        func(t *testing.T, id, text string) { deliver(t, waEcho(n.phoneID, id, text)) },
		disconnect: func(t *testing.T) { c.disconnectConnection(t, "whatsapp", n.id) },
		reconnect:  func(t *testing.T) chatAccount { return whatsAppAccount(t, c, c.connectNumberFor(t, n.phoneID, n.team)) },
	}
}

func zaloAccount(t *testing.T, c *telegramEnv, oa zaloOA) chatAccount {
	t.Helper()
	deliver := func(t *testing.T, body string) {
		t.Helper()
		if status := c.deliverZalo(t, oa, body); status != http.StatusOK {
			t.Fatalf("delivery → %d, want 200", status)
		}
	}
	return chatAccount{
		id:         oa.oaID,
		in:         func(t *testing.T, id, text string) { deliver(t, zaloInbound(oa.oaID, id, text)) },
		out:        func(t *testing.T, id, text string) { deliver(t, zaloOutbound(oa.oaID, id, text)) },
		disconnect: func(t *testing.T) { c.disconnectConnection(t, "zalo-oa", oa.id) },
		reconnect:  func(t *testing.T) chatAccount { return zaloAccount(t, c, c.connectOAFor(t, oa.oaID, oa.team)) },
	}
}

func webhookNetworks() []webhookNetwork {
	return []webhookNetwork{
		{
			name: "whatsapp", job: compose.WhatsAppIngestArgs{}.Kind(), setup: setupWhatsApp,
			connect: func(t *testing.T, c *telegramEnv, account string) chatAccount {
				return whatsAppAccount(t, c, c.connectNumber(t, account))
			},
			enqueue: func(ctx context.Context, in *jobs.Runner, args []byte) error {
				var a compose.WhatsAppIngestArgs
				if err := json.Unmarshal(args, &a); err != nil {
					return err
				}
				return in.Enqueue(ctx, a, nil)
			},
			filed:      (*telegramEnv).waFiled,
			activities: (*telegramEnv).waActivities,
			raw:        (*telegramEnv).waRaw,
			await:      awaitWA,
			thread:     func(account string) string { return "whatsapp:biz:" + account + ":491701234567" },
		},
		{
			name: "zalo", job: compose.ZaloOAIngestArgs{}.Kind(), setup: setupZalo,
			connect: func(t *testing.T, c *telegramEnv, account string) chatAccount {
				return zaloAccount(t, c, c.connectOA(t, account))
			},
			enqueue: func(ctx context.Context, in *jobs.Runner, args []byte) error {
				var a compose.ZaloOAIngestArgs
				if err := json.Unmarshal(args, &a); err != nil {
					return err
				}
				return in.Enqueue(ctx, a, nil)
			},
			filed:      (*telegramEnv).zaloFiled,
			activities: (*telegramEnv).zaloActivities,
			raw:        (*telegramEnv).zaloRaw,
			await:      awaitZalo,
			thread:     func(account string) string { return "zalo_oa:biz:" + account + ":" + zaloCustomer },
		},
	}
}

func (c *telegramEnv) disconnectConnection(t *testing.T, route, id string) {
	t.Helper()
	if status := c.Call(t, http.MethodDelete, "/v1/channel-connections/"+route+"/"+id, nil, nil, nil); status != http.StatusNoContent {
		t.Fatalf("disconnect → %d, want 204", status)
	}
}

// storedJobArgs reads the args of every ingest job the webhook queued.
func (c *telegramEnv) storedJobArgs(t *testing.T, kind string) [][]byte {
	t.Helper()
	rows, err := c.Pool.Query(context.Background(), `SELECT args FROM river_job WHERE kind = $1 ORDER BY id`, kind)
	if err != nil {
		t.Fatalf("reading the queued %s jobs: %v", kind, err)
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var args []byte
		if err := rows.Scan(&args); err != nil {
			t.Fatalf("scanning job args: %v", err)
		}
		out = append(out, args)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the queued %s jobs: %v", kind, err)
	}
	return out
}

func (c *telegramEnv) contacts(t *testing.T) int {
	return c.count(t, `SELECT count(*) FROM contact`)
}

// assertFiledOnce checks one message is one activity on one contact with one
// audit row, however many attempts reached it.
func assertFiledOnce(t *testing.T, c *telegramEnv, n webhookNetwork, msgID string, contactsBefore int) {
	t.Helper()
	filed, ok := n.filed(c, t, msgID)
	if !ok {
		t.Fatalf("%s: message %s was not filed", n.name, msgID)
	}
	if got := n.activities(c, t); got != 1 {
		t.Errorf("%s: %d activities, want 1", n.name, got)
	}
	if got := n.raw(c, t); got != 1 {
		t.Errorf("%s: %d raw originals, want 1", n.name, got)
	}
	if got := c.contacts(t) - contactsBefore; got != 1 {
		t.Errorf("%s: %d contacts made, want 1", n.name, got)
	}
	if got := c.count(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1`, filed.id); got != 1 {
		t.Errorf("%s: %d audit rows for the activity, want 1", n.name, got)
	}
}

func TestTheSameMessageRunByTwoWorkersAtOnceFilesOnce(t *testing.T) {
	for _, n := range webhookNetworks() {
		t.Run(n.name, func(t *testing.T) {
			c := n.setup(t)
			runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
			acct := n.connect(t, c, "8101")
			contactsBefore := c.contacts(t)
			acct.in(t, "dup-1", "one message")
			queued := c.storedJobArgs(t, n.job)
			if len(queued) != 1 {
				t.Fatalf("%d jobs queued by one delivery, want 1", len(queued))
			}
			const copies = 3
			for range copies {
				if err := n.enqueue(context.Background(), c.inserter, queued[0]); err != nil {
					t.Fatalf("queueing a second run of the same item: %v", err)
				}
			}
			startTelegramWorker(t, runner)
			for range copies + 1 {
				n.await(t, sub)
			}
			assertFiledOnce(t, c, n, "dup-1", contactsBefore)
		})
	}
}

func TestAReplyThatArrivesBeforeTheCustomerMessageFilesInOneThreadOnOneContact(t *testing.T) {
	for _, n := range webhookNetworks() {
		t.Run(n.name, func(t *testing.T) {
			c := n.setup(t)
			runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
			startTelegramWorker(t, runner)
			acct := n.connect(t, c, "8102")
			contactsBefore := c.contacts(t)

			acct.out(t, "reply-1", "Yes, shipping Monday.")
			n.await(t, sub)
			acct.in(t, "ask-1", "Is it in stock?")
			n.await(t, sub)

			reply, okReply := n.filed(c, t, "reply-1")
			ask, okAsk := n.filed(c, t, "ask-1")
			if !okReply || !okAsk {
				t.Fatalf("filed reply %v, customer message %v, want both", okReply, okAsk)
			}
			if reply.direction != "outbound" || ask.direction != "inbound" {
				t.Errorf("directions = %q and %q, want outbound and inbound", reply.direction, ask.direction)
			}
			if want := n.thread(acct.id); reply.thread != want || ask.thread != want {
				t.Errorf("threads = %q and %q, want one thread %q", reply.thread, ask.thread, want)
			}
			if reply.contact == "" || ask.contact != reply.contact {
				t.Errorf("contacts = %q and %q, want one customer contact", reply.contact, ask.contact)
			}
			if got := c.contacts(t) - contactsBefore; got != 1 {
				t.Errorf("%d contacts made, want 1", got)
			}
		})
	}
}

// failEventOutbox makes every insert into event_outbox fail until the returned
// function lifts it. The filing writes its event in the same transaction as the
// activity. A run that reaches it fails after part of its work and must leave
// nothing behind.
func failEventOutbox(t *testing.T) (lift func()) {
	t.Helper()
	conn := integration.OwnerConn(t)
	ctx := context.Background()
	for _, stmt := range []string{
		`CREATE OR REPLACE FUNCTION chat_recovery_fault() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected commit fault'; END $$`,
		`CREATE TRIGGER chat_recovery_fault BEFORE INSERT ON event_outbox FOR EACH ROW EXECUTE FUNCTION chat_recovery_fault()`,
	} {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("arming the commit fault: %v", err)
		}
	}
	lifted := false
	lift = func() {
		if lifted {
			return
		}
		lifted = true
		owner := integration.OwnerConn(t)
		for _, stmt := range []string{
			`DROP TRIGGER IF EXISTS chat_recovery_fault ON event_outbox`,
			`DROP FUNCTION IF EXISTS chat_recovery_fault()`,
		} {
			if _, err := owner.Exec(ctx, stmt); err != nil {
				t.Errorf("lifting the commit fault: %v", err)
			}
		}
	}
	t.Cleanup(lift)
	return lift
}

func awaitJobFailure(t *testing.T, failed <-chan *river.Event, kind string) {
	t.Helper()
	timeout := time.After(60 * time.Second)
	for {
		select {
		case <-timeout:
			t.Fatalf("timed out waiting for a %s job to fail", kind)
		case ev := <-failed:
			if ev != nil && ev.Job != nil && ev.Job.Kind == kind {
				return
			}
		}
	}
}

func TestAnIngestThatFailsMidFilingLeavesNothingAndItsRetryFilesOnce(t *testing.T) {
	for _, n := range webhookNetworks() {
		t.Run(n.name, func(t *testing.T) {
			c := n.setup(t)
			runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
			failed, cancelFailed := runner.SubscribeFailed()
			t.Cleanup(cancelFailed)
			acct := n.connect(t, c, "8103")
			contactsBefore := c.contacts(t)
			acct.in(t, "retry-1", "one message")
			lift := failEventOutbox(t)

			startTelegramWorker(t, runner)
			awaitJobFailure(t, failed, n.job)
			if a, ct := n.activities(c, t), c.contacts(t)-contactsBefore; a != 0 || ct != 0 {
				t.Errorf("a failed run left %d activities and %d contacts, want none", a, ct)
			}
			if got := n.raw(c, t); got != 1 {
				t.Fatalf("a failed run left %d raw originals, want the one the retry reads", got)
			}

			lift()
			n.await(t, sub)
			assertFiledOnce(t, c, n, "retry-1", contactsBefore)
		})
	}
}

func TestAReconnectedAccountKeepsItsThreadAndContact(t *testing.T) {
	for _, n := range webhookNetworks() {
		t.Run(n.name, func(t *testing.T) {
			c := n.setup(t)
			runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
			startTelegramWorker(t, runner)
			contactsBefore := c.contacts(t)

			first := n.connect(t, c, "8104")
			first.in(t, "before-1", "before the disconnect")
			n.await(t, sub)
			first.disconnect(t)
			again := first.reconnect(t)
			again.in(t, "after-1", "after the reconnect")
			n.await(t, sub)

			before, ok1 := n.filed(c, t, "before-1")
			after, ok2 := n.filed(c, t, "after-1")
			if !ok1 || !ok2 {
				t.Fatalf("filed before %v, after %v, want both", ok1, ok2)
			}
			if before.thread != after.thread || before.thread != n.thread("8104") {
				t.Errorf("threads = %q and %q, want one thread %q", before.thread, after.thread, n.thread("8104"))
			}
			if before.contact == "" || before.contact != after.contact {
				t.Errorf("contacts = %q and %q, want one customer contact", before.contact, after.contact)
			}
			if got := c.contacts(t) - contactsBefore; got != 1 {
				t.Errorf("%d contacts made across the reconnect, want 1", got)
			}
		})
	}
}

func TestTwoAccountsThatSeeTheSameProviderIDsKeepTheirMessagesApart(t *testing.T) {
	for _, n := range webhookNetworks() {
		t.Run(n.name, func(t *testing.T) {
			c := n.setup(t)
			runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
			startTelegramWorker(t, runner)
			a, b := n.connect(t, c, "8105"), n.connect(t, c, "8106")

			a.in(t, "same-id", "to a")
			n.await(t, sub)
			b.in(t, "same-id", "to b")
			n.await(t, sub)

			if got := n.activities(c, t); got != 2 {
				t.Fatalf("%d activities for one provider message id on two accounts, want 2", got)
			}
			if got := n.raw(c, t); got != 2 {
				t.Errorf("%d raw originals, want one per account", got)
			}
			for account, want := range map[string]string{"8105": n.thread("8105"), "8106": n.thread("8106")} {
				if got := c.count(t, `SELECT count(*) FROM activity WHERE thread_key = $1`, want); got != 1 {
					t.Errorf("account %s: %d activities on thread %q, want 1", account, got, want)
				}
			}
		})
	}
}

func TestATelegramOwnersReplyThatArrivesBeforeTheCustomerMessageFilesInOneThreadOnOneContact(t *testing.T) {
	c := setupTelegramConnected(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)
	c.link(t, 880031, "bc-order", "connected")
	reply := businessMessage{updateID: 7501, messageID: 21, chatID: 990006, fromID: 880031, connection: "bc-order", text: "Yes, shipping Monday."}
	ask := businessMessage{updateID: 7502, messageID: 20, chatID: 990006, fromID: 990006, connection: "bc-order", text: "Is it in stock?"}
	contactsBefore := c.contacts(t)

	c.deliverMessage(t, sub, reply)
	awaitJobKind(t, sub, compose.TelegramIngestArgs{}.Kind())
	c.deliverMessage(t, sub, ask)
	awaitJobKind(t, sub, compose.TelegramIngestArgs{}.Kind())

	first, ok1 := c.filed(t, reply.sourceID(880031))
	second, ok2 := c.filed(t, ask.sourceID(880031))
	if !ok1 || !ok2 {
		t.Fatalf("filed reply %v, customer message %v, want both", ok1, ok2)
	}
	if first.direction != "outbound" || second.direction != "inbound" {
		t.Errorf("directions = %q and %q, want outbound and inbound", first.direction, second.direction)
	}
	if first.thread != "telegram:biz:880031:990006" || second.thread != first.thread {
		t.Errorf("threads = %q and %q, want one thread telegram:biz:880031:990006", first.thread, second.thread)
	}
	if first.contact == "" || second.contact != first.contact {
		t.Errorf("contacts = %q and %q, want one customer contact", first.contact, second.contact)
	}
	if got := c.contacts(t) - contactsBefore; got != 1 {
		t.Errorf("%d contacts made, want 1", got)
	}
}

// A first run fails after part of its work. Three more runs of the same item are
// queued while the fault holds. Once the fault lifts, the runs retry together and
// must end as one filed message.
func TestATelegramUpdateThatFailsMidFilingAndIsRunAgainByAFewWorkersFilesOnce(t *testing.T) {
	c := setupTelegramConnected(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	failed, cancelFailed := runner.SubscribeFailed()
	t.Cleanup(cancelFailed)
	c.link(t, 880041, "bc-retry", "connected")
	lift := failEventOutbox(t)
	startTelegramWorker(t, runner)
	m := businessMessage{updateID: 7601, messageID: 31, chatID: 990007, fromID: 990007, connection: "bc-retry", text: "one message"}
	contactsBefore := c.contacts(t)

	c.api.hold(m.body(t))
	c.pollNow(t, sub, m.updateID+1)
	awaitJobFailure(t, failed, compose.TelegramIngestArgs{}.Kind())
	if a := c.activitiesOnBot(t); a != 0 {
		t.Fatalf("a failed run left %d activities, want none", a)
	}

	queued := c.storedJobArgs(t, compose.TelegramIngestArgs{}.Kind())
	if len(queued) != 1 {
		t.Fatalf("%d ingest jobs queued by one update, want 1", len(queued))
	}
	var args compose.TelegramIngestArgs
	if err := json.Unmarshal(queued[0], &args); err != nil {
		t.Fatalf("reading the queued job: %v", err)
	}
	const copies = 3
	for range copies {
		if err := c.inserter.Enqueue(context.Background(), args, nil); err != nil {
			t.Fatalf("queueing another run of the same update: %v", err)
		}
	}
	lift()
	for range copies + 1 {
		awaitJobKind(t, sub, compose.TelegramIngestArgs{}.Kind())
	}

	filed, ok := c.filed(t, m.sourceID(880041))
	if !ok {
		t.Fatal("the update was not filed")
	}
	if got := c.activitiesOnBot(t); got != 1 {
		t.Errorf("%d activities, want 1", got)
	}
	if got := c.rawCaptures(t, m.updateID); got != 1 {
		t.Errorf("%d raw originals, want 1", got)
	}
	if got := c.contacts(t) - contactsBefore; got != 1 {
		t.Errorf("%d contacts made, want 1", got)
	}
	if got := c.count(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1`, filed.id); got != 1 {
		t.Errorf("%d audit rows for the activity, want 1", got)
	}
}
