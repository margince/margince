// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The thread confidentiality ledger: one open question per thread per seat, and
// the answers that close it.
//
// Per seat, not per thread, for the reason capture_import exists: a thread
// reaching two mailboxes is two people's correspondence, and each may conclude
// differently about it. The audience derivation takes the strictest.
//
// This is the WRITE side of what verdictinherit.go reads. That file states the
// contract this one has to satisfy — an opening answer is inherited only by a
// sender the verdict actually saw, a holding answer is inherited from anyone —
// so read it before changing what this writes into seen_addresses or status.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// ThreadVerdictMaxAttempts bounds the retries on one thread. It matches the
// sender ledger's bound deliberately: both are "a model was asked and could not
// answer", and two different ceilings would be two different stories about the
// same kind of giving up.
const ThreadVerdictMaxAttempts = PendingMaxAttempts

// threadVerdictLease is how long a claimed thread stays off other workers'
// scans, matching the sender ledger's for the same reason.
const threadVerdictLease = pendingLease

// PendingThread is one thread awaiting a confidentiality answer, with the text
// the classifier is allowed to see.
//
// Subject and body come from the thread's FIRST message and are capped at the
// same lengths the sender verdict uses. The whole thread is not sent: a
// classifier deciding whether a conversation is confidential does not need
// every reply on it, and every extra message is more of somebody's mail leaving
// the row it lives in.
type PendingThread struct {
	ID         ids.UUID
	ThreadKey  string
	UserID     ids.UUID
	ActivityID ids.UUID
	Subject    string
	Body       string
	// Attachments are the filenames on the first message. A name can carry the
	// answer on its own — `Aufhebungsvertrag_final.pdf` is a termination
	// agreement whatever the covering note says.
	Attachments []string
	// Claim is the lease token this row was taken under. Every write back to
	// the row carries it, so a worker that outran its lease cannot overwrite an
	// answer the row's new holder has since written.
	Claim ids.UUID
}

// ThreadVerdictStore is the ledger's write side.
type ThreadVerdictStore struct {
	db *database.DB
}

// NewThreadVerdictStore builds the store over the pool.
func NewThreadVerdictStore(db *database.DB) *ThreadVerdictStore {
	return &ThreadVerdictStore{db: db}
}

// EnsureTx opens the question for one seat's view of one thread, and does
// nothing when it is already open or already answered.
//
// Called from inside the capture transaction under the connector principal, so
// a message that lands is either on the ledger or not captured at all — there
// is no window in which mail exists with nobody scheduled to judge it. That is
// also why it carries no auth gate of its own: it is reached only from the sink,
// which took the grant, and the exception is ratified in the RBAC gate.
//
// The conflict clause writes in exactly one case: a row that is pending and has
// no message to judge. That is the state a reopen leaves behind — it clears the
// pointer so the re-ask cannot read the text a previous answer already covered
// — and this is what supplies the new one.
//
// It does NOT re-open an answered thread. A settled row fails the WHERE and is
// left alone: re-opening is a deliberate act, taken where that decision is made
// (an unseen sender on an opening verdict, or a confidential marker), never a
// side effect of the next message arriving.
func (s *ThreadVerdictStore) EnsureTx(
	ctx context.Context, tx pgx.Tx, threadKey string, user ids.UUID, firstActivity ids.UUID, due time.Time,
) error {
	if threadKey == "" || user == ids.Nil {
		// A message with no thread key is judged by its own posture; there is
		// no conversation for a verdict to be about.
		return nil
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO capture_thread_verdict (thread_key, user_id, first_activity_id, status, next_attempt_at)
		VALUES ($1, $2, $3, 'pending', $4)
		ON CONFLICT (thread_key, user_id) DO UPDATE
		   SET first_activity_id = EXCLUDED.first_activity_id,
		       next_attempt_at = EXCLUDED.next_attempt_at,
		       updated_at = now()
		 WHERE capture_thread_verdict.status = 'pending'
		   AND capture_thread_verdict.first_activity_id IS NULL`,
		threadKey, user, firstActivity, due)
	if err != nil {
		return fmt.Errorf("capture: opening the confidentiality question for a thread: %w", err)
	}
	return nil
}

// ClaimDue takes up to limit threads whose next attempt has come round.
//
// The attempt is charged AT CLAIM, not at answer, which is what makes a worker
// that dies mid-call cost one attempt rather than nothing: the bound is a
// property of the row, so a thread whose content reliably kills the verdict
// step retires instead of being re-claimed at every lease expiry forever.
func (s *ThreadVerdictStore) ClaimDue(ctx context.Context, limit int) ([]PendingThread, error) {
	claim := ids.NewV7()
	var out []PendingThread
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			UPDATE capture_thread_verdict v
			   SET attempts = v.attempts + 1,
			       claimed_until = now() + make_interval(secs => $2),
			       claimed_by = $4,
			       updated_at = now()
			 WHERE v.id IN (
			   SELECT id FROM capture_thread_verdict
			    WHERE status = 'pending'
			      AND next_attempt_at IS NOT NULL AND next_attempt_at <= now()
			      AND (claimed_until IS NULL OR claimed_until <= now())
			      AND attempts < $3
			    ORDER BY next_attempt_at
			    LIMIT $1
			    FOR UPDATE SKIP LOCKED)
			RETURNING v.id, v.thread_key, v.user_id, v.first_activity_id,
			          coalesce(left((SELECT a.subject FROM activity a
			                          WHERE a.id = v.first_activity_id AND a.restricted_at IS NULL), $5), ''),
			          coalesce(left((SELECT a.body FROM activity a
			                          WHERE a.id = v.first_activity_id AND a.restricted_at IS NULL), $6), ''),
			          coalesce((SELECT array_agg(at.filename ORDER BY at.filename)
			                      FROM attachment at
			                     WHERE at.entity_type = 'activity' AND at.entity_id = v.first_activity_id
			                       AND at.archived_at IS NULL), '{}')`,
			limit, threadVerdictLease.Seconds(), ThreadVerdictMaxAttempts, claim,
			MaxCapturedSubjectChars, MaxCapturedBodyChars)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			t := PendingThread{Claim: claim}
			var activity *ids.UUID
			if err := rows.Scan(&t.ID, &t.ThreadKey, &t.UserID, &activity,
				&t.Subject, &t.Body, &t.Attachments); err != nil {
				return err
			}
			if activity != nil {
				t.ActivityID = *activity
			}
			out = append(out, t)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("capture: claiming threads awaiting a confidentiality verdict: %w", err)
	}
	return out, nil
}

// ResolveAs closes the question, recording the kind and the addresses the
// verdict saw.
//
// seen is what binds a later message to this answer: verdictinherit.go admits
// an OPENING verdict only for a sender in this list. Writing it is therefore
// not bookkeeping — an empty list on a cleared thread means every later message
// re-opens it, and a list containing an address the classifier did not actually
// see is a hole.
//
// Guarded by the claim, so a worker that outran its lease cannot overwrite the
// answer its successor wrote.
func (s *ThreadVerdictStore) ResolveAs(
	ctx context.Context, tx pgx.Tx, t PendingThread, status, kind string, confidence float64, seen []string, reason string,
) (bool, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE capture_thread_verdict
		   SET status = $2, kind = NULLIF($3, ''), confidence = $4,
		       seen_addresses = $5, disposition_reason = NULLIF($6, ''),
		       resolved_at = now(), next_attempt_at = NULL,
		       claimed_until = NULL, claimed_by = NULL, updated_at = now()
		 WHERE id = $1 AND status = 'pending' AND claimed_by = $7`,
		t.ID, status, kind, confidence, seen, reason, t.Claim)
	if err != nil {
		return false, fmt.Errorf("capture: resolving the confidentiality verdict for a thread: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// Defer returns a claimed thread to the queue for a later pass.
//
// refundAttempt says whether this cause reached a model, and the distinction is
// the same one the sender ledger draws: a budget stop or an outage never asked
// the question, so charging for it would let two quiet cycles exhaust a
// thread's allowance and retire it to `unsure` for no reason but the workspace
// running out of budget. A reply the validator rejected DID reach a model and
// its cause is the message itself, which an outsider writes.
func (s *ThreadVerdictStore) Defer(
	ctx context.Context, t PendingThread, backoff time.Duration, reason string, refundAttempt bool,
) error {
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE capture_thread_verdict
			   SET attempts = CASE WHEN $4 THEN greatest(attempts - 1, 0) ELSE attempts END,
			       next_attempt_at = now() + make_interval(secs => $2),
			       disposition_reason = NULLIF($3, ''),
			       claimed_until = NULL, claimed_by = NULL, updated_at = now()
			 WHERE id = $1 AND status = 'pending' AND claimed_by = $5`,
			t.ID, backoff.Seconds(), reason, refundAttempt, t.Claim)
		return err
	})
	if err != nil {
		return fmt.Errorf("capture: deferring the confidentiality verdict for a thread: %w", err)
	}
	return nil
}

// RetireExhausted ends the threads that spent every attempt without an answer,
// moving them to `unsure` — which HOLDS, because a thread nothing could judge
// is exactly the one not to publish on a guess.
//
// A terminal state, reached by spending the attempts rather than by a second
// spelling of "give up".
func (s *ThreadVerdictStore) RetireExhausted(ctx context.Context, reason string) (int64, error) {
	var retired int64
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE capture_thread_verdict
			   SET status = 'unsure', disposition_reason = NULLIF($1, ''),
			       resolved_at = now(), next_attempt_at = NULL,
			       claimed_until = NULL, claimed_by = NULL, updated_at = now()
			 WHERE status = 'pending' AND attempts >= $2
			   AND (claimed_until IS NULL OR claimed_until <= now())`,
			reason, ThreadVerdictMaxAttempts)
		if err != nil {
			return err
		}
		retired = tag.RowsAffected()
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("capture: retiring threads that exhausted their verdict attempts: %w", err)
	}
	return retired, nil
}

// RecordOutcomeTx writes this seat's answer onto its own import row for the
// message the verdict was about.
//
// It lives here rather than in the engine because capture owns capture_import.
// It takes the caller's transaction, which is the one ResolveAs just wrote the
// ledger row on: both land together or neither does.
//
// The MESSAGE the model actually read, not every message on the thread. The
// claim hands the classifier the thread's first message only, so opening the
// whole thread would clear correspondence nobody looked at: a routine opening
// message can be followed by a termination agreement before the pass runs, or
// while the model call is in flight, and an answer about the first message says
// nothing about those. A later message inherits through inheritedVerdictTx
// instead, which admits an opening verdict only for a sender the verdict saw.
//
// Scoped to the seat whose verdict it is. A thread reaching two mailboxes is
// two people's correspondence, each may conclude differently, and the
// derivation takes the strictest — so a stamp that ignored user_id would let
// one seat's answer publish a message their colleague's mailbox is holding.
//
// The KIND travels with the status as the row's reason, so a held message says
// what held it. Without it the derivation falls back to a generic `verdict` and
// `personnel`, `legal` and `security_incident` never reach the row a reader
// sees.
func (s *ThreadVerdictStore) RecordOutcomeTx(
	ctx context.Context, tx pgx.Tx, row PendingThread, status, kind string,
) error {
	if row.ActivityID == ids.Nil {
		// The message was erased while the question stood. The verdict is still
		// worth recording on the ledger for the threads that inherit from it.
		return nil
	}
	_, err := tx.Exec(ctx, `
		UPDATE capture_import
		   SET verdict_status = $3, verdict_reason = NULLIF($4, '')
		 WHERE activity_id = $1 AND user_id = $2`,
		row.ActivityID, row.UserID, status, kind)
	if err != nil {
		return fmt.Errorf("capture: recording a thread verdict on its import row: %w", err)
	}
	return nil
}

// openConfidentialityQuestionTx schedules the thread this message belongs to
// for a confidentiality answer, for the seat that just imported it.
//
// Held messages only, and only where the MAILBOX POSTURE is what holds them.
// A `held` mailbox is never asked: its owner said "hold this whatever a
// classifier concludes", and the derivation evaluates a verdict before a
// posture, so a cleared answer would override the strongest privacy setting the
// product offers. The other holds — a workspace floor, a counterparty hold, a
// confidential marker — hold on their own authority and are not a classifier's
// to lift either.
//
// An already-open message is not asked about at all: there is nothing for a
// verdict to open, and a `shared` mailbox would spend one model call per
// message to be told what its posture already said.
//
// Due immediately: the engine's own claim scan paces the work, and a delay here
// would be a second, quieter place where the cadence is decided.
func openConfidentialityQuestionTx(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, owner ids.UUID,
	rec connector.NormalizedRecord, fields ActivityFields, birth birthDecision,
) error {
	if fields.Kind != "email" {
		// A meeting or a channel message is not correspondence a
		// confidentiality classifier was ever asked about.
		return nil
	}
	// Checked on the posture rather than the derived audience, because `held`
	// and `classified` collapse to the same audience and only the posture can
	// tell them apart.
	if birth.posture != PostureClassified {
		return nil
	}
	if audience, _ := birth.bornAudience(); audience != audienceParticipants {
		return nil
	}
	// A zero-value store: EnsureTx runs entirely on the caller's transaction and
	// touches no pool, which is what lets the capture path and the engine share
	// one spelling of what opening a question means.
	return (&ThreadVerdictStore{}).EnsureTx(ctx, tx, rec.ThreadKey, owner, id.UUID, time.Now())
}
