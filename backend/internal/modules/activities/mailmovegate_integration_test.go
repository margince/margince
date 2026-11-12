// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// mailThreads seeds the reader's mail to one contact, each on its own thread,
// with the replies a colleague filed.
type mailThreads struct {
	e       *loadEnv
	store   *Store
	contact ids.UUID
}

// mail seeds one email on thread, linked to the contact, after the given
// offset. Outbound mail names the reader as its sender.
func (m *mailThreads) mail(t *testing.T, direction, thread string, after time.Duration) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	m.e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key, source, captured_by)
		VALUES ($1, 'email', $2, 'the proposal', $3, $4, 'seed', 'human:' || $5::text)`,
		id, direction, requestInstant.Add(after), thread, m.e.other)
	if direction == "outbound" {
		m.e.exec(t, `INSERT INTO activity_participant (id, activity_id, role, user_id)
			VALUES ($1, $2, 'from', $3)`, ids.NewV7(), id, m.e.rep)
	}
	m.e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, contact_id)
		VALUES ($1, $2, 'contact', $3)`, ids.NewV7(), id, m.contact)
	return id
}

// limit hands the activity's content to the colleague alone, through the real
// writer and as its author.
func (m *mailThreads) limit(t *testing.T, activity ids.UUID) {
	t.Helper()
	if _, err := m.store.SetAudience(m.e.asSeat(m.e.other), ids.From[ids.ActivityKind](activity), SetAudienceInput{
		Audience: "selected",
		Members:  []AudienceMember{{SubjectType: "user", SubjectID: m.e.other}},
	}); err != nil {
		t.Fatalf("limiting the activity to a colleague: %v", err)
	}
}

// A reply the reader may not read leaves the reader waiting for them. A reply
// they may read ends the wait. A mail they may not read has no move.
func TestAMailsMoveReadsOnlyTheThreadTheReaderMayRead(t *testing.T) {
	e := setupLoad(t)
	m := &mailThreads{e: e, store: NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws))), contact: e.buyer(t)}
	thread := func() string { return "move-" + ids.NewV7().String() }

	answeredThread, hiddenReplyThread := thread(), thread()
	answered := m.mail(t, "outbound", answeredThread, 0)
	m.mail(t, "inbound", answeredThread, time.Hour)
	repliedOutOfSight := m.mail(t, "outbound", hiddenReplyThread, 0)
	m.limit(t, m.mail(t, "inbound", hiddenReplyThread, time.Hour))
	// The reader is a participant of every mail they sent, so the mail they may
	// not read is one they received.
	unreadable := m.mail(t, "inbound", thread(), 0)
	m.limit(t, unreadable)

	var moves map[ids.UUID]crmcontracts.EmailSummaryMove
	reader := e.asSeat(e.rep)
	if err := database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)).Tx(reader, func(tx pgx.Tx) error {
		var err error
		moves, err = MailMovesFor(reader, tx, []ids.UUID{answered, repliedOutOfSight, unreadable}, requestInstant.Add(24*time.Hour))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct {
		name string
		id   ids.UUID
		move crmcontracts.EmailSummaryMove
	}{
		{"the answered mail", answered, crmcontracts.EmailSummaryMoveNone},
		{"the mail whose reply is limited", repliedOutOfSight, crmcontracts.EmailSummaryMoveWaitingForThem},
	} {
		if got, ok := moves[want.id]; !ok || got != want.move {
			t.Errorf("%s: move %q (has one %v), want %q", want.name, got, ok, want.move)
		}
	}
	if _, ok := moves[unreadable]; ok {
		t.Error("a mail the reader may not read has a move")
	}
}
