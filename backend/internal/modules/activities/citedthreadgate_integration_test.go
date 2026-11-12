// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// citedThreads is a set of cited messages: each kind of evidence once in sight
// and once limited to a colleague. A verdict that skips the content gate reads
// the limited row.
type citedThreads struct {
	e     *loadEnv
	store *Store
	at    time.Time
}

// mail seeds one email a colleague filed, on its own thread unless one is given.
func (c *citedThreads) mail(t *testing.T, direction, thread, counterparty string, after time.Duration) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	c.e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key,
	                                  counterparty_email, counterparty_outbound_attested, source, captured_by)
		VALUES ($1, 'email', $2, 'the quote', $3, $4, NULLIF($5, ''), $2 = 'outbound', 'seed', 'human:' || $6::text)`,
		id, direction, c.at.Add(after), thread, counterparty, c.e.other)
	return id
}

// promise seeds our mail to the contact that opens a promise thread.
func (c *citedThreads) promise(t *testing.T, thread string, contact ids.UUID, address string) ids.UUID {
	t.Helper()
	id := c.mail(t, "outbound", thread, address, 0)
	c.e.exec(t, `INSERT INTO activity_participant (id, activity_id, role, contact_id, address)
		VALUES ($1, $2, 'to', $3, $4)`, ids.NewV7(), id, contact, address)
	return id
}

// limit hands the activity's content to the colleague alone, through the real
// writer and as its author.
func (c *citedThreads) limit(t *testing.T, activity ids.UUID) {
	t.Helper()
	if _, err := c.store.SetAudience(c.e.asSeat(c.e.other), ids.From[ids.ActivityKind](activity), SetAudienceInput{
		Audience: "selected",
		Members:  []AudienceMember{{SubjectType: "user", SubjectID: c.e.other}},
	}); err != nil {
		t.Fatalf("limiting the activity to a colleague: %v", err)
	}
}

// An ask settles on an answer and a promise on a follow-up only when the
// reader may read that evidence. A cited message the reader may not read has
// no verdict at all.
func TestACitedThreadSettlesOnlyOnEvidenceTheReaderMayRead(t *testing.T) {
	e := setupLoad(t)
	c := &citedThreads{
		e: e, store: NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws))), at: requestInstant,
	}
	thread := func() string { return "cited-" + ids.NewV7().String() }

	answeredThread, hiddenAnswerThread := thread(), thread()
	answered := c.mail(t, "inbound", answeredThread, "", 0)
	c.mail(t, "outbound", answeredThread, "", time.Hour)
	answeredOutOfSight := c.mail(t, "inbound", hiddenAnswerThread, "", 0)
	c.limit(t, c.mail(t, "outbound", hiddenAnswerThread, "", time.Hour))
	unreadable := c.mail(t, "inbound", thread(), "", 0)
	c.limit(t, unreadable)

	contact, address := e.mailContact(t, "buyer", "workspace")
	keptThread, hiddenKeptThread := thread(), thread()
	kept := c.promise(t, keptThread, contact, address)
	c.mail(t, "outbound", keptThread, address, time.Hour)
	keptOutOfSight := c.promise(t, hiddenKeptThread, contact, address)
	c.limit(t, c.mail(t, "outbound", hiddenKeptThread, address, time.Hour))

	var settled map[ids.UUID]bool
	reader := e.asSeat(e.rep)
	if err := database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)).Tx(reader, func(tx pgx.Tx) error {
		var err error
		settled, err = SettledCitations(reader, tx,
			[]ids.UUID{answered, answeredOutOfSight, unreadable, kept, keptOutOfSight},
			[]ids.ContactID{ids.From[ids.ContactKind](contact)}, c.at.Add(2*time.Hour))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct {
		name    string
		id      ids.UUID
		settled bool
	}{
		{"the answered ask", answered, true},
		{"the ask whose answer is limited", answeredOutOfSight, false},
		{"the kept promise", kept, true},
		{"the promise whose follow-up is limited", keptOutOfSight, false},
	} {
		if got, ok := settled[want.id]; !ok || got != want.settled {
			t.Errorf("%s: settled %v (has a verdict %v), want %v", want.name, got, ok, want.settled)
		}
	}
	if _, ok := settled[unreadable]; ok {
		t.Error("a cited message the reader may not read has a verdict")
	}
}
