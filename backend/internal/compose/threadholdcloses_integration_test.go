// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A thread opened for the workspace and then held closes the messages it had
// opened. The mail arrives through the real capture sink, and the verdicts come
// from the real engine and the owner's own decision.

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	holdCustomer = "pat@counterparty.example"
	holdLawyer   = "anwalt@kanzlei.example"
)

// reopenedThread is a thread whose first two messages were opened, and whose
// third, from a party nobody judged, sent the question back to the classifier.
type reopenedThread struct {
	question ids.UUID
	opened   []ids.UUID
	reopener ids.UUID
}

// classifyRep1sMail makes Rep1's mailbox one whose mail waits for a verdict.
func classifyRep1sMail(t *testing.T, e *integration.Env) {
	t.Helper()
	seatAddress(t, e, e.Rep1)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE capture_connection SET mail_posture = 'classified' WHERE user_id = $1`, e.Rep1)
		return err
	}); err != nil {
		t.Fatalf("classifying the mailbox: %v", err)
	}
}

// mailFrom is threadMail sent by another party.
func mailFrom(from, to, messageID, at, inReplyTo string, references ...string) []byte {
	return []byte(strings.Replace(string(threadMail(to, messageID, at, inReplyTo, references...)),
		"From: Pat Counterparty <"+holdCustomer+">", "From: "+from, 1))
}

func activityOfMessage(t *testing.T, e *integration.Env, messageID string) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT activity_id FROM activity_identity
			 WHERE identity_kind = 'mail' AND identity_key = $1`, messageID).Scan(&id)
	}); err != nil {
		t.Fatalf("reading the activity of %s: %v", messageID, err)
	}
	return id
}

// openThenReopen captures two messages from the customer, opens them, then
// captures a lawyer's reply that sends the thread back for a verdict. share
// opens them through the owner's decision; otherwise the classifier opens them.
func openThenReopen(t *testing.T, e *integration.Env, root string, share bool) reopenedThread {
	t.Helper()
	classifyRep1sMail(t, e)
	addr := seatAddress(t, e, e.Rep1)
	captureThreadMail(t, e, e.Rep1, threadMail(addr, root, "Mon, 01 Jun 2026 08:00:00 +0000", ""))
	captureThreadMail(t, e, e.Rep1, threadMail(addr, "second-"+root, "Tue, 02 Jun 2026 08:00:00 +0000", root, root))
	var thread reopenedThread
	thread.opened = []ids.UUID{activityOfMessage(t, e, root), activityOfMessage(t, e, "second-"+root)}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT id FROM capture_thread_verdict WHERE thread_key = $1 AND user_id = $2`,
			root, e.Rep1).Scan(&thread.question)
	}); err != nil {
		t.Fatalf("reading the thread question: %v", err)
	}
	if share {
		if _, err := NewThreadAudienceSetter(e.Pool).Decide(purgeCtx(e, e.Rep1), root, true); err != nil {
			t.Fatalf("sharing the thread: %v", err)
		}
	} else {
		runConfidentiality(t, e, thread.question, confidentialityOrdinary, 0.95)
	}
	for _, id := range thread.opened {
		if got := activityAudience(t, e, id); got != "workspace" {
			t.Fatalf("an opened message is %q, want workspace", got)
		}
	}
	captureThreadMail(t, e, e.Rep1, mailFrom(holdLawyer, addr, "lawyer-"+root, "Wed, 03 Jun 2026 08:00:00 +0000", root, root))
	thread.reopener = activityOfMessage(t, e, "lawyer-"+root)
	if got := threadStatus(t, e, thread.question); got != capture.VerdictPending {
		t.Fatalf("thread status after an unread party wrote = %q, want pending", got)
	}
	return thread
}

// assertClosed checks every message of the thread is held, and Rep1's own
// contribution says why.
func assertClosed(t *testing.T, e *integration.Env, thread reopenedThread, status string) {
	t.Helper()
	for _, id := range append(thread.opened, thread.reopener) {
		if got := activityAudience(t, e, id); got != "participants" {
			t.Errorf("a message of a thread held as %s is %q, want participants", status, got)
		}
		if got := importVerdictFor(t, e, id, e.Rep1); got != status {
			t.Errorf("its import row says %q, want %s", got, status)
		}
	}
}

func TestAHeldVerdictClosesTheMessagesTheThreadHadOpened(t *testing.T) {
	e := integration.Setup(t)
	thread := openThenReopen(t, e, "held-root@counterparty.example", false)

	runConfidentiality(t, e, thread.question, confidentialityPersonnel, 0.95)

	assertClosed(t, e, thread, capture.VerdictHeld)
}

func TestAnUnsureVerdictClosesTheMessagesTheThreadHadOpened(t *testing.T) {
	e := integration.Setup(t)
	thread := openThenReopen(t, e, "unsure-root@counterparty.example", false)

	// An opening answer below the confidence floor holds the thread as unsure.
	runConfidentiality(t, e, thread.question, confidentialityOrdinary, 0.1)

	if got := threadStatus(t, e, thread.question); got != capture.VerdictUnsure {
		t.Fatalf("thread status = %q, want unsure", got)
	}
	assertClosed(t, e, thread, capture.VerdictUnsure)
}

func TestAHeldVerdictClosesMessagesTheOwnerHadShared(t *testing.T) {
	e := integration.Setup(t)
	thread := openThenReopen(t, e, "shared-root@counterparty.example", true)
	for _, id := range thread.opened {
		if got := importVerdictFor(t, e, id, e.Rep1); got != capture.VerdictSharedByOwner {
			t.Fatalf("a shared message's import row says %q, want shared_by_owner", got)
		}
	}

	runConfidentiality(t, e, thread.question, confidentialityPersonnel, 0.95)

	assertClosed(t, e, thread, capture.VerdictHeld)
}

// A colleague who imported the same message keeps their own contribution. The
// message still ends at the strictest of the two, which is now held.
func TestAHeldVerdictLeavesAColleaguesOwnClearanceAlone(t *testing.T) {
	e := integration.Setup(t)
	thread := openThenReopen(t, e, "colleague-root@counterparty.example", false)
	both := thread.opened[0]
	addImportRowFor(t, e, both, e.Rep2)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			UPDATE capture_import SET verdict_status = 'cleared'
			 WHERE activity_id = $1 AND user_id = $2`, both, e.Rep2)
		return err
	}); err != nil {
		t.Fatalf("clearing the colleague's own contribution: %v", err)
	}

	runConfidentiality(t, e, thread.question, confidentialityPersonnel, 0.95)

	if got := importVerdictFor(t, e, both, e.Rep2); got != capture.VerdictCleared {
		t.Fatalf("the colleague's own contribution says %q, want cleared: one seat's verdict "+
			"never writes another seat's row", got)
	}
	assertClosed(t, e, thread, capture.VerdictHeld)
}

// The owner can open the thread again after it was held.
func TestAnOwnerCanShareAThreadAgainAfterItWasHeld(t *testing.T) {
	e := integration.Setup(t)
	root := "reshare-root@counterparty.example"
	thread := openThenReopen(t, e, root, false)
	runConfidentiality(t, e, thread.question, confidentialityPersonnel, 0.95)
	assertClosed(t, e, thread, capture.VerdictHeld)

	if _, err := NewThreadAudienceSetter(e.Pool).Decide(purgeCtx(e, e.Rep1), root, true); err != nil {
		t.Fatalf("sharing the thread again: %v", err)
	}

	for _, id := range append(thread.opened, thread.reopener) {
		if got := activityAudience(t, e, id); got != "workspace" {
			t.Errorf("a message of a thread its owner shared again is %q, want workspace", got)
		}
	}
}

// One RecordOutcomeOnThreadTx pass takes 500 messages. A held verdict on a
// longer thread still closes every message in the same transaction.
func TestAHeldVerdictClosesEveryOpenedMessageOfALongThread(t *testing.T) {
	e := integration.Setup(t)
	messages := seedOpenedUnderVerdict(t, e, "long-live", capture.VerdictCleared, capture.VerdictCleared, 620)
	var question ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		// Back to an open question, as a new party's reply leaves it.
		return tx.QueryRow(context.Background(), `
			UPDATE capture_thread_verdict
			   SET status = 'pending', first_activity_id = $2, resolved_at = NULL,
			       next_attempt_at = now() - interval '1 minute'
			 WHERE thread_key = 'long-live' AND user_id = $1
			RETURNING id`, e.Rep1, messages[0]).Scan(&question)
	}); err != nil {
		t.Fatalf("reopening the long thread: %v", err)
	}

	runConfidentiality(t, e, question, confidentialityPersonnel, 0.95)

	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = ANY($1) AND audience <> 'participants'`, messages); n != 0 {
		t.Fatalf("%d of %d messages of the held thread are still open", n, len(messages))
	}
	if n := e.WsCount(t, `SELECT count(*) FROM capture_import WHERE activity_id = ANY($1) AND verdict_status <> 'held'`, messages); n != 0 {
		t.Fatalf("%d import rows of the held thread do not say held", n)
	}
}
