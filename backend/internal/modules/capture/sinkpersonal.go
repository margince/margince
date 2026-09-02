// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// threadIsPrivateTx reports whether the confidentiality classifier has judged
// this thread PERSONAL — the mailbox owner's own life rather than the
// workspace's business.
//
// The two verdict lanes never spoke, and a founder's aunt is what that cost.
// Twenty of her threads were judged personal and held, and she was a contact in
// the shared CRM the whole time: the confidentiality lane decided what could be
// READ, the tier ladder decided what got CREATED, and nothing carried the first
// answer to the second. Deciding a conversation is somebody's private life and
// then filing its author as a business contact is one system disagreeing with
// itself in front of the person it is about.
//
// It reads the KIND and not only the status. A thread can be held for many
// reasons — legal, personnel, an explicit confidentiality marking — and those
// are business the workspace conducts privately, whose parties are genuine
// contacts. Only `personal` says the conversation is not the workspace's at all.
//
// Scoped to this seat's own verdict row, because that is how the ledger is keyed
// and because the question is about THEIR mailbox: another seat's conversation
// with the same person says nothing about whose this one is.
//
// The caller refuses the CREATE and writes no ledger row. That is deliberate:
// the disposition ledger is keyed on the ADDRESS and this is a fact about one
// THREAD, so a row here would bar an aunt who later sends a genuine business
// enquiry — or a customer who once wrote about something private — from ever
// becoming a contact, on a decision that was never about them as a sender.
//
// The message itself commits and keeps its audience. A personal thread is
// already held to the people on it, and refusing the record is not a reason to
// lose the mail.
func threadIsPrivateTx(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord) (bool, error) {
	user := actorUserID(ctx)
	if user == ids.Nil || rec.ThreadKey == "" {
		return false, nil
	}
	var status, kind string
	var seen []string
	if err := tx.QueryRow(ctx, `
		SELECT status, COALESCE(kind, ''), seen_addresses FROM capture_thread_verdict
		 WHERE thread_key = $1 AND user_id = $2`, rec.ThreadKey, user).Scan(&status, &kind, &seen); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No verdict yet. The thread may still turn out to be personal, and
			// the sweep that runs after the classifier answers is what catches
			// the contact this pass creates — see the compose seam.
			return false, nil
		}
		return false, fmt.Errorf("capture: reading whether this thread is private: %w", err)
	}
	if kind != ThreadKindPersonal {
		return false, nil
	}
	// Bound to the addresses the classifier actually SAW, exactly as an opening
	// verdict is (senderWasSeen, verdictinherit.go). thread_key is the message's
	// own References root, so a sender picks it verbatim: without this, anybody
	// who learns or provokes a personal thread key could write from a fresh
	// address onto that root and be refused a record — quietly keeping
	// themselves out of the CRM by borrowing somebody else's private
	// conversation.
	//
	// A party the verdict never saw is not in the conversation it judged, and
	// the ordinary ladder decides about them.
	if !senderWasSeen(rec, seen) {
		return false, nil
	}
	// held_by_owner is the seat's own hand: they marked the conversation
	// private, which is a stronger statement than the classifier's and says the
	// same thing about whether its author is a business contact.
	return status == VerdictHeld || status == VerdictHeldByOwner, nil
}

// PrivateThreadContact names one contact a personal verdict has just orphaned:
// somebody capture created whose correspondence with this seat is now entirely
// private.
type PrivateThreadContact struct {
	PersonID ids.PersonID
	OwnerID  ids.UUID
	Email    string
}

// ContactsOrphanedByPrivacyTx answers which contacts a thread's personal
// verdict has left with no business reason to exist.
//
// The verdict almost always arrives after the contact — capture creates on
// commit, classification reads the thread later — so the gate at creation time
// catches only the messages that arrive afterwards. This is what answers for the
// records already made.
//
// It asks about the person's WHOLE correspondence with this seat, not about the
// thread that triggered it. Somebody who writes about a private matter on
// Monday and a contract on Tuesday is a business contact who also has a private
// thread, and retracting them would lose a real counterparty. Only somebody
// whose every conversation here is personal has no business reason to be in the
// CRM.
//
// It reads addresses rather than person ids from the thread, because that is
// what the activity carries; the caller resolves each to the record capture
// made for it.
func ContactsOrphanedByPrivacyTx(
	ctx context.Context, tx pgx.Tx, threadKey string, user ids.UUID,
) ([]PrivateThreadContact, error) {
	if threadKey == "" || user == ids.Nil {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT p.id, p.owner_id, pe.email
		  FROM activity a
		  JOIN person_email pe ON pe.email = a.counterparty_email AND pe.archived_at IS NULL
		  JOIN person p ON p.id = pe.person_id AND p.archived_at IS NULL
		 WHERE a.thread_key = $1
		   AND a.counterparty_email <> ''
		   AND p.owner_id = $2
		   -- Every thread this seat shares with them is personal. One ordinary
		   -- conversation makes them a business contact who also has a private
		   -- one, and retracting that loses a real counterparty.
		   AND NOT EXISTS (
		         SELECT 1
		           FROM activity other
		           LEFT JOIN capture_thread_verdict tv
		             ON tv.thread_key = other.thread_key AND tv.user_id = $2
		          WHERE other.counterparty_email = a.counterparty_email
		            AND other.thread_key <> ''
		            AND (tv.kind IS DISTINCT FROM $3
		                 OR tv.status NOT IN ($4, $5)))`,
		threadKey, user, ThreadKindPersonal, VerdictHeld, VerdictHeldByOwner)
	if err != nil {
		return nil, fmt.Errorf("capture: reading the contacts a private verdict orphaned: %w", err)
	}
	defer rows.Close()
	var out []PrivateThreadContact
	for rows.Next() {
		var c PrivateThreadContact
		if err := rows.Scan(&c.PersonID, &c.OwnerID, &c.Email); err != nil {
			return nil, fmt.Errorf("capture: reading the contacts a private verdict orphaned: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("capture: reading the contacts a private verdict orphaned: %w", err)
	}
	return out, nil
}
