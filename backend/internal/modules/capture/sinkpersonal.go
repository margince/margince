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
	if err := tx.QueryRow(ctx, `
		SELECT status, COALESCE(kind, '') FROM capture_thread_verdict
		 WHERE thread_key = $1 AND user_id = $2`, rec.ThreadKey, user).Scan(&status, &kind); err != nil {
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
	// held_by_owner is the seat's own hand: they marked the conversation
	// private, which is a stronger statement than the classifier's and says the
	// same thing about whether its author is a business contact.
	return status == VerdictHeld || status == VerdictHeldByOwner, nil
}
