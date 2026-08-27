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
		messages, err = threadMessages(context.Background(), tx, key)
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
