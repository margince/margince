// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Mail that arrived at an address before the seat was known to hold it.
//
// A message is imported for a seat only when one of that seat's own addresses
// is on it (mailboxWasARecipientTx). Alias discovery learns a forwarding
// address from the mail arriving at it, which means there is always mail that
// landed BEFORE the claim — and for that mail the seat is nobody. It gets no
// import row, so no confidentiality question is opened for its thread, so under
// a classified mailbox it stays held forever with nothing scheduled to judge
// it. Held looks exactly like broken.
//
// That is not a corner: a founder's private clinic wrote twice, and the second
// thread reached a forwarding address claimed ninety seconds after the message
// was captured. The thread was never judged, the retraction read "not judged"
// as "business", and the clinic kept its contact in the shared CRM.
//
// Adoption writes the import row the message should have had. It does not
// re-decide anything else: the audience recompute derives the rest from every
// contributor once the row exists.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// adoptBatch bounds one pass. The population only shrinks — every adopted
// message leaves it — so a small bound costs one probe a tick once it is empty
// and keeps a large mailbox's backlog from holding a transaction open.
const adoptBatch = 200

// adoptableMessage is one message a seat should have imported and did not.
type adoptableMessage struct {
	id           ids.ActivityID
	threadKey    string
	counterparty string
	direction    string
}

// adoptMailForNewIdentityTx is the sink's door onto adoption: it resolves the
// provider from the acting connector principal, adopts, and recomputes each
// adopted message's audience.
//
// The recompute is what turns an import row into a visible change. Without it
// the row exists and every reader still sees the audience the message was born
// with — which for a shared mailbox is the difference between mail the seat can
// read and mail that stays held for no reason anybody could name.
//
// A nil recomputer adopts anyway. That is a role that derives no audiences at
// all (the same nil the sink's own captures honour), and refusing to write the
// import row because of it would leave the mail unimported forever rather than
// merely unrecomputed.
func (s *Sink) adoptMailForNewIdentityTx(
	ctx context.Context, tx pgx.Tx, seat ids.UUID, address string,
) error {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return nil
	}
	provider := strings.TrimPrefix(actor.ID, "connector:")
	if provider == "" || provider == actor.ID {
		// Not a connector principal, so there is no captured mail of this
		// seat's to adopt through this door.
		return nil
	}
	adopted, err := adoptHeldMailForIdentityTx(ctx, tx, seat, provider, address)
	if err != nil || s.recomputeAudience == nil {
		return err
	}
	for _, id := range adopted {
		if err := s.recomputeAudience(ctx, tx, id); err != nil {
			return err
		}
	}
	return nil
}

// adoptHeldMailForIdentityTx imports the messages already on the timeline that
// one of this seat's addresses is on, and which the seat has no import row for.
//
// Called when an address BECOMES the seat's own — discovered by alias sighting,
// or declared by the contact themselves. Both are the same event as far as the
// mail is concerned: an address the product did not know was theirs now is.
//
// Returns the messages adopted, so the caller can recompute their audience.
// The recompute is the caller's because it belongs to another module, and
// running it inside this transaction is what keeps the import row and the
// audience it implies from disagreeing.
func adoptHeldMailForIdentityTx(
	ctx context.Context, tx pgx.Tx, seat ids.UUID, provider, address string,
) ([]ids.ActivityID, error) {
	if seat == ids.Nil || provider == "" || address == "" {
		return nil, nil
	}
	candidates, err := adoptableMessagesTx(ctx, tx, seat, provider, address)
	if err != nil || len(candidates) == 0 {
		return nil, err
	}
	// The posture is read ONCE for the batch: it is a property of the mailbox,
	// not of any message, and re-reading it per message would let a posture
	// changed mid-batch import half a backlog under each answer.
	posture, err := mailboxPostureForTx(ctx, tx, seat, provider)
	if err != nil {
		return nil, err
	}
	adopted := make([]ids.ActivityID, 0, len(candidates))
	for _, msg := range candidates {
		if err := adoptOneMessageTx(ctx, tx, seat, posture, msg); err != nil {
			return nil, err
		}
		adopted = append(adopted, msg.id)
	}
	return adopted, nil
}

// adoptableMessagesTx lists the mail this seat's own connector captured, which
// names the address, and which the seat never imported.
//
// Bounded to messages THIS SEAT'S connector landed. An address becoming mine
// says nothing about mail that reached the workspace through somebody else's
// mailbox, and importing a colleague's capture on the strength of a header
// would hand this seat a grant on their correspondence.
//
// A restricted message is skipped rather than adopted: a row under a statutory
// obligation is out of every ordinary path, and an import row against one would
// give a seat a grant the obligation has already taken away. Same rule
// seatDeliveredTx applies at capture time.
func adoptableMessagesTx(
	ctx context.Context, tx pgx.Tx, seat ids.UUID, provider, address string,
) ([]adoptableMessage, error) {
	rows, err := tx.Query(ctx, `
		SELECT a.id, COALESCE(a.thread_key, ''), COALESCE(a.counterparty_email, ''),
		       COALESCE(a.direction, '')
		  FROM activity a
		  JOIN activity_participant p ON p.activity_id = a.id
		 WHERE p.address = $3
		   AND a.kind = 'email'
		   AND a.captured_by = $2
		   AND a.archived_at IS NULL
		   AND a.restricted_at IS NULL
		   AND NOT EXISTS (
		         SELECT 1 FROM capture_import i
		          WHERE i.activity_id = a.id AND i.user_id = $1)
		 ORDER BY a.occurred_at DESC
		 LIMIT $4`,
		seat, "connector:"+provider+":"+seat.String(), address, adoptBatch)
	if err != nil {
		return nil, fmt.Errorf("capture: listing the mail an alias claim adopts: %w", err)
	}
	defer rows.Close()
	var out []adoptableMessage
	for rows.Next() {
		var m adoptableMessage
		if err := rows.Scan(&m.id, &m.threadKey, &m.counterparty, &m.direction); err != nil {
			return nil, fmt.Errorf("capture: reading the mail an alias claim adopts: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("capture: listing the mail an alias claim adopts: %w", err)
	}
	return out, nil
}

// adoptOneMessageTx writes the import row this message should have had, and
// opens the confidentiality question when the mailbox owes one.
//
// WHAT VERDICT STATUS TO WRITE is the whole decision here, and the wrong answer
// is worse than none. `pending` renders as HELD, and a question is only ever
// opened for a classified mailbox — so writing `pending` on a shared mailbox
// would hold the message forever with nothing scheduled to free it, which is
// the exact defect adoption exists to fix, moved to a different mailbox.
//
//   - A thread this seat has already settled: inherit that answer. The message
//     is part of a conversation the classifier read, and re-asking would spend
//     a model call to reach the answer already on the ledger.
//   - Classified, nothing settled, and the message HAS a thread: `pending`, and
//     open the question. That is what a message captured under this posture
//     would have got. A message with no thread key gets no status: EnsureTx has
//     no conversation to open a question about, and a pending row nothing can
//     resolve holds the mail forever.
//   - Anything else (shared, held): write no status and let the posture speak.
//     A held mailbox holds it, a shared one opens it, and neither needs a
//     verdict to say so.
func adoptOneMessageTx(
	ctx context.Context, tx pgx.Tx, seat ids.UUID, posture string, msg adoptableMessage,
) error {
	inherited, err := settledVerdictForTx(ctx, tx, seat, msg.threadKey, msg.counterparty)
	if err != nil {
		return err
	}
	// `pending` is written ONLY where a question is actually opened, and the
	// two are decided together on purpose. A pending status renders as HELD, so
	// a row carrying one that no question will resolve is mail held forever
	// with nothing scheduled to free it — the defect adoption exists to end.
	// The thread-key check is DEFENCE rather than a live case: mailmap falls
	// back to a message's own Message-ID, so mail arriving through a connector
	// always carries one. It is here because EnsureTx no-ops silently on an
	// empty key — a future transport that produces one would strand every
	// message it adopted, with no failing assertion anywhere to say so.
	status := inherited
	openQuestion := status == "" && posture == PostureClassified && msg.threadKey != ""
	if openQuestion {
		status = VerdictPending
	}
	if err := recordImportTx(ctx, tx, msg.id, seat, birthDecision{
		posture:       posture,
		verdictStatus: status,
	}); err != nil {
		return err
	}
	// The seat's own participant row, so they can actually read what they now
	// hold. A message adopted with no participant row is imported for somebody
	// the audience derivation cannot see.
	if err := stampCaptureParticipants(ctx, tx, msg.id, seat, kindEmail, msg.direction,
		connector.Counterparty{Email: msg.counterparty}); err != nil {
		return err
	}
	if !openQuestion {
		return nil
	}
	return (&ThreadVerdictStore{}).EnsureTx(ctx, tx, msg.threadKey, seat, msg.id.UUID, time.Now())
}

// settledVerdictForTx is the READ half of inheritedVerdictTx, for a message
// already on the timeline rather than one arriving.
//
// It deliberately does NOT re-open a cleared thread an unseen sender wrote on.
// That re-opening is a reaction to a NEW party joining a conversation; this
// message was already there, judged or not, and re-opening on it would spend a
// model call to re-answer a question about mail the seat has had all along. An
// opening answer it cannot inherit becomes no answer, and the posture decides.
func settledVerdictForTx(
	ctx context.Context, tx pgx.Tx, seat ids.UUID, threadKey, counterparty string,
) (string, error) {
	if threadKey == "" {
		return "", nil
	}
	var status string
	var seen []string
	err := tx.QueryRow(ctx, `
		SELECT status, seen_addresses FROM capture_thread_verdict
		 WHERE thread_key = $1 AND user_id = $2`, threadKey, seat).Scan(&status, &seen)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("capture: reading a thread's settled verdict: %w", err)
	}
	switch status {
	case VerdictHeld, VerdictUnsure, VerdictHeldByOwner:
		return status, nil
	case VerdictCleared, VerdictSharedByOwner:
		if addressWasSeen(counterparty, seen) {
			return status, nil
		}
	}
	return "", nil
}

// StrandedSeatMail is one mailbox holding mail it never imported: the seat, its
// provider, and one of that seat's own addresses the mail names.
type StrandedSeatMail struct {
	Seat     ids.UUID
	Provider string
	Address  string
}

// AdoptStrandedMail imports the mail this workspace's seats hold at addresses
// they already own but never claimed in time.
//
// The claim-time path (adoptMailForNewIdentityTx) covers everything from the
// moment it shipped. This covers what was already stuck, and there is a lot of
// it: an address is only ever discovered FROM arriving mail, so every mailbox
// that gained an alias has messages older than the claim. One founder's mailbox
// held forty-nine, every one of them classified-and-held with nothing scheduled
// to judge it.
//
// It derives its work from state rather than from a list of known-bad rows, so
// it also catches mail stranded by any other route to the same condition — a
// declared address, a re-connected mailbox, a capture that failed halfway.
//
// One transaction per seat-address pair: a mailbox whose adoption fails costs
// that mailbox and not the sweep. The recompute runs inside it, so an import
// row and the audience it implies cannot commit apart.
func (s *PendingStore) AdoptStrandedMail(
	ctx context.Context, recompute func(context.Context, pgx.Tx, ids.ActivityID) error, limit int,
) (int, error) {
	pairs, err := s.strandedSeats(ctx, limit)
	if err != nil || len(pairs) == 0 {
		return 0, err
	}
	adopted := 0
	var failed error
	for _, pair := range pairs {
		n, err := s.adoptOnePair(ctx, pair, recompute)
		if err != nil {
			failed = errors.Join(failed, fmt.Errorf("adopting %s's mail at %s: %w", pair.Seat, pair.Address, err))
			continue
		}
		adopted += n
	}
	return adopted, failed
}

// strandedSeats names the seat-address pairs with mail to adopt.
//
// The pair, not the message: adoption reads one mailbox's posture once and
// walks its backlog, so handing it a message at a time would re-read the
// posture per row and could import one backlog under two different answers.
func (s *PendingStore) strandedSeats(ctx context.Context, limit int) ([]StrandedSeatMail, error) {
	var out []StrandedSeatMail
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT DISTINCT oi.user_id, c.provider, oi.value
			  FROM capture_owner_identity oi
			  JOIN capture_connection c ON c.user_id = oi.user_id AND c.archived_at IS NULL
			  JOIN activity_participant p ON p.address = oi.value
			  JOIN activity a ON a.id = p.activity_id
			 WHERE oi.kind = $1
			   AND a.kind = 'email'
			   AND a.captured_by = 'connector:' || c.provider || ':' || oi.user_id
			   AND a.archived_at IS NULL
			   AND a.restricted_at IS NULL
			   AND NOT EXISTS (
			         SELECT 1 FROM capture_import i
			          WHERE i.activity_id = a.id AND i.user_id = oi.user_id)
			 LIMIT $2`, IdentityKindAddress, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var pair StrandedSeatMail
			if err := rows.Scan(&pair.Seat, &pair.Provider, &pair.Address); err != nil {
				return err
			}
			out = append(out, pair)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("capture: listing the mailboxes holding unimported mail: %w", err)
	}
	return out, nil
}

// adoptOnePair adopts one seat's stranded mail at one of its addresses.
func (s *PendingStore) adoptOnePair(
	ctx context.Context, pair StrandedSeatMail,
	recompute func(context.Context, pgx.Tx, ids.ActivityID) error,
) (int, error) {
	var adopted int
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		ids, err := adoptHeldMailForIdentityTx(ctx, tx, pair.Seat, pair.Provider, pair.Address)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if recompute == nil {
				continue
			}
			if err := recompute(ctx, tx, id); err != nil {
				return err
			}
		}
		adopted = len(ids)
		return nil
	})
	return adopted, err
}
