// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// When a message may take the answer a classifier already gave its thread.
//
// A thread's verdict is about the conversation the classifier SAW. A reply from
// a party who was not in it is not that conversation, and treating it as one is
// how a settled customer thread carries a lawyer's first message into a shared
// timeline: same thread key, same subject line, entirely different message.
//
// So inheritance binds to the exact addresses the verdict saw, never to their
// domain. `x@acme.com` clearing a thread says nothing about `y@acme.com` — one
// is the buyer, the other may be their counsel, and a domain match cannot tell
// them apart.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The verdict states a thread's ledger row can be in.
const (
	VerdictPending       = "pending"
	VerdictCleared       = "cleared"
	VerdictHeld          = "held"
	VerdictUnsure        = "unsure"
	VerdictSharedByOwner = "shared_by_owner"
	VerdictHeldByOwner   = "held_by_owner"
)

// inheritedVerdictTx answers the verdict status this message takes from its
// thread, or "" when it takes none and the posture decides.
//
// An OPENING verdict (cleared, shared_by_owner) is inherited only when this
// message's sender is among the addresses the verdict saw. Any other sender
// re-opens the thread to pending and the message is born held — the classifier
// gets to look at the conversation again now that somebody new is in it.
//
// A HOLDING verdict (held, unsure, held_by_owner) is inherited whoever sent it.
// The asymmetry is deliberate: an unseen sender is a reason to doubt an opening
// answer and no reason at all to doubt a holding one.
func inheritedVerdictTx(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord) (string, error) {
	user := actorUserID(ctx)
	if user == ids.Nil || rec.ThreadKey == "" {
		return "", nil
	}
	var status string
	var seen []string
	err := tx.QueryRow(ctx, `
		SELECT status, seen_addresses FROM capture_thread_verdict
		 WHERE thread_key = $1 AND user_id = $2`, rec.ThreadKey, user).Scan(&status, &seen)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("capture: reading this thread's verdict: %w", err)
	}

	switch status {
	case VerdictHeld, VerdictUnsure, VerdictHeldByOwner:
		return status, nil
	case VerdictCleared, VerdictSharedByOwner:
		if senderWasSeen(rec, seen) {
			return status, nil
		}
		// A party the verdict never saw. Re-open the ledger rather than
		// inheriting either way, and let the posture hold the message until the
		// classifier has looked at the conversation this sender is now part of.
		if err := reopenClearedThreadTx(ctx, tx, rec.ThreadKey); err != nil {
			return "", err
		}
		return VerdictPending, nil
	}
	// pending: nothing has concluded yet, so the posture decides.
	return "", nil
}

// senderWasSeen answers whether this message's author is one of the addresses
// the thread's verdict was given.
//
// The FROM address alone, not every party on the message. A cleared thread that
// a new recipient is copied on is still the conversation the classifier read;
// a message WRITTEN by somebody it never saw is not.
func senderWasSeen(rec connector.NormalizedRecord, seen []string) bool {
	from := strings.ToLower(strings.TrimSpace(rec.Counterparty.Email))
	if from == "" {
		return false
	}
	for _, s := range seen {
		if s == from {
			return true
		}
	}
	return false
}

// reopenClearedThreadTx returns a settled thread to pending, so the classifier
// looks at it again.
//
// Only an OPENING verdict is re-opened. A thread already held stays held: new
// information that a conversation involves somebody unexpected is never a
// reason to re-ask whether it may be published.
func reopenClearedThreadTx(ctx context.Context, tx pgx.Tx, threadKey string) error {
	if threadKey == "" {
		return nil
	}
	user := actorUserID(ctx)
	if user == ids.Nil {
		return nil
	}
	// first_activity_id is CLEARED, not kept.
	//
	// The classifier is shown the message that row names, so leaving it
	// pointing at the message a previous answer was about would re-ask the same
	// question about the same text — and then apply the answer to whatever
	// triggered the re-ask. The unseen sender that caused the reopen would be
	// judged by correspondence they were never part of.
	//
	// The next message on this thread supplies a new one: EnsureTx fills the
	// column on the row it finds empty, and a claim skips a row that still has
	// none rather than judging an empty prompt.
	if _, err := tx.Exec(ctx, `
		UPDATE capture_thread_verdict
		   SET status = 'pending', kind = NULL, confidence = NULL,
		       first_activity_id = NULL, seen_addresses = '{}',
		       resolved_at = NULL, next_attempt_at = now(), updated_at = now()
		 WHERE thread_key = $1 AND user_id = $2
		   AND status IN ('cleared', 'shared_by_owner')`,
		threadKey, user); err != nil {
		return fmt.Errorf("capture: re-opening the thread's verdict: %w", err)
	}
	return nil
}
