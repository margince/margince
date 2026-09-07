// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Whether the message about to go is the message that was authorized.
//
// A decision is taken twice: once when the message is staged, and again before
// the provider is handed it. Both phases stamp a fingerprint of the wording, so
// a later reader can tell the two apart — but the staging value was never read
// back, which made the second stamp a record of what went out rather than a
// check on it. A body edited between the two phases authorized one message and
// sent another, and nothing said so.
//
// MAIL AND CHANNEL AGREE, which was worth checking rather than assuming: a
// channel reply carries no subject at all — SendMessageInput has no such field,
// because a chat message has no subject line — so its request leaves Subject
// empty. WordingDigest separates the two strings with a NUL, so an empty
// subject is unambiguous rather than colliding with a body that begins the same
// way, and the two paths reduce identical content to the same digest.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// stagedWordingFor answers the fingerprint this delivery's staging phase
// recorded, and whether there was one.
//
// FALSE is not a fault. A delivery staged before this column carried anything
// has no fingerprint to compare, and inventing a mismatch from an absence would
// park mail that nothing is wrong with. The caller treats "no staged wording"
// as "nothing to disagree with", which is the honest reading of a record that
// was never made.
//
// ERASURE IS NOT ONE OF THOSE CASES, though it looks like it should be. The
// privacy paths tombstone a decision row in place — they blank the address and
// the subject and leave the fingerprint standing — because the row is Art. 5(2)
// accountability evidence, and migration 1788529047 revokes UPDATE on it from
// the runtime role besides. An erased subject's delivery still has its wording
// on record.
//
// Any recipient's row answers, because the fingerprint is per MESSAGE rather
// than per addressee: the staging writer computes the digest once above its
// recipient loop and binds the same value into every row.
//
// A delivery has ONE staging set today: all three production callers mint the
// delivery id and stage it in the same transaction, and a retry produces a new
// delivery rather than a second staging set. The ordering is what keeps that
// from being load-bearing — if a re-stage ever lands, this names the newest set
// deterministically instead of leaving the planner to choose.
func stagedWordingFor(ctx context.Context, tx pgx.Tx, deliveryID ids.UUID) ([]byte, bool, error) {
	var sum []byte
	// ONE staging set, named by its own id, and the newest by TIME rather than
	// by row id. uuidv7 here is a millisecond timestamp over random low bits, so
	// two rows written inside one millisecond do not order — the id is unique,
	// not monotonic at that resolution. decided_at is the transaction clock, so
	// it ties across a set and separates two sets; decision_set_id breaks the
	// remaining tie, which is what keeps this deterministic rather than
	// planner-dependent.
	//
	// Every row of one set carries the same fingerprint — the staging writer
	// computes the digest once above its recipient loop — so which row of the
	// chosen set answers does not matter. Which SET answers does.
	err := tx.QueryRow(ctx, `
		SELECT content_fingerprint
		  FROM communication_decision
		 WHERE delivery_id = $1 AND phase = $2
		   AND content_fingerprint IS NOT NULL
		 ORDER BY decided_at DESC, decision_set_id DESC
		 LIMIT 1`, deliveryID, string(commsauthz.PhaseStaging)).Scan(&sum)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("consent: read the wording this delivery was staged with: %w", err)
	}
	return sum, len(sum) > 0, nil
}

// wordingChangedSinceStaging reports whether the message being transmitted
// differs from the one that was authorized at staging.
//
// The comparison is over the DIGEST rather than the text, which is the whole
// reason the column holds a hash: the decision must be able to answer "is this
// the same message" without becoming a second copy of the mail.
// A wrong-length stored value reads as "nothing to compare" rather than as a
// mismatch. No writer in this tree produces one — the only staging writer binds
// a 32-byte digest — so this guards a row that could only arrive by hand, and
// it fails OPEN because parking real mail over a value this code cannot read
// would be the worse mistake.
func wordingChangedSinceStaging(staged []byte, subject, body, htmlBody string) bool {
	if len(staged) != sha256.Size {
		return false
	}
	now := SendingDigest(subject, body, htmlBody)
	if bytes.Equal(now[:], staged) {
		return false
	}
	// A delivery staged BEFORE this change carries the two-field digest, which
	// covered subject and body and knew nothing about markup. Its wording has
	// not changed — the shape of the question has — and refusing it would park
	// every message in flight at the moment this deployed.
	//
	// Accepted only for a plain-text send. A message that HAS markup cannot
	// have been staged under a digest that never saw it, so allowing the older
	// shape there would be accepting a fingerprint that answers a narrower
	// question than the one being asked.
	if htmlBody == "" {
		legacy := WordingDigest(subject, body)
		if bytes.Equal(legacy[:], staged) {
			return false
		}
	}
	return true
}

// wordingDiffersFromStaging answers whether this delivery's wording changed
// since the decision that authorized it.
//
// It runs on the transmit transaction, so the comparison and the ticket it
// settles commit together: reading the staged value outside would let a
// re-stage land between the read and the decision.
func (g *Gate) wordingDiffersFromStaging(
	ctx context.Context, tx pgx.Tx, req commsauthz.TransmitRequest,
) (bool, error) {
	staged, recorded, err := stagedWordingFor(ctx, tx, req.DeliveryID)
	if err != nil {
		return false, err
	}
	if !recorded {
		return false, nil
	}
	return wordingChangedSinceStaging(staged, req.Subject, req.Body, req.HTMLBody), nil
}
