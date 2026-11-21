// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// mailAt writes one message on thread, at the instant given, between us and buyer.
func (e *loadEnv) mailAt(t *testing.T, thread, direction, buyer string, at time.Time) ids.UUID {
	t.Helper()
	activity := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key, counterparty_email, source, captured_by)
		VALUES ($1, 'email', $2, 'the order', $3, $4, $5, 'manual', 'system')`,
		activity, direction, at, thread, buyer)
	return activity
}

// owedAt answers, per message, whether a reply is owed on it at each instant.
func (e *loadEnv) owedAt(t *testing.T, messages []ids.UUID, instants []time.Time) map[ids.UUID][]bool {
	t.Helper()
	owed := map[ids.UUID][]bool{}
	for _, at := range instants {
		rows, err := e.owner.Query(context.Background(), `SELECT a.id, coalesce(`+
			owedSQL("$1::timestamptz", "$3", neverRelaxed, neverRelaxed, neverRelaxed)+`, false)
			FROM activity a WHERE a.id = ANY($2)`, at, messages, []string{})
		if err != nil {
			t.Fatalf("asking what is owed: %v", err)
		}
		for rows.Next() {
			var id ids.UUID
			var isOwed bool
			if err := rows.Scan(&id, &isOwed); err != nil {
				t.Fatalf("reading what is owed: %v", err)
			}
			owed[id] = append(owed[id], isOwed)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("reading what is owed: %v", err)
		}
	}
	return owed
}

func (e *loadEnv) stampSettled(t *testing.T) {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:reply-settled",
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"activity": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	for {
		n, err := store.StampReplySettledBatch(ctx, 50)
		if err != nil {
			t.Fatalf("stamping: %v", err)
		}
		if n == 0 {
			return
		}
	}
}

func (e *loadEnv) stampOf(t *testing.T, message ids.UUID) (stamp *time.Time, version int64) {
	t.Helper()
	if err := e.owner.QueryRow(context.Background(),
		`SELECT CASE WHEN reply_settled_at = 'infinity' THEN NULL ELSE reply_settled_at END, version
		   FROM activity WHERE id = $1`, message).Scan(&stamp, &version); err != nil {
		t.Fatalf("reading the stamp: %v", err)
	}
	return stamp, version
}

// The stamp only lets the owed check stop early. At every instant, a message is
// owed when and only when it was before the stamp existed.
func TestTheSettledStampNeverChangesWhatIsOwed(t *testing.T) {
	e := setupLoad(t)
	buyer := "buyer-" + ids.NewV7().String() + "@customer.test"
	now := clocktest.Now(t)
	answered := "thread-" + ids.NewV7().String()
	asked := e.mailAt(t, answered, "inbound", buyer, now.Add(-5*time.Hour))
	reply := e.mailAt(t, answered, "outbound", buyer, now.Add(-3*time.Hour))
	followed := "thread-" + ids.NewV7().String()
	first := e.mailAt(t, followed, "inbound", buyer, now.Add(-4*time.Hour))
	second := e.mailAt(t, followed, "inbound", buyer, now.Add(-2*time.Hour))
	silent := e.mailAt(t, "thread-"+ids.NewV7().String(), "inbound", buyer, now.Add(-time.Hour))
	messages := []ids.UUID{asked, first, second, silent}
	instants := []time.Time{now.Add(-4 * time.Hour), now.Add(-150 * time.Minute), now.Add(-90 * time.Minute), now}

	before := e.owedAt(t, messages, instants)
	if !before[asked][0] || before[asked][3] || !before[silent][3] || !before[first][0] || before[first][3] {
		t.Fatalf("the fixture does not exercise the check: owed %v", before)
	}
	_, versionBefore := e.stampOf(t, asked)
	e.stampSettled(t)
	after := e.owedAt(t, messages, instants)
	for _, message := range messages {
		for i := range instants {
			if before[message][i] != after[message][i] {
				t.Errorf("message %s at instant %d: owed %v before the stamp, %v after", message, i, before[message][i], after[message][i])
			}
		}
	}
	if stamp, version := e.stampOf(t, asked); stamp == nil || version != versionBefore {
		t.Errorf("the answered message has stamp %v at version %d, want our reply's time at version %d", stamp, version, versionBefore)
	}
	if stamp, _ := e.stampOf(t, first); stamp == nil {
		t.Error("a message the buyer followed up on carries no stamp")
	}
	if stamp, _ := e.stampOf(t, silent); stamp != nil {
		t.Errorf("a message nobody answered is stamped settled at %v", stamp)
	}

	e.exec(t, `UPDATE activity SET archived_at = $2 WHERE id = $1`, reply, now)
	var cleared bool
	if err := e.owner.QueryRow(context.Background(),
		`SELECT reply_settled_at IS NULL FROM activity WHERE id = $1`, asked).Scan(&cleared); err != nil || !cleared {
		t.Fatalf("archiving the reply left the answered message's stamp in place (cleared %v, err %v)", cleared, err)
	}
	e.stampSettled(t)
	if owed := e.owedAt(t, []ids.UUID{asked}, []time.Time{now}); !owed[asked][0] {
		t.Error("with its only reply archived, the message is still read as settled")
	}

	e.exec(t, `DELETE FROM activity WHERE id = $1`, second)
	if stamp, _ := e.stampOf(t, first); stamp != nil {
		t.Errorf("deleting the follow-up left the first message settled at %v", stamp)
	}
}

// A sync writes many messages in one statement. Each thread is cleared from the
// earliest message written on it, and a thread whose messages all came later
// keeps its stamp.
func TestOneStatementClearsEachThreadFromItsOwnEarliestMessage(t *testing.T) {
	e := setupLoad(t)
	buyer := "buyer-" + ids.NewV7().String() + "@customer.test"
	now := clocktest.Now(t)
	reopened, kept := "thread-"+ids.NewV7().String(), "thread-"+ids.NewV7().String()
	reopenedAsk := e.mailAt(t, reopened, "inbound", buyer, now.Add(-5*time.Hour))
	e.mailAt(t, reopened, "outbound", buyer, now.Add(-3*time.Hour))
	keptAsk := e.mailAt(t, kept, "inbound", buyer, now.Add(-5*time.Hour))
	e.mailAt(t, kept, "outbound", buyer, now.Add(-3*time.Hour))
	e.stampSettled(t)

	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key, counterparty_email, source, captured_by)
		VALUES ($1, 'email', 'outbound', 'the order', $4, $6, $7, 'manual', 'system'),
		       ($2, 'email', 'inbound', 'the order', $5, $6, $7, 'manual', 'system'),
		       ($3, 'email', 'outbound', 'the order', $4, $8, $7, 'manual', 'system')`,
		ids.NewV7(), ids.NewV7(), ids.NewV7(), now.Add(-time.Hour), now.Add(-4*time.Hour), reopened, buyer, kept)

	if stamp, _ := e.stampOf(t, reopenedAsk); stamp != nil {
		t.Errorf("a message written before the thread's settling reply left the stamp at %v", stamp)
	}
	if stamp, _ := e.stampOf(t, keptAsk); stamp == nil {
		t.Error("a message written after the settling reply cleared the stamp of a thread it cannot unsettle")
	}
}

// settledThread is one thread with an ask our reply settled, and a later ask
// still open. The stamp of the first is our reply's time, the second's infinity.
type settledThread struct {
	key, buyer             string
	sender                 ids.UUID
	answered, reply, later ids.UUID
	now                    time.Time
}

func (e *loadEnv) settledThread(t *testing.T) settledThread {
	t.Helper()
	s := settledThread{key: "thread-" + ids.NewV7().String(), buyer: "buyer-" + ids.NewV7().String() + "@customer.test",
		sender: ids.NewV7(), now: clocktest.Now(t)}
	s.answered = e.mailAt(t, s.key, "inbound", s.buyer, s.now.Add(-6*time.Hour))
	s.reply = e.mailAt(t, s.key, "outbound", s.buyer, s.now.Add(-5*time.Hour))
	s.later = e.mailAt(t, s.key, "inbound", s.buyer, s.now.Add(-2*time.Hour))
	e.exec(t, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Buyer', 'manual', 'system')`, s.sender)
	e.exec(t, `INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by)
		VALUES ($1, $2, true, 'manual', 'system')`, s.sender, s.buyer)
	e.exec(t, `INSERT INTO activity_participant (activity_id, contact_id, address, role) VALUES ($1, $2, $3, 'from')`,
		s.answered, s.sender, s.buyer)
	e.exec(t, `INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, $2, 'to')`, s.reply, s.buyer)
	e.stampSettled(t)
	if !e.stamped(t, s.answered) || !e.stamped(t, s.later) {
		t.Fatal("the fixture left a message of the thread unstamped")
	}
	return s
}

// stamped says the message carries a stamp, settled or checked open.
func (e *loadEnv) stamped(t *testing.T, message ids.UUID) bool {
	t.Helper()
	var set bool
	if err := e.owner.QueryRow(context.Background(),
		`SELECT reply_settled_at IS NOT NULL FROM activity WHERE id = $1`, message).Scan(&set); err != nil {
		t.Fatalf("reading the stamp: %v", err)
	}
	return set
}

// A participant of a message at t can move only stamps at or after t. A new
// reply's recipients keep the older settled stamp; a recipient added to the old
// ask itself clears it.
func TestAParticipantClearsOnlyTheStampsFromItsOwnMessageOn(t *testing.T) {
	e := setupLoad(t)
	s := e.settledThread(t)

	newer := e.mailAt(t, s.key, "outbound", s.buyer, s.now.Add(-time.Hour))
	e.exec(t, `INSERT INTO activity_participant (activity_id, address, role)
		VALUES ($1, $2, 'to'), ($1, 'colleague@customer.test', 'cc')`, newer, s.buyer)
	if !e.stamped(t, s.answered) {
		t.Error("the recipients of a newer reply cleared the stamp of an ask settled before it")
	}
	if e.stamped(t, s.later) {
		t.Error("the recipients of a reply to the open ask left its stamp in place")
	}

	e.stampSettled(t)
	e.exec(t, `INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, 'colleague@customer.test', 'cc')`, s.answered)
	if e.stamped(t, s.answered) {
		t.Error("a participant added to the settled ask left its stamp in place")
	}

	e.stampSettled(t)
	e.exec(t, `DELETE FROM activity_participant WHERE activity_id = $1 AND role = 'to'`, s.reply)
	if e.stamped(t, s.answered) {
		t.Error("removing the recipient of the settling reply left the ask settled")
	}
}

// Columns the owed check never reads move no stamp; each column it reads does.
func TestOnlyAColumnTheOwedCheckReadsClearsTheStamp(t *testing.T) {
	e := setupLoad(t)
	s := e.settledThread(t)
	other := ids.NewV7()
	e.exec(t, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Other', 'manual', 'system')`, other)

	e.exec(t, `UPDATE activity_participant SET response = 'accepted', display_name = 'Buyer' WHERE activity_id = $1`, s.answered)
	e.exec(t, `UPDATE contact_email SET is_primary = false, email_type = 'personal' WHERE contact_id = $1`, s.sender)
	if !e.stamped(t, s.answered) || !e.stamped(t, s.later) {
		t.Error("a write of a column the owed check never reads cleared a stamp")
	}

	reads := []struct{ column, sql string }{
		{"activity_participant.role", `UPDATE activity_participant SET role = 'cc' WHERE activity_id = $1 AND role = 'from'`},
		{"activity_participant.role back", `UPDATE activity_participant SET role = 'from' WHERE activity_id = $1 AND role = 'cc'`},
		{"activity_participant.address", `UPDATE activity_participant SET address = 'Buyer@customer.test' WHERE activity_id = $1`},
		{"activity_participant.contact_id", `UPDATE activity_participant SET contact_id = NULL WHERE activity_id = $1`},
		{"activity_participant.contact_id back", `UPDATE activity_participant SET contact_id = '` + s.sender.String() + `' WHERE activity_id = $1`},
		{"activity_participant.activity_id", `UPDATE activity_participant SET activity_id = '` + s.later.String() + `' WHERE activity_id = $1`},
		{"activity_participant.activity_id back", `UPDATE activity_participant SET activity_id = $1 WHERE activity_id = '` + s.later.String() + `'`},
	}
	for _, read := range reads {
		e.stampSettled(t)
		e.exec(t, read.sql, s.answered)
		if e.stamped(t, s.answered) {
			t.Errorf("a change of %s left the ask's stamp in place", read.column)
		}
	}

	senderReads := []struct{ column, sql string }{
		{"contact_email.email", `UPDATE contact_email SET email = 'renamed-' || email WHERE contact_id = $1`},
		{"contact_email.archived_at", `UPDATE contact_email SET archived_at = now() WHERE contact_id = $1`},
		{"contact_email.contact_id", `UPDATE contact_email SET contact_id = '` + other.String() + `' WHERE contact_id = $1`},
		{"a new contact_email", `INSERT INTO contact_email (contact_id, email, source, captured_by) VALUES ($1, 'second@customer.test', 'manual', 'system')`},
		{"a deleted contact_email", `DELETE FROM contact_email WHERE contact_id = $1`},
	}
	for _, read := range senderReads {
		e.stampSettled(t)
		e.exec(t, read.sql, s.sender)
		if e.stamped(t, s.answered) {
			t.Errorf("a change of %s left the stamp of the sender's ask in place", read.column)
		}
	}
}
