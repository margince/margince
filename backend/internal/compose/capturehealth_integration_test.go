// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The capture-health page reports a backlog nobody else can see.
//
// The contacts it counts are owner-private, and ownerPrivateTables makes them
// invisible to every reader but their owner — not even an administrator. So an
// admin looking for them finds nothing, and a count is the only way the backlog
// can be reported at all. That is the case worth holding: the page must see
// what its reader cannot.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestCaptureHealthCountsABacklogAnAdminCannotSee(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)

	// A captured contact still owner-private, with no settled answer about its
	// sender: exactly the row the sweeps repair and nobody can see.
	person := ids.NewV7()
	e.WsExec(t, `INSERT INTO person (id, full_name, owner_id, visibility, captured_by, source)
		VALUES ($1, 'Waiting Contact', $2, 'owner', 'connector:gmail', 'capture')`, person, e.Rep1)
	e.WsExec(t, `INSERT INTO person_email (person_id, email, source, captured_by) VALUES ($1, 'waiting@example.test', 'capture', 'connector:gmail')`, person)
	// And a thread whose confidentiality question is still open.
	e.WsExec(t, `INSERT INTO capture_thread_verdict (thread_key, user_id, status)
		VALUES ('thread:open', $1, 'pending')`, e.Rep1)
	// The ledger rows point at the message that raised the question.
	activity := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'Hello', now(), 'capture', 'connector:gmail')`, activity)

	// A sender the classifier has not answered, and one it gave up on.
	e.WsExec(t, `INSERT INTO capture_pending_counterparty (email, owner_id, activity_id, status, next_attempt_at)
		VALUES ('asked@example.test', $1, $2, 'pending', now())`, e.Rep1, activity)
	e.WsExec(t, `INSERT INTO capture_pending_counterparty (email, owner_id, activity_id, status, next_attempt_at)
		VALUES ('gaveup@example.test', $1, $2, 'pending', NULL)`, e.Rep1, activity)
	e.WsExec(t, `INSERT INTO capture_pending_counterparty (email, owner_id, activity_id, status)
		VALUES ('cannottell@example.test', $1, $2, 'unsure')`, e.Rep1, activity)

	var health crmcontracts.CaptureHealth
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		health, err = readCaptureHealth(ctx, tx, time.Now().UTC())
		return err
	}); err != nil {
		t.Fatalf("reading capture health: %v", err)
	}

	var mine *int
	for _, box := range health.Mailboxes {
		if ids.UUID(box.UserId) == e.Rep1 {
			contacts := box.ContactsAwaitingDecision
			mine = &contacts
			if box.ThreadsAwaitingVerdict != 1 {
				t.Errorf("threads awaiting a verdict = %d, want the one held thread",
					box.ThreadsAwaitingVerdict)
			}
			if box.OldestContactAgeSeconds == nil {
				t.Error("a mailbox with a waiting contact reports no age — absent means an " +
					"empty queue, which is the opposite of what this row says")
			}
		}
	}
	if mine == nil {
		t.Fatalf("the mailbox with the backlog is absent from %+v — a page that cannot see "+
			"an owner-private contact reports the same nothing an admin already sees",
			health.Mailboxes)
	}
	if *mine != 1 {
		t.Errorf("contacts awaiting a decision = %d, want the one owner-private contact", *mine)
	}

	// The three classifier states are counted apart, which is the reading that
	// distinguishes "the mail is hard" from "the machine is not running".
	if health.Classifier.Pending != 1 {
		t.Errorf("pending = %d, want the one still due", health.Classifier.Pending)
	}
	if health.Classifier.Exhausted != 1 {
		t.Errorf("exhausted = %d, want the one nothing will ask about again",
			health.Classifier.Exhausted)
	}
	if health.Classifier.Unsure != 1 {
		t.Errorf("unsure = %d, want the one waiting on a human", health.Classifier.Unsure)
	}
}
