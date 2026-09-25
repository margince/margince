// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The seat's own copy of a message whose natural key somebody else holds.
//
// A captured email is keyed ('email', <Message-ID>), and that header is typed
// by whoever sent the mail. So a collision on it is evidence of nothing: it may
// be two mailboxes genuinely holding one message, or it may be a row filed by a
// seat who only knew what to type. replayClaimIsProvenTx exists to tell those
// apart before the arriving seat is given anything over the incumbent, and it
// refuses whenever it cannot — which is the right direction for the PRIVACY
// question it was asked.
//
// It was the wrong direction for availability, and this is the other half.
// Refusing used to mean the arriving message was dropped: the skip wraps
// connector.ErrSkip, which advances the watermark, so a seat's own mailbox
// content was denied by a row they could prove nothing about and no later pass
// retried it. Now the collision costs the arriving seat the shared key, not the
// message — it lands as THEIR row, under a key of their own.
//
// The key is seat-scoped and deterministic, so the seat's next sync of the same
// message collides with its own row and is the ordinary replay it always was.
//
// MAIL ONLY, and the line is the natural key's provenance rather than
// convenience. A Message-ID is sender-controlled, so a collision on it proves
// nothing. Every other source system keys on an id the PROVIDER issued — a
// HubSpot engagement, a calendar event — where a collision means two seats
// reached one object, and filing a second row would invent a duplicate of
// something that really is one record.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// scopedToSeat answers whether this collision may be resolved by filing the
// seat's own row, and the record to file when it may.
//
// False for a record with no seat behind it: replayClaimIsProvenTx already
// treats one as claiming nothing, so an unprovable collision cannot arise, and
// a key scoped to the nil seat would be a second shared key rather than
// anybody's own.
func scopedToSeat(ctx context.Context, rec connector.NormalizedRecord) (connector.NormalizedRecord, bool) {
	if rec.NaturalKey.SourceSystem != connector.EmailSourceSystem || rec.NaturalKey.SourceID == "" {
		return connector.NormalizedRecord{}, false
	}
	seat := actorUserID(ctx)
	if seat == ids.Nil {
		return connector.NormalizedRecord{}, false
	}
	scoped := rec
	scoped.NaturalKey.SourceID = connector.SeatScopedMailKey(rec.NaturalKey.SourceID, seat.String())
	return scoped, true
}

// fileUnderOwnReplayKey lands the arriving message as this seat's own row.
//
// The incumbent is not read, not rewritten and not joined: the seat gets the
// message their own mailbox delivered, assembled from their own payload, and
// takes nothing from the row that held the shared key. That is what makes this
// safe where widening the proof would not be — a fifth arm admitting "my
// address is on the arriving message" is satisfied by a forger who typed it,
// and would hand them a content grant on the colleague's mail.
//
// The cross-door identity is claimed under the SHARED key, not the scoped one:
// the message's identity is its Message-ID whoever holds the row, and
// ClaimIdentity reports a losing claim as a non-error by contract. The
// incumbent keeps it, which is correct — it got there first.
func (s *Sink) fileUnderOwnReplayKey(
	ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord,
	fields ActivityFields, birth birthDecision, memberBound bool,
) (datasource.EntityRef, bool, counterpartyDecision, error) {
	scoped, ok := scopedToSeat(ctx, rec)
	if !ok {
		return datasource.EntityRef{}, false, counterpartyDecision{}, skipInvisibleIncumbent(rec, "activity")
	}
	// THIS message's own original, under this message's own key.
	//
	// storeRawCapture ran before the collision was known, under the SHARED key,
	// and its conflict path answers with the row the FIRST delivery stored. Left
	// as it stands, the scoped activity would name the incumbent's bytes: the
	// participant replay would derive this seat's parties from somebody else's
	// message, and purging this row would destroy the other mailbox's original.
	//
	// Nil first, so the append-once early return does not hand the shared id
	// straight back. An answer of nothing means this record carried no bytes of
	// its own — a connector that stored its own original, or a record with no raw
	// at all — and then what it already names IS this message's own.
	scoped.StoredOriginal = ids.Nil
	own, err := storeRawCapture(ctx, tx, scoped)
	if err != nil {
		return datasource.EntityRef{}, false, counterpartyDecision{}, err
	}
	if own.IsZero() {
		own = rec.StoredOriginal
	}
	scoped.StoredOriginal = own

	id, created, err := s.upsertActivity(ctx, tx, scoped, fields, birth)
	if err != nil {
		return datasource.EntityRef{}, false, counterpartyDecision{}, err
	}
	ref := datasource.EntityRef{Type: datasource.EntityActivity, ID: id.UUID}
	if !created {
		// This seat's own earlier copy: the ordinary replay, which writes
		// nothing. Reached on every later sync of the same message.
		return ref, false, counterpartyDecision{}, nil
	}
	if err := s.claimRecordIdentity(ctx, tx, id, rec); err != nil {
		return datasource.EntityRef{}, false, counterpartyDecision{}, err
	}
	decision, err := s.finishNewActivity(ctx, tx, id, scoped, fields, birth, memberBound)
	if err != nil {
		return datasource.EntityRef{}, false, counterpartyDecision{}, err
	}
	return ref, true, decision, nil
}
