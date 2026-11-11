// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// edgeEnsurer runs the real ensure, so its contact, link and audit row are
// written in the capture's transaction, and hands back a hook it watches. fail
// then fails as a resolver fault does; cancel ends the capture's context, so
// the capture transaction rolls back after the ensure succeeded.
type edgeEnsurer struct {
	delegate capture.CounterpartyEnsurer
	fail     bool
	cancel   context.CancelFunc
	created  ids.UUID
	hookRan  atomic.Bool
}

func (f *edgeEnsurer) EnsureCounterpartyTx(ctx context.Context, tx pgx.Tx, in capture.EnsureRequest) (capture.EnsureOutcome, error) {
	outcome, err := f.delegate.EnsureCounterpartyTx(ctx, tx, in)
	if err != nil {
		return outcome, err
	}
	f.created = outcome.ContactID
	outcome.AfterCommit = func(context.Context) { f.hookRan.Store(true) }
	if f.cancel != nil {
		f.cancel()
	}
	if f.fail {
		return outcome, errors.New("the resolver failed after its writes")
	}
	return outcome, nil
}

func realEnsurer(t *testing.T, e *integration.Env) contactsEnsurer {
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	return contactsEnsurer{
		store: newCounterpartyStoreAt(e.Pool, func() time.Time { return clocktest.Now(t) }), pool: e.Pool,
		triage: newDomainTriageTrigger(e.Pool, quiet, true), log: quiet,
	}
}

// ensureMail is one message; sent marks the provider attesting the seat sent it.
func ensureMail(t *testing.T, owner, from, to, cc, msgID, refs string, sent bool) connector.NormalizedRecord {
	t.Helper()
	lines := []string{
		"From: " + from, "To: " + to, "Subject: Supply terms",
		"Date: Mon, 5 Oct 2026 10:00:00 +0000", "Message-ID: <" + msgID + ">", "Content-Type: text/plain; charset=utf-8",
	}
	if cc != "" {
		lines = append(lines, "Cc: "+cc)
	}
	if refs != "" {
		lines = append(lines, "References: <"+refs+">")
	}
	raw := []byte(strings.Join(append(lines, "", "Could we agree the supply terms this week?", ""), "\r\n"))
	parsed, err := mailmap.Parse(raw, owner)
	if err != nil {
		t.Fatal(err)
	}
	return parsed.AttestSentByOwner(sent).ToRecord("gmail", raw)
}

// captureWithReply captures an inbound mail from address and returns the seat's
// attested reply, which makes the address a counterparty the ensure creates.
func captureWithReply(t *testing.T, e *integration.Env, sink *capture.Sink, address string) connector.NormalizedRecord {
	t.Helper()
	e.GrantRole(t, e.AdminUser, "admin")
	owner := seatAddress(t, e, e.AdminUser)
	first := ids.NewV7().String() + "@supplier.test"
	if _, err := sink.Upsert(threadConnectorCtx(e, e.AdminUser), ensureMail(t, owner, address, owner, "", first, "", false)); err != nil {
		t.Fatal(err)
	}
	return ensureMail(t, owner, owner, address, "", ids.NewV7().String()+"@ws.example", first, true)
}

func TestAnEnsureFaultRollsBackItsWritesAndKeepsTheMail(t *testing.T) {
	e := integration.Setup(t)
	faulty := &edgeEnsurer{delegate: realEnsurer(t, e), fail: true}
	sink := newCaptureSink(e.Pool, CaptureConfig{}).WithEnsurer(faulty, capture.NewTransactionalList(nil, nil))
	reply := captureWithReply(t, e, sink, "fault-buyer@supplier.test")

	if _, err := sink.Upsert(threadConnectorCtx(e, e.AdminUser), reply); err != nil {
		t.Fatal(err)
	}
	if faulty.created.IsZero() {
		t.Fatal("the fixture's reply created no contact, so nothing was written to roll back")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE source_id = $1`, reply.NaturalKey.SourceID); n != 1 {
		t.Fatal("the mail rolled back with the ensure that failed")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM contact WHERE id = $1`, faulty.created); n != 0 {
		t.Error("the contact the failed ensure wrote was committed")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1`, faulty.created); n != 0 {
		t.Error("the audit row of the failed ensure was committed")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity_link l JOIN activity a ON a.id = l.activity_id
		WHERE a.source_id = $1`, reply.NaturalKey.SourceID); n != 0 {
		t.Error("the link the failed ensure wrote was committed")
	}
	if faulty.hookRan.Load() {
		t.Error("the after-commit work of a failed ensure ran")
	}
}

func TestAnEnsuresAfterCommitWorkWaitsForTheCaptureToCommit(t *testing.T) {
	e := integration.Setup(t)
	reply := captureWithReply(t, e, newCaptureSink(e.Pool, CaptureConfig{}), "rollback-buyer@supplier.test")
	ctx, cancel := context.WithCancel(threadConnectorCtx(e, e.AdminUser))
	defer cancel()
	ensurer := &edgeEnsurer{delegate: realEnsurer(t, e), cancel: cancel}
	sink := newCaptureSink(e.Pool, CaptureConfig{}).WithEnsurer(ensurer, capture.NewTransactionalList(nil, nil))

	if _, err := sink.Upsert(ctx, reply); err == nil {
		t.Fatal("the capture committed after its context ended")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE source_id = $1`, reply.NaturalKey.SourceID); n != 0 {
		t.Fatal("the capture transaction committed, so this proves nothing about a rollback")
	}
	if ensurer.hookRan.Load() {
		t.Error("the after-commit work ran for a capture that rolled back")
	}
}

// meetAtTheEnsure holds the first capture at its ensure until the second gets
// there too or waits on a lock. A lock-cycle retry shows as a third call.
type meetAtTheEnsure struct {
	delegate capture.CounterpartyEnsurer
	pool     *pgxpool.Pool
	mu       sync.Mutex
	calls    int
	both     chan struct{}
}

func (m *meetAtTheEnsure) EnsureCounterpartyTx(ctx context.Context, tx pgx.Tx, in capture.EnsureRequest) (capture.EnsureOutcome, error) {
	m.mu.Lock()
	m.calls++
	call := m.calls
	if call == 2 {
		close(m.both)
	}
	m.mu.Unlock()
	for waiting := false; call == 1 && !waiting; {
		select {
		case <-m.both:
			waiting = true
		default:
			if err := m.pool.QueryRow(context.Background(), `SELECT EXISTS (SELECT 1 FROM pg_stat_activity
				WHERE datname = current_database() AND wait_event_type = 'Lock')`).Scan(&waiting); err != nil {
				return capture.EnsureOutcome{}, err
			}
		}
	}
	return m.delegate.EnsureCounterpartyTx(ctx, tx, in)
}

// Two seats' mails name one pair in swapped roles: to A with B copied, and to
// B with A copied. Each files the copied contact through the participant
// filing and the addressed one through the ensure. Both lock the pair in one
// order first, so neither waits on the other in a cycle. Two seats, because
// one seat's captures queue behind each other.
func TestMailsNamingOnePairInSwappedRolesCommitWithoutALockCycle(t *testing.T) {
	e := integration.Setup(t)
	const a, b = "pair-a@supplier.test", "pair-b@supplier.test"
	for _, address := range []string{a, b} {
		if _, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
			FullName: address, Source: "manual",
			Emails: []contacts.ContactEmailInput{{Email: address, EmailType: "work", IsPrimary: true}},
		}); err != nil {
			t.Fatal(err)
		}
		captureWithReply(t, e, newCaptureSink(e.Pool, CaptureConfig{}), address)
	}
	judgeAsCustomers(t, e, 2)
	meet := &meetAtTheEnsure{delegate: realEnsurer(t, e), pool: e.Pool, both: make(chan struct{})}
	sink := newCaptureSink(e.Pool, CaptureConfig{}).WithEnsurer(meet, capture.NewTransactionalList(nil, nil))
	seats, to, copied := []ids.UUID{e.AdminUser, e.Rep1}, []string{a, b}, []string{b, a}
	pair := make([]connector.NormalizedRecord, len(seats))
	for i, seat := range seats {
		e.GrantRole(t, seat, "admin")
		owner := seatAddress(t, e, seat)
		pair[i] = ensureMail(t, owner, owner, to[i], copied[i], ids.NewV7().String()+"@ws.example", "", true)
	}

	errs := make([]error, len(pair))
	var wg sync.WaitGroup
	for i, record := range pair {
		wg.Go(func() { _, errs[i] = sink.Upsert(threadConnectorCtx(e, seats[i]), record) })
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		t.Fatalf("capturing the pair: %v", err)
	}
	if meet.calls != 2 {
		t.Errorf("the ensure ran %d times for two mails: a lock cycle aborted one and it ran again", meet.calls)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity_link l JOIN activity act ON act.id = l.activity_id
		JOIN contact_email ce ON ce.contact_id = l.contact_id WHERE act.source_id = ANY($1) AND ce.email IN ($2, $3)`,
		[]string{pair[0].NaturalKey.SourceID, pair[1].NaturalKey.SourceID}, a, b); n != 4 {
		t.Errorf("%d links from the pair's mails to the pair, want 4: each mail under both", n)
	}
}

// judgeAsCustomers settles the open sender questions as the verdict engine does.
// A judged sender's mail takes no deferral lock, which would queue the pair.
func judgeAsCustomers(t *testing.T, e *integration.Env, want int) {
	t.Helper()
	ledger := capture.NewPendingStore(InstallationDB(e.Pool))
	due, err := ledger.ClaimDue(e.Admin(), want+1)
	if err != nil || len(due) != want {
		t.Fatalf("claimed %d questions (err %v), want %d", len(due), err, want)
	}
	for _, question := range due {
		if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			_, err := ledger.Resolve(e.Admin(), tx, question, capture.PendingStatusReal, "fixture")
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
}
