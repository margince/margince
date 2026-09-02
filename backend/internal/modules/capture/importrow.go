// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Which mailbox a message came from, recorded once per mailbox rather than
// once per message.
//
// The activity row can only name ONE importer — captured_by is a single string,
// stamped by whichever sync landed the message first. Every seat whose mailbox
// also delivered it left no trace, and so had no way to say anything about it:
// no posture, no verdict, no hold. capture_import is that trace.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// recordImportTx records that THIS seat's mailbox delivered this message.
//
// Written on every capture of the message, not only the one that created the
// activity row: the second mailbox to deliver a message is exactly the case the
// table exists for, and it reaches this code through the replay path, where the
// activity already exists and nothing else is written.
//
// ON CONFLICT DO NOTHING rather than an upsert: a re-sync of the same message
// into the same mailbox is the same import, and the columns a later step writes
// onto this row (the posture at import, the verdict) are decisions that must not
// be reset by the mailbox simply syncing again.
func recordImportTx(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID,
	ownerUserID ids.UUID, birth birthDecision,
) error {
	if ownerUserID == ids.Nil {
		// No identifiable seat behind this capture. Nothing to attribute the
		// import to, and a row keyed on a nil user would claim a seat that
		// does not exist.
		return nil
	}
	// The SEAT, not the connection that delivered the message. The Sink is
	// handed a record and a principal, not the grant that produced them, so the
	// delivering connection is not in reach here — and guessing it from the
	// seat's newest live connection would name the wrong mailbox for anyone who
	// has two. A seat's decisions are per seat anyway, so the column would have
	// no reader; it lands with the code that can fill it honestly.
	if _, err := tx.Exec(ctx, `
		INSERT INTO capture_import (activity_id, user_id, posture_at_import, verdict_status, verdict_reason, verdict_reasons)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), $6)
		ON CONFLICT (activity_id, user_id) DO NOTHING`,
		activityID, ownerUserID, birth.posture, birth.verdictStatus, birth.reason, birth.reasons); err != nil {
		return fmt.Errorf("capture: recording the import of %s: %w", activityID, err)
	}
	return nil
}

// AudienceRecomputer derives a captured activity's audience from every
// mailbox that imported it. The activities module owns the activity table and
// so owns this derivation; compose injects it, because capture never imports a
// sibling.
//
// Nil is a Sink that records import rows and derives nothing from them. That is
// a real configuration rather than a broken one — a deployment composing capture
// without the timeline still wants the provenance — and it is also what every
// existing test fixture is until it says otherwise.
type AudienceRecomputer func(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID) error

// WithAudienceRecompute returns a copy that derives the audience of every
// message it imports from the set of mailboxes that imported it.
func (s *Sink) WithAudienceRecompute(recompute AudienceRecomputer) *Sink {
	c := *s
	c.recomputeAudience = recompute
	return &c
}

// recordThisImport writes the acting seat's import row, makes them a
// participant, and re-derives the audience over the contributors that now
// includes them.
//
// Both writes are idempotent, which is what lets the replay path and the
// creation path call the same function: on a first capture the participant row
// is the one stampCaptureParticipants already wrote, and on a replay it is the
// one this seat never had.
func (s *Sink) recordThisImport(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID,
	rec connector.NormalizedRecord, fields ActivityFields, birth birthDecision,
) error {
	// actor.UserID, the same seat captured_by names and stampCaptureParticipants
	// stamps — NOT capturePrincipal's OnBehalfOf fallback. Every connector
	// principal sets the two equal today, so this changes nothing now; it means
	// a future principal that sets only one cannot write an import row naming a
	// different seat than the one the content gate below just checked.
	owner := actorUserID(ctx)
	// This seat's mailbox must actually have DELIVERED the message before they
	// are recorded as having imported it.
	//
	// A message's identity is (source_system, source_id), source_id is the
	// RFC822 Message-ID, and a Message-ID is a header the sender types: any seat
	// can mint a mail carrying somebody else's Message-ID and sync it. Without a
	// check that capture hits the incumbent, writes an import row and a
	// participant row against a colleague's held thread, and the audience arm
	// reads both as GRANTS — so a forged header would buy the content of
	// correspondence the forger was never on.
	//
	// The evidence is one of the SEAT'S OWN addresses appearing on the message.
	// A forger controls every header they write, but not which mailbox a message
	// was delivered to: they cannot put their own connected address on a message
	// nobody sent them, and an address they merely claim is not in their identity
	// set unless they declared it, which is a claim about themselves.
	//
	// Not "can this seat already read it": on an INBOUND message the first
	// capture deliberately refuses to bind a colleague's user_id from a Cc line
	// the sender wrote (participant.go says why), so a genuine second recipient
	// has no participant row yet and cannot read a held message — which is
	// exactly the case this path exists to serve.
	delivered, err := mailboxWasARecipientTx(ctx, tx, rec)
	if err != nil {
		return err
	}
	if !delivered {
		// Not this seat's message to claim. The capture already stored nothing
		// (the natural key collided), so there is nothing to undo and nothing to
		// tell the connector: the message is on the timeline, for the people it
		// belongs to.
		return nil
	}
	if err := recordImportTx(ctx, tx, id, owner, birth); err != nil {
		return err
	}
	if err := stampCaptureParticipants(ctx, tx, id, owner, fields.Kind, fields.Direction, rec.Counterparty.Email); err != nil {
		return err
	}
	// The confidentiality question for THIS seat's view of the thread, opened
	// in the same transaction as the message it is about. A message that landed
	// with nobody scheduled to judge it would stay held forever under the
	// classified posture, which is indistinguishable from the product being
	// broken.
	//
	// Only for a message this seat's posture actually holds: an already-open
	// message has nothing for the verdict to open, and asking anyway would
	// spend a model call per message on a `shared` mailbox to reach the answer
	// it already has.
	if err := openConfidentialityQuestionTx(ctx, tx, id, owner, rec, fields, birth); err != nil {
		return err
	}
	if s.recomputeAudience == nil {
		return nil
	}
	return s.recomputeAudience(ctx, tx, id)
}

// mailboxWasARecipientTx answers whether one of the acting seat's own addresses
// is on this message — the evidence that their provider delivered it, rather
// than that they typed its Message-ID.
//
// EXACT addresses only, never a declared domain. A seat declares a domain with
// no proof of control, so a domain arm here would let anybody claim a colleague's
// domain and then treat any message naming an address on it as delivered to
// them — which is the forgery this gate exists to refuse, taking the long way
// round. An exact address is different in kind: it is either the mailbox the
// provider attested at grant, or one the seat declared about themselves, and
// neither names a colleague.
func mailboxWasARecipientTx(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord) (bool, error) {
	self, err := ownerIdentitiesTx(ctx, tx)
	if err != nil {
		return false, err
	}
	if self.Empty() {
		return false, nil
	}
	for _, a := range rec.Addresses {
		if self.CoversAddressExactly(a) {
			return true, nil
		}
	}
	return false, nil
}
