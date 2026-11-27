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
	"maps"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
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

// filingHoldKey is the advisory lock the filing hold waits on.
const filingHoldKey = 7770001

// holdFilingUntilOverlap holds the first filing transaction open at its event
// insert, until a second worker is observably waiting on a lock of its own. The
// first worker already holds the lock of the message key by then, so the second
// is inside filing the same message. Release lets both go.
func holdFilingUntilOverlap(t *testing.T) (overlapped, release func()) {
	t.Helper()
	ctx := context.Background()
	holder, watcher, owner := integration.FreshOwnerConn(t), integration.FreshOwnerConn(t), integration.OwnerConn(t)
	if _, err := holder.Exec(ctx, `SELECT pg_advisory_lock($1)`, filingHoldKey); err != nil {
		t.Fatalf("taking the hold: %v", err)
	}
	for _, stmt := range []string{
		`CREATE OR REPLACE FUNCTION chat_recovery_hold() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_advisory_xact_lock(7770001); RETURN NEW; END $$`,
		`CREATE TRIGGER chat_recovery_hold BEFORE INSERT ON event_outbox FOR EACH ROW WHEN (NEW.envelope->'actor'->>'type' = 'connector') EXECUTE FUNCTION chat_recovery_hold()`,
	} {
		if _, err := owner.Exec(ctx, stmt); err != nil {
			t.Fatalf("arming the hold: %v", err)
		}
	}
	released := false
	release = func() {
		if released {
			return
		}
		released = true
		if _, err := holder.Exec(ctx, `SELECT pg_advisory_unlock($1)`, filingHoldKey); err != nil {
			t.Errorf("releasing the hold: %v", err)
		}
		for _, stmt := range []string{`DROP TRIGGER IF EXISTS chat_recovery_hold ON event_outbox`, `DROP FUNCTION IF EXISTS chat_recovery_hold()`} {
			if _, err := integration.OwnerConn(t).Exec(ctx, stmt); err != nil {
				t.Errorf("lifting the hold: %v", err)
			}
		}
	}
	t.Cleanup(release)
	overlapped = func() {
		t.Helper()
		limit, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		for {
			var held, other int
			if err := watcher.QueryRow(ctx, `SELECT count(*) FILTER (WHERE objid = $1), count(*) FILTER (WHERE objid <> $1)
				FROM pg_locks WHERE locktype = 'advisory' AND NOT granted`, filingHoldKey).Scan(&held, &other); err != nil {
				t.Fatalf("reading the waiting locks: %v", err)
			}
			if held >= 1 && other >= 1 {
				return
			}
			if limit.Err() != nil {
				t.Fatalf("%d workers wait on the hold and %d on another lock: no second worker entered filing while the first held it", held, other)
			}
			if _, err := watcher.Exec(ctx, `SELECT pg_sleep(0.05)`); err != nil {
				t.Fatalf("waiting to look again: %v", err)
			}
		}
	}
	return overlapped, release
}

// startWorkerWhileHeld starts the runner without waiting for its start to
// return, because a held job can slow the start on a loaded machine. The
// returned function waits for the start once the hold is released.
func startWorkerWhileHeld(t *testing.T, runner *jobs.Runner) (started func()) {
	t.Helper()
	errc := make(chan error, 1)
	go func() { errc <- runner.Start(context.Background()) }()
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := runner.Stop(stopCtx); err != nil {
			t.Errorf("stopping the worker: %v", err)
		}
	})
	return func() {
		t.Helper()
		if err := <-errc; err != nil {
			t.Fatalf("starting the worker: %v", err)
		}
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
			overlapped, release := holdFilingUntilOverlap(t)
			started := startWorkerWhileHeld(t, runner)
			overlapped()
			release()
			started()
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
		`CREATE TRIGGER chat_recovery_fault BEFORE INSERT ON event_outbox FOR EACH ROW WHEN (NEW.envelope->'actor'->>'type' = 'connector') EXECUTE FUNCTION chat_recovery_fault()`,
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
	limit, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	for {
		select {
		case <-limit.Done():
			t.Fatalf("timed out waiting for a %s job to fail", kind)
		case ev := <-failed:
			if ev != nil && ev.Job != nil && ev.Job.Kind == kind {
				return
			}
		}
	}
}

// runnerBookkeeping are the tables the worker role writes on start-up and on its
// own schedule, whatever a test delivers. Measured on a runner that was given no
// message: its rows change while the runner runs, so counting them says nothing
// about a filing. The audit log and the event outbox are counted for the connector
// alone, which is the actor of a filing. The audit log leaves out the
// connection's own rows, which the Zalo token renewal writes on start-up.
// activity_participant_replay is the periodic participant backfill's own mark,
// which lands after a filing or not, as that pass's schedule falls.
var runnerBookkeeping = []string{"activity_participant_replay", "brief_run", "capture_sweep_run", "data_task_run", "installation_state", "system_log"}

// tableCounts counts the rows a filing could write, in every table apart from
// the job queue's own and the runner's bookkeeping.
func (c *telegramEnv) tableCounts(t *testing.T) map[string]int {
	t.Helper()
	counts := map[string]int{}
	err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `SELECT tablename FROM pg_tables WHERE schemaname = 'public' AND tablename NOT LIKE 'river%'`)
		if err != nil {
			return err
		}
		tables, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		for _, name := range tables {
			if slices.Contains(runnerBookkeeping, name) {
				continue
			}
			query := `SELECT count(*) FROM ` + pgx.Identifier{name}.Sanitize()
			switch name {
			case "audit_log":
				query += ` WHERE actor_type = 'connector' AND entity_type <> 'channel_connection'`
			case "event_outbox":
				query += ` WHERE envelope->'actor'->>'type' = 'connector'`
			}
			var n int
			if err := tx.QueryRow(context.Background(), query).Scan(&n); err != nil {
				return err
			}
			counts[name] = n
		}
		return nil
	})
	if err != nil {
		t.Fatalf("counting the rows of every table: %v", err)
	}
	return counts
}

// countDelta is what changed between two counts, by table.
func countDelta(after, before map[string]int) map[string]int {
	delta := map[string]int{}
	for table, n := range after {
		if n != before[table] {
			delta[table] = n - before[table]
		}
	}
	return delta
}

// assertJobsCompleted checks the queue holds want jobs of the kind, all completed.
func (c *telegramEnv) assertJobsCompleted(t *testing.T, kind string, want int) {
	t.Helper()
	var total, done int
	if err := c.Pool.QueryRow(context.Background(),
		`SELECT count(*), count(*) FILTER (WHERE state = 'completed') FROM river_job WHERE kind = $1`, kind).Scan(&total, &done); err != nil {
		t.Fatalf("reading the %s jobs: %v", kind, err)
	}
	if total != want || done != want {
		t.Errorf("%d %s jobs, %d completed, want %d of each", total, kind, done, want)
	}
}

// filingRows is what one run of a stored message added to the tables. Mid is the
// state after a failed first run, nil when the run did not fail. Final is the
// state once the message filed.
type filingRows struct {
	mid, final map[string]int
	audit      []string
}

// connectorAudit lists the connector's audit rows by entity and action.
func (c *telegramEnv) connectorAudit(t *testing.T) []string {
	t.Helper()
	var out []string
	err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `SELECT entity_type || ':' || action FROM audit_log WHERE actor_type = 'connector' AND entity_type <> 'channel_connection' ORDER BY id`)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	if err != nil {
		t.Fatalf("listing the connector's audit rows: %v", err)
	}
	return out
}

// onlyTables keeps the counts of the named tables.
func onlyTables(counts, tables map[string]int) map[string]int {
	out := map[string]int{}
	for table := range tables {
		out[table] = counts[table]
	}
	return out
}

// failingFiling runs one stored message to its filing, with or without a fault
// on its first run.
func failingFiling(t *testing.T, n webhookNetwork, fault bool) filingRows {
	t.Helper()
	c := n.setup(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	runner.ClearPeriodicsForTest()
	failed, cancelFailed := runner.SubscribeFailed()
	t.Cleanup(cancelFailed)
	acct := n.connect(t, c, "8103")
	acct.in(t, "retry-1", "one message")
	before := c.tableCounts(t)
	var rows filingRows
	if fault {
		lift := failEventOutbox(t)
		startTelegramWorker(t, runner)
		awaitJobFailure(t, failed, n.job)
		rows.mid = countDelta(c.tableCounts(t), before)
		lift()
	} else {
		startTelegramWorker(t, runner)
	}
	n.await(t, sub)
	c.assertJobsCompleted(t, n.job, 1)
	rows.final = countDelta(c.tableCounts(t), before)
	rows.audit = c.connectorAudit(t)
	return rows
}

// assertRetryMatchesCleanRun compares a failed run and its retry with a clean
// run. The tables a filing writes are the ones the clean run changed. The failed
// run must have changed none of them, and the retry must have changed each by the
// same count as the clean run.
func assertRetryMatchesCleanRun(t *testing.T, clean, retried filingRows, storedBeforeRun ...string) {
	t.Helper()
	if clean.final["activity"] != 1 {
		t.Fatalf("a clean run added %d activities, want 1 (%v)", clean.final["activity"], clean.final)
	}
	for table, n := range onlyTables(retried.mid, clean.final) {
		if n != 0 && !slices.Contains(storedBeforeRun, table) {
			t.Errorf("a failed run left %d rows in %s, want none", n, table)
		}
	}
	if got := onlyTables(retried.final, clean.final); !maps.Equal(got, clean.final) {
		t.Errorf("a failed run and its retry added %v, but a clean run adds %v\nconnector audit rows, retried: %v, clean: %v",
			got, clean.final, retried.audit, clean.audit)
	}
}

func TestAnIngestThatFailsMidFilingLeavesNothingAndItsRetryFilesOnce(t *testing.T) {
	for _, n := range webhookNetworks() {
		t.Run(n.name, func(t *testing.T) {
			var clean, retried filingRows
			t.Run("clean", func(t *testing.T) { clean = failingFiling(t, n, false) })
			t.Run("faulty", func(t *testing.T) { retried = failingFiling(t, n, true) })
			assertRetryMatchesCleanRun(t, clean, retried)
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

// telegramFiling runs one business message to its filing, with or without a
// fault on the first run.
func telegramFiling(t *testing.T, fault bool) filingRows {
	t.Helper()
	c := setupTelegramConnected(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	runner.ClearPeriodicsForTest()
	failed, cancelFailed := runner.SubscribeFailed()
	t.Cleanup(cancelFailed)
	c.link(t, 880041, "bc-retry", "connected")
	m := businessMessage{updateID: 7601, messageID: 31, chatID: 990007, fromID: 990007, connection: "bc-retry", text: "one message"}
	before := c.tableCounts(t)
	var lift func()
	if fault {
		lift = failEventOutbox(t)
	}
	startTelegramWorker(t, runner)
	c.api.hold(m.body(t))
	c.pollNow(t, sub, m.updateID+1)
	var rows filingRows
	if fault {
		awaitJobFailure(t, failed, compose.TelegramIngestArgs{}.Kind())
		rows.mid = countDelta(c.tableCounts(t), before)
		lift()
	}
	awaitJobKind(t, sub, compose.TelegramIngestArgs{}.Kind())
	c.assertJobsCompleted(t, compose.TelegramIngestArgs{}.Kind(), 1)
	rows.final = countDelta(c.tableCounts(t), before)
	rows.audit = c.connectorAudit(t)
	return rows
}

func TestATelegramUpdateThatFailsMidFilingLeavesNothingAndItsRetryFilesOnce(t *testing.T) {
	var clean, retried filingRows
	t.Run("clean", func(t *testing.T) { clean = telegramFiling(t, false) })
	t.Run("faulty", func(t *testing.T) { retried = telegramFiling(t, true) })
	assertRetryMatchesCleanRun(t, clean, retried, "raw_capture")
}

func TestATelegramUpdateRunByTwoWorkersAtOnceFilesOnce(t *testing.T) {
	c := setupTelegramConnected(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	c.link(t, 880042, "bc-twice", "connected")
	m := businessMessage{updateID: 7602, messageID: 32, chatID: 990008, fromID: 990008, connection: "bc-twice", text: "one message"}
	contactsBefore := c.contacts(t)
	overlapped, release := holdFilingUntilOverlap(t)
	startTelegramWorker(t, runner)

	c.api.hold(m.body(t))
	c.pollNow(t, sub, m.updateID+1)
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
	overlapped()
	release()
	for range copies + 1 {
		awaitJobKind(t, sub, compose.TelegramIngestArgs{}.Kind())
	}

	filed, ok := c.filed(t, m.sourceID(880042))
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
