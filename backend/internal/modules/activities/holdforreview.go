// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Holding a refused message so the review that records it has something to
// resume.
//
// WHAT WAS WRONG. A send refused at the keyboard rolled its whole transaction
// back, and the message went with it. The review the refusal opened named no
// delivery intent, because there was nothing to name: the staging row the
// transaction would have written never committed. So a rep who came back to the
// review found a record of what happened and no way to act on it, and a
// controller-directed send had nothing to execute — the exact message that was
// judged no longer existed anywhere.
//
// WHAT THIS DOES. Before the refusal is recorded, the message is frozen into a
// HELD scheduled_send: the same freeze a scheduled send performs, into the same
// column, read back by the same fire path. The review then binds to that row,
// so resuming means firing an intent that already holds the message rather than
// asking the rep to retype it.
//
// WHAT IS FROZEN IS THE REP'S INPUT, NOT THE RENDERED MESSAGE, and that is the
// same choice scheduling makes for the same reason. The row keeps the subject,
// the body, the recipients, the attachment ids and the claimed purpose; the
// signature, the unsubscribe footer and the attachment contents are re-derived
// at fire, against the state that exists then.
//
// So a resumed message is not byte-identical to the one the engine refused: a
// rep who changes their signature in between sends the new one. That is
// deliberate rather than overlooked — a footer frozen a week ago can name a
// withdrawal link that has since been revoked — and it is safe because resuming
// re-runs every gate on what would actually go out.
//
// A DIRECTED SEND WILL NEED MORE THAN THIS ROW. Executing a controller's
// instruction means sending the message a human was shown and acknowledged, and
// this row cannot promise that on its own. Whatever binds an instruction to a
// message will have to compare what is about to go out against what was
// judged, rather than trusting that a held payload re-renders the same way.
// That comparison is that slice's to build; this one owes it a message to
// compare, which is what it provides.
//
// WHY A HELD ROW RATHER THAN A NEW TABLE. scheduled_send is already a resumable
// delivery intent. It carries a frozen payload, an origin, the agent
// provenance, a version for optimistic concurrency, and a 'held' status whose
// shape CHECK has always required a reason. A second store for the same thing
// would be a second fire path to keep in step with this one, and it is the
// second one that falls behind.
//
// NO TIMER IS ARMED. A scheduled send is a promise to send at a moment; this is
// the opposite — a message that must NOT go out until a human decides
// something. scheduled_at is set to now because the column is NOT NULL and the
// row's moment has already passed; nothing reads it as a due date, because
// nothing schedules a wake-up for it.
//
// BEST EFFORT, DELIBERATELY. A failure to hold does not replace the refusal.
// The send was refused either way, and answering a storage fault instead would
// tell the rep their message was fine and the database was not. What they lose
// is the ability to resume, which is a regression to the behaviour of the day
// before this file existed — not a new failure.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// holdForReview freezes a refused message into a held scheduled_send and
// answers the row's id, so the review recorded next can bind to it.
//
// A zero id means nothing was held, and every caller treats that as "record the
// review without an intent" rather than as an error — see the file comment.
func (s *Store) holdForReview(ctx context.Context, origin SendOrigin, in SendEmailInput) ids.UUID {
	id, err := s.holdForReviewTx(ctx, origin, in)
	if err != nil {
		// Swallowed on purpose, and this is the only place it happens. The
		// caller is on its way to answering a refusal the rep must see; a hold
		// that failed is an operator's problem, and raising it here would
		// replace the answer they need with one they cannot act on.
		return ids.UUID{}
	}
	return id
}

// holdForReviewTx is the write, split out so a test can see why a hold failed
// rather than only that it did.
func (s *Store) holdForReviewTx(ctx context.Context, origin SendOrigin, in SendEmailInput) (ids.UUID, error) {
	actor, err := storekit.Actor(ctx)
	if err != nil {
		return ids.UUID{}, err
	}
	if actor.UserID.IsZero() {
		// The same rule scheduling applies: a frozen message names the seat it
		// would fire under, and a row that names nobody could never be resumed
		// by anybody.
		return ids.UUID{}, errNoSchedulingUser
	}
	payload, err := json.Marshal(freezePayload(in))
	if err != nil {
		return ids.UUID{}, fmt.Errorf("held send: freezing the refused message: %w", err)
	}
	originLinks, err := marshalOriginLinks(origin)
	if err != nil {
		return ids.UUID{}, err
	}
	alsoLinks, err := marshalAlsoLinks(origin)
	if err != nil {
		return ids.UUID{}, err
	}
	prov := provenanceOf(actor)
	held := heldIdentity{
		reason:          HeldSendRefused,
		seat:            actor.UserID,
		principalKind:   principalKind(actor),
		originKind:      originKind(origin),
		agentActorID:    prov.ActorID,
		agentPassportID: prov.PassportID,
		anchor:          nullableAnchor(origin),
		originLinks:     originLinks,
		alsoLinks:       alsoLinks,
		payload:         payload,
	}
	id := ids.NewV7()
	now := s.now().UTC()
	err = s.tx(ctx, func(tx pgx.Tx) error {
		// LOCKED AGAINST A CONCURRENT ERASURE, FIRST.
		//
		// This transaction opens AFTER the refused send has rolled back, so
		// there is a gap: an erasure can sweep the installation in that gap,
		// certify the subject's data destroyed, and commit — and then this
		// write would put the subject's address, name and the words meant for
		// them back into the database, in a row nothing would find again. The
		// erasure's address list is derived from contact_email, which the sweep
		// has by then deleted, so a later erasure of the same contact would not
		// reach this row either.
		//
		// The same advisory lock the erasure takes on each address closes it.
		// One of the two waits for the other: the erasure either sweeps this
		// row along with everything else, or it finds nothing here and this
		// write lands before it starts.
		//
		// The addresses are lowered because the erasure hashes them lowered.
		// Two spellings of one mailbox taking two different locks would leave
		// the two transactions free to interleave, which is the gap itself.
		addresses := loweredRecipients(in)
		if err := storekit.LockSubjectKeys(ctx, tx, nil, addresses); err != nil {
			return err
		}
		// AND THEN ASKED WHETHER THE ERASURE ALREADY WON.
		//
		// The lock orders the two transactions; it does not decide which goes
		// first. If the erasure went first it has committed, certified the
		// subject's data destroyed, and released the lock — and this write
		// would then be the resurrection, not a race with one. So the question
		// is asked after the lock is held, when the answer cannot change until
		// this transaction ends.
		erased, err := anyAddressErased(ctx, tx, addresses)
		if err != nil {
			return err
		}
		if erased {
			// Held NOTHING, and the refusal still stands. The rep is told their
			// message was refused, which is true; what they do not get is a
			// message to resume, which is correct — there is nobody left to
			// send it to.
			return nil
		}
		written, err := holdOrReuseTx(ctx, tx, held, prov, now, in.Subject)
		if err != nil {
			return err
		}
		id = written
		return nil
	})
	if err != nil {
		return ids.UUID{}, err
	}
	return id, nil
}

// holdOrReuseTx writes the held row, or answers the id of the one this seat is
// already holding for the same message.
//
// THE SAME MESSAGE PRESSED TWICE IS ONE HELD MESSAGE. A rep refused at the
// keyboard reads the refusal and presses send again — sometimes immediately,
// sometimes after changing something that did not change the answer. Writing a
// fresh row each time would fill their held lane with copies of one message,
// and the review would bind to the newest while the older ones sat there
// forever with nothing pointing at them.
//
// Reusing the row is also what keeps ONE review per message: the review index
// is unique on the live intent, so a reused intent updates the standing review
// instead of opening a rival.
//
// What counts as the same message is heldIdentity, which is the unique index's
// own column list — see there.
func holdOrReuseTx(
	ctx context.Context, tx pgx.Tx, held heldIdentity,
	prov agentProvenance, now time.Time, subject string,
) (ids.UUID, error) {
	existing, err := heldTwinOf(ctx, tx, held)
	if err != nil {
		return ids.UUID{}, err
	}
	if !existing.IsZero() {
		return existing, nil
	}
	// ON CONFLICT because the read above and this write are not atomic: two
	// refusals of one message can both find nothing and both arrive here. The
	// index refuses the second, and DO NOTHING turns that refusal into an empty
	// result rather than an error — which is the signal to go and read the row
	// the winner wrote.
	var id ids.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO scheduled_send
		  (id, status, held_reason, scheduled_at, scheduled_tz,
		   origin_kind, anchor_activity_id, origin_links, also_links,
		   payload, payload_version, scheduled_by, principal_kind,
		   agent_actor_id, agent_passport_id, agent_on_behalf_of)
		VALUES ($1, 'held', $2, $3, 'UTC', $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT DO NOTHING
		RETURNING id`,
		ids.NewV7(), held.reason, now,
		held.originKind, held.anchor, held.originLinks, held.alsoLinks,
		held.payload, payloadVersionCurrent, held.seat, held.principalKind,
		prov.ActorID, prov.PassportID, prov.OnBehalfOf).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		// The other refusal won. Its row is the held message for both of us,
		// and both reviews bind to it — which is the one review per message
		// this whole rule exists to keep.
		//
		// A SECOND EMPTY READ IS NOT AN ERROR. The winner's row can stop being
		// held between the conflict and this read: an erasure cancels it, or a
		// rep resumes it. Answering an error there would replace the refusal
		// the rep is waiting for with a storage fault about a row that is
		// simply no longer theirs to reuse. A zero id means "nothing held", the
		// caller records the review without an intent, and the rep still gets
		// the answer they pressed send for.
		existing, err := heldTwinOf(ctx, tx, held)
		if err != nil {
			return ids.UUID{}, err
		}
		return existing, nil
	}
	if err != nil {
		return ids.UUID{}, fmt.Errorf("held send: recording the refused message: %w", err)
	}
	// Audited as 'hold' rather than 'schedule': the two write the same columns
	// and mean opposite things, and an auditor reading 'schedule' on a message
	// nothing will ever fire would be reading a lie.
	if _, err := storekit.Audit(ctx, tx, "hold", "scheduled_send", id, nil, map[string]any{
		"held_reason": held.reason,
		fieldSubject:  subject,
	}); err != nil {
		return ids.UUID{}, err
	}
	return id, nil
}

// holdRefusedSend holds a message the CONSENT ENGINE refused, and nothing else.
//
// THE NARROWNESS IS THE POINT. A send transaction fails for many reasons — a
// deadlock, an unreadable attachment, a provider seam that is not wired, a
// constraint nobody expected. None of those is work waiting on a human
// decision, and freezing a message for each of them would fill the held lane
// with rows whose only resolution is to try again.
//
// A consent refusal is different in kind: the message is fine, the engine
// decided it may not go to these recipients, and somebody has to decide what
// happens next. That is the one case with something to resume.
func (s *Store) holdRefusedSend(ctx context.Context, origin SendOrigin, in SendEmailInput, cause error) ids.UUID {
	if !errors.Is(cause, apperrors.ErrConsentNotGranted) {
		return ids.UUID{}
	}
	return s.holdForReview(ctx, origin, in)
}

// heldTwinOf finds a held row this seat already holds for the same message
// under the same authority, so a second press reuses it. A zero id means there
// is none.
//
// THE MATCH IS THE UNIQUE INDEX, column for column. The database refuses a
// second row for the same identity (scheduled_send_one_held_message_per_seat),
// and a query looser than that index would find a twin the index would not
// consider one — reusing a row for a message that is not the same message.
//
// The identity is wider than the payload for two reasons the index comments
// spell out. An agent send carries the granting human's user id, so the seat
// alone would let an agent inherit a human's held row and fire it over their
// signature with the passport check skipped. And two identical messages on two
// different threads are two messages: the anchor is part of the question the
// engine was asked, so resuming the wrong one files under the wrong record.
func heldTwinOf(ctx context.Context, tx pgx.Tx, held heldIdentity) (ids.UUID, error) {
	var id ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT id FROM scheduled_send
		 WHERE status = 'held'
		   AND held_reason = $1
		   AND scheduled_by = $2
		   AND principal_kind = $3
		   AND payload_version = $4
		   AND origin_kind = $5
		   AND coalesce(agent_actor_id, '') = coalesce($6::text, '')
		   AND coalesce(agent_passport_id, '00000000-0000-0000-0000-000000000000'::uuid)
		       = coalesce($7::uuid, '00000000-0000-0000-0000-000000000000'::uuid)
		   AND coalesce(anchor_activity_id, '00000000-0000-0000-0000-000000000000'::uuid)
		       = coalesce($8::uuid, '00000000-0000-0000-0000-000000000000'::uuid)
		   AND md5(coalesce(origin_links, '[]'::jsonb)::text) = md5(coalesce($9::jsonb, '[]'::jsonb)::text)
		   AND md5(coalesce(also_links, '[]'::jsonb)::text) = md5(coalesce($10::jsonb, '[]'::jsonb)::text)
		   AND md5(payload::text) = md5($11::jsonb::text)
		 LIMIT 1`,
		held.reason, held.seat, held.principalKind, payloadVersionCurrent, held.originKind,
		held.agentActorID, held.agentPassportID, held.anchor,
		held.originLinks, held.alsoLinks, held.payload).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.UUID{}, nil
	}
	if err != nil {
		return ids.UUID{}, fmt.Errorf("held send: looking for a message this seat already holds: %w", err)
	}
	return id, nil
}

// heldIdentity decides whether two refusals are refusals of ONE message: what
// would be sent, from whose mailbox, under whose authority, and onto which
// thread. Its fields are the unique index's columns, carried as one value so
// the writer and the twin read stay in step.
//
// Held by: TestTheHeldTwinReadMatchesItsUniquenessIndex
// (backend/gates/heldsendtwin_test.go)
type heldIdentity struct {
	reason          string
	seat            ids.UUID
	principalKind   string
	originKind      string
	agentActorID    *string
	agentPassportID *ids.UUID
	anchor          *ids.UUID
	originLinks     []byte
	alsoLinks       []byte
	payload         []byte
}

// loweredRecipients folds this message's addresses for the lock above. To, Cc
// and Bcc are all read: a blind copy is a recipient, and an erasure of a bcc'd
// subject must serialize against a hold that names them.
func loweredRecipients(in SendEmailInput) []string {
	out := make([]string, 0, len(in.Recipients)+len(in.Cc)+len(in.Bcc))
	for _, group := range [][]string{in.Recipients, in.Cc, in.Bcc} {
		for _, address := range group {
			if trimmed := strings.TrimSpace(address); trimmed != "" {
				out = append(out, strings.ToLower(trimmed))
			}
		}
	}
	return out
}

// anyAddressErased reports whether any of these addresses is on the erasure
// suppression list — the durable record an erasure leaves precisely so that
// re-capture cannot resurrect a subject it has certified destroyed.
//
// READ-ONLY, and this module does not own the table. Whether an address may be
// written to is consent's judgement; whether the installation has been told to
// forget it is a fact this module may look at before storing a message that
// names it.
func anyAddressErased(ctx context.Context, tx pgx.Tx, addresses []string) (bool, error) {
	if len(addresses) == 0 {
		return false, nil
	}
	hashes := make([]string, 0, len(addresses))
	for _, address := range addresses {
		hashes = append(hashes, storekit.SuppressionHash(address))
	}
	var erased bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM erasure_suppression
			 WHERE kind = 'email' AND value_hash = ANY($1))`, hashes).Scan(&erased); err != nil {
		return false, fmt.Errorf("held send: checking whether a recipient has been erased: %w", err)
	}
	return erased, nil
}
