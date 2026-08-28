// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// How much of a conversation the extraction lane reads, and what it says about
// the rest.
//
// The cut is made in SQL, so a body of exactly extractBodyLimit characters and
// one truncated to that length arrive identically — the reading is drawn from
// the head either way, and downstream nothing can tell them apart. The
// remainder is what tells them apart, and it is measured in the same statement
// that cuts, because asked for afterwards it would describe a row that may have
// changed.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedThreadMail plants one email on a thread with a body of the given length.
func seedThreadMail(t *testing.T, e *integration.Env, threadKey string, bodyRunes, minutesAgo int) {
	t.Helper()
	id := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, subject, body, direction, thread_key, occurred_at,
			                      source_system, source_id, source, captured_by)
			VALUES ($1, 'email', 'a subject', $2, 'inbound', $3,
			        now() - make_interval(mins => $4),
			        'gmail', $5, 'gmail:seed', 'connector:gmail')`,
			id, strings.Repeat("x", bodyRunes), threadKey, minutesAgo, id.String())
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAThreadReportsHowMuchOfItselfWasNotRead(t *testing.T) {
	e := integration.Setup(t)
	key := "thread-" + ids.NewV7().String()

	// One mail the limit holds whole, and one past it by a known amount. Both,
	// because the finding is the DIFFERENCE: a fixture of only-long mails would
	// pass against an implementation that reported every body as truncated.
	over := 400
	seedThreadMail(t, e, key, extractBodyLimit, 2)
	seedThreadMail(t, e, key, extractBodyLimit+over, 1)

	var messages []threadMessage
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		messages, err = threadMessages(context.Background(), tx, &settledThread{Key: key})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if len(messages) != 2 {
		t.Fatalf("the thread read %d messages, want 2", len(messages))
	}
	for _, message := range messages {
		if got := len([]rune(message.Body)); got > extractBodyLimit {
			t.Errorf("a body reached the prompt at %d runes, past the %d limit", got, extractBodyLimit)
		}
	}
	// Oldest first, so the whole one comes back first.
	if messages[0].UnreadRunes != 0 {
		t.Errorf("a mail the limit holds whole reported %d runes unread — a warning that fires on "+
			"every ordinary mail is one an operator learns to ignore", messages[0].UnreadRunes)
	}
	if messages[1].UnreadRunes != over {
		t.Errorf("the truncated mail reported %d runes unread, want %d — the limit alone cannot say "+
			"what was lost, because a body one rune over and one far over arrive identically",
			messages[1].UnreadRunes, over)
	}
}

// TestABackfillIsReadRatherThanMarkedRead is the defect this closes.
//
// The read always took the newest six messages. A backfill changes the message
// count, so the thread becomes due again — the same newest six are re-read, and
// the new count is recorded. The inserted older messages never reach the model
// and the thread now looks read: the state says handled, nothing errored, and
// nothing says content was skipped.
func TestABackfillIsReadRatherThanMarkedRead(t *testing.T) {
	e := integration.Setup(t)
	key := "backfill-" + ids.NewV7().String()

	// A thread longer than one window, so the newest six do not cover it.
	base := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	for i := range 8 {
		seedThreadMailAt(t, e, key, base.Add(time.Duration(i)*time.Hour))
	}

	// The first read: the newest window, and the cursor it leaves behind.
	first := settledThread{Key: key, Newest: base.Add(7 * time.Hour), Count: 8}
	read(t, e, &first)
	if len(first.Messages) != extractThreadMessages {
		t.Fatalf("the first read took %d messages, want the window of %d",
			len(first.Messages), extractThreadMessages)
	}
	if first.ReadFromNow == nil {
		t.Fatal("the first read recorded no cursor, so nothing tells the next one where it started")
	}
	firstOldest := *first.ReadFromNow

	// Now a backfill: three messages older than anything the thread held.
	for i := range 3 {
		seedThreadMailAt(t, e, key, base.Add(-time.Duration(i+1)*time.Hour))
	}

	// The thread is due again — the count moved — and nothing is NEWER than
	// what was read, so the window must walk backwards rather than re-reading
	// the same six.
	second := settledThread{
		Key: key, Newest: base.Add(7 * time.Hour), Count: 11,
		ReadTo: timePtr(base.Add(7 * time.Hour)), ReadFrom: &firstOldest,
	}
	read(t, e, &second)
	if len(second.Messages) == 0 {
		t.Fatal("the second read took nothing, so a backfilled message is never sent to the model")
	}
	for _, message := range second.Messages {
		if !message.At.Before(firstOldest) {
			t.Errorf("the second read took a message at %s, which the first read had already covered "+
				"(it started at %s) — the window did not move", message.At, firstOldest)
		}
	}
	// And it reached the backfill specifically, not merely something older.
	if oldest := second.Messages[0].At; !oldest.Before(base) {
		t.Errorf("the oldest message read was %s, and the backfill sits before %s — the window walked "+
			"back but not far enough to reach it", oldest, base)
	}
}

// New mail outranks a backfill: a reply nobody has read is why the conversation
// is interesting now, and a thread walking backwards through its history while
// that sits there would be reading its least useful end.
func TestNewMailIsReadBeforeTheOlderRange(t *testing.T) {
	e := integration.Setup(t)
	key := "newmail-" + ids.NewV7().String()

	base := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	for i := range 8 {
		seedThreadMailAt(t, e, key, base.Add(time.Duration(i)*time.Hour))
	}
	first := settledThread{Key: key, Newest: base.Add(7 * time.Hour), Count: 8}
	read(t, e, &first)
	firstOldest := *first.ReadFromNow

	// A reply arrives, and older messages remain unread below the cursor.
	reply := base.Add(20 * time.Hour)
	seedThreadMailAt(t, e, key, reply)

	second := settledThread{
		Key: key, Newest: reply, Count: 9,
		ReadTo: timePtr(base.Add(7 * time.Hour)), ReadFrom: &firstOldest,
	}
	read(t, e, &second)
	newest := second.Messages[len(second.Messages)-1].At
	if !newest.Equal(reply) {
		t.Errorf("the window ends at %s, want the new message at %s — a thread with unread mail must "+
			"read that before walking back through its history", newest, reply)
	}
}

func timePtr(at time.Time) *time.Time { return &at }

// seedThreadMailAt places one message on a thread at an exact instant, which is
// what a window test needs: the ordering IS the subject, and "minutes ago"
// cannot express a backfill below everything already there.
func seedThreadMailAt(t *testing.T, e *integration.Env, threadKey string, at time.Time) {
	t.Helper()
	id := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, subject, body, direction, thread_key, occurred_at,
			                      source_system, source_id, source, captured_by)
			VALUES ($1, 'email', 'a subject', 'a body', 'inbound', $2, $3,
			        'gmail', $4, 'gmail:seed', 'connector:gmail')`,
			id, threadKey, at, id.String())
		return err
	}); err != nil {
		t.Fatalf("seeding a message at %s: %v", at, err)
	}
}

// read runs one window read in a transaction, filling the thread's messages and
// its cursor the way the pass does.
func read(t *testing.T, e *integration.Env, thread *settledThread) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		messages, err := threadMessages(context.Background(), tx, thread)
		thread.Messages = messages
		return err
	}); err != nil {
		t.Fatalf("reading the conversation: %v", err)
	}
}
