// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The window read's own audience clause, asserted against the statement rather
// than through the pass.
//
// It cannot be reached through the pass. dueThreads refuses a conversation any
// of whose messages is limited, so by the time a thread is offered its window
// has nothing to exclude — a test driving RunWorkspace passes whether the
// clause is there or not, and one written that way is vacuous however
// convincing its narrative.
//
// What the clause defends is the interval between two transactions: dueThreads
// commits, readThread opens its own, and under read-committed a narrowing by
// another process lands in between and is visible only to the later statement.
// A single-process test cannot stage a concurrent writer, so this calls
// threadMessages directly with a thread the offer would have refused, which
// puts the statement in exactly the state that interval produces.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheWindowReadExcludesALimitedMessageOfItsOwnAccord(t *testing.T) {
	e := integration.Setup(t)
	owner := integration.OwnerConn(t)
	ctx := context.Background()
	at := time.Now().UTC().Add(-48 * time.Hour)

	const key = "thread-window"
	seed := func(body, audience string, offset time.Duration) {
		t.Helper()
		id := ids.NewV7()
		if _, err := owner.Exec(ctx, `
			INSERT INTO activity (id, kind, direction, subject, body, thread_key,
			                      occurred_at, created_at, source, captured_by, audience)
			VALUES ($1, 'email', 'inbound', 'Renewal', $2, $3, $4, $4, 'gmail', 'connector:gmail', $5)`,
			id, body, key, at.Add(offset), audience); err != nil {
			t.Fatal(err)
		}
	}
	seed("Happy to continue.", "workspace", 0)
	seed("internal note: we are preparing to terminate", "participants", time.Minute)

	// The thread as dueThreads would have handed it over BEFORE the narrowing:
	// settled, never read, both messages counted. That is the only state in
	// which the window read is reached with something to exclude.
	thread := settledThread{Key: key, Newest: at.Add(time.Minute), Count: 2}

	var window []threadMessage
	as := e.Admin()
	if err := database.WithWorkspaceTx(as, e.Pool, func(tx pgx.Tx) error {
		var err error
		window, err = threadMessages(as, tx, &thread)
		return err
	}); err != nil {
		t.Fatalf("reading the window: %v", err)
	}

	if len(window) == 0 {
		t.Fatal("the window came back empty — the fixture cannot tell an excluded message from a broken query")
	}
	for _, m := range window {
		if m.Body == "internal note: we are preparing to terminate" {
			t.Error("the window carried a message limited after its thread was offered — " +
				"between dueThreads and this statement is exactly where a narrowing by another process lands")
		}
	}
	if len(window) != 1 {
		t.Errorf("the window carried %d message(s), want the one open message", len(window))
	}
}
