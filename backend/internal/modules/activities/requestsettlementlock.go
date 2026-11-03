// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// settlementEvidenceKeysSQL names the conversation keys a request's answers
// arrive under: its thread, its sender's addresses and the sender's contact.
// Writers of activities, participants, links and contact addresses take the
// same keys shared (the settlement_evidence_share triggers).
const settlementEvidenceKeysSQL = `SELECT settlement_evidence_key('thread', a.thread_key) FROM activity a
   WHERE a.id = $1 AND a.thread_key IS NOT NULL
 UNION SELECT settlement_evidence_key('address', lower(btrim(a.counterparty_email))) FROM activity a
   WHERE a.id = $1 AND a.counterparty_email IS NOT NULL
 UNION SELECT settlement_evidence_key('address', lower(btrim(p.address))) FROM activity_participant p
   WHERE p.activity_id = $1 AND p.role = 'from' AND p.address IS NOT NULL
 UNION SELECT settlement_evidence_key('address', lower(btrim(e.email))) FROM activity_participant p
   JOIN contact_email e ON e.contact_id = p.contact_id WHERE p.activity_id = $1 AND p.role = 'from'
 UNION SELECT settlement_evidence_key('contact', p.contact_id::text) FROM activity_participant p
   WHERE p.activity_id = $1 AND p.role = 'from' AND p.contact_id IS NOT NULL`

// settlementLockWait bounds each lock wait of a verdict. It is shorter than
// Postgres's default deadlock_timeout of one second. In a lock cycle with a
// writer, the verdict gives up first and the writer commits.
const settlementLockWait = "500ms"

// errSettlementBusy is a verdict that gave up waiting for its conversation. It
// is dropped, and the next pass reads the request again.
var errSettlementBusy = errors.New("activities: the request's conversation is being written")

// holdSettlementEvidenceTx takes a verdict's locks in the writers' order: rows
// first, then the conversation keys. A writer locks its row and then meets the
// key in its trigger. So the verdict locks the request and its answers first,
// then the keys, exclusively and in key order. A writer already holding a key
// finishes first and its change is read. A later one waits for this commit. It
// answers whether the request may still be judged.
func holdSettlementEvidenceTx(ctx context.Context, tx pgx.Tx, request RepliedRequest) (bool, error) {
	var restore string
	if err := tx.QueryRow(ctx, `SELECT current_setting('lock_timeout'), set_config('lock_timeout', $1, true)`,
		settlementLockWait).Scan(&restore, new(string)); err != nil {
		return false, fmt.Errorf("activities: bounding the verdict's lock wait: %w", err)
	}
	live, err := requestStillWritable(ctx, tx, request.RequestID)
	if err == nil && live {
		// The request row sits inside the IN, so the answers are found once and
		// each is locked by its key. Beside seen, it made Postgres walk every
		// activity and find the answers again for each one.
		_, err = tx.Exec(ctx, `SELECT seen.id FROM activity seen
		  WHERE seen.id IN (SELECT answered.id FROM activity a
		    CROSS JOIN LATERAL (`+settlementAnswersSQL()+`) answered WHERE a.id = $2)
		  FOR SHARE OF seen`, request.AsOf, request.RequestID)
	}
	if err == nil && live {
		_, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(k) FROM (`+settlementEvidenceKeysSQL+`) keys(k) ORDER BY k`, request.RequestID)
	}
	if err != nil {
		return false, settlementWaitFailed(err)
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('lock_timeout', $1, true)`, restore); err != nil {
		return false, fmt.Errorf("activities: restoring the lock wait after the verdict's locks: %w", err)
	}
	return live, nil
}

// settlementWaitFailed names a lock wait the verdict gave up on as busy. A
// deadlock is not mapped: the lock order and the bound above leave none, so one
// is a fault to report.
func settlementWaitFailed(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.LockNotAvailable {
		return errSettlementBusy
	}
	return fmt.Errorf("activities: locking the request's evidence: %w", err)
}

// requestStillWritable re-tests, at write time, that this pass may still
// record a judgement about the request. SetOwedVerdict asks the same in its
// own update. This write lands in another table, so it asks outright. Without
// it, a settlement that finished after an erasure would write about a subject
// the cascade already swept. `FOR NO KEY UPDATE` lets a writer add a
// participant or link under the request while the verdict waits.
func requestStillWritable(ctx context.Context, tx pgx.Tx, requestID ids.UUID) (bool, error) {
	var live bool
	err := tx.QueryRow(ctx, `
		SELECT true FROM activity
		 WHERE id = $1 AND archived_at IS NULL
		   AND audience = 'workspace' AND restricted_at IS NULL
		   FOR NO KEY UPDATE`, requestID).Scan(&live)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("activities: re-reading the request before settling it: %w", err)
	}
	return live, nil
}

// lostSettlementBasisSQL is a settled verdict that recorded its evidence and
// has no answer left the pass may read: the evidence it rested on is gone.
var lostSettlementBasisSQL = `judged.verdict = '` + RequestSettled + `' AND judged.judged_evidence IS NOT NULL
 AND NOT EXISTS (` + settlementAnswersSQL() + `)
 AND judged.judged_evidence IS DISTINCT FROM (` + settlementEvidenceSQL() + `)`

// WithdrawLostSettlements turns a settled verdict whose answers are all gone
// into an unsure one. Absence of an answer is no proof of anything either way,
// so the request stops being hidden as answered and nothing is reopened.
func (s *Store) WithdrawLostSettlements(ctx context.Context, asOf time.Time, limit int) (int, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalSystem {
		return 0, apperrors.ErrPermissionDenied
	}
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return 0, err
	}
	var found []lostSettlement
	if err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT judged.request_activity_id, judged.judged_through_activity_id
			  FROM activity_request_settlement judged JOIN activity a ON a.id = judged.request_activity_id
			 WHERE a.restricted_at IS NULL AND `+lostSettlementBasisSQL+` ORDER BY judged.request_activity_id LIMIT $2`, asOf, limit)
		if err != nil {
			return fmt.Errorf("activities: reading settled verdicts that lost their evidence: %w", err)
		}
		found, err = pgx.CollectRows(rows, pgx.RowToStructByPos[lostSettlement])
		return err
	}); err != nil {
		return 0, err
	}
	withdrawn := 0
	for _, l := range found {
		done, err := s.withdrawLostSettlement(ctx, l, asOf)
		if err != nil {
			return withdrawn, err
		}
		if done {
			withdrawn++
		}
	}
	return withdrawn, nil
}

// lostSettlement is a settled verdict read as having lost its evidence.
type lostSettlement struct{ Request, Through ids.UUID }

// withdrawLostSettlement records one lost verdict as unsure in its own
// transaction, after rechecking it under the verdict's locks.
func (s *Store) withdrawLostSettlement(ctx context.Context, l lostSettlement, asOf time.Time) (bool, error) {
	request := RepliedRequest{RequestID: l.Request, NewestAnswerID: l.Through, AsOf: asOf}
	withdrawn := false
	err := s.tx(ctx, func(tx pgx.Tx) error {
		live, err := holdSettlementEvidenceTx(ctx, tx, request)
		if err != nil || !live {
			return err
		}
		var evidence *string
		var still bool
		if err := tx.QueryRow(ctx, `SELECT `+lostSettlementBasisSQL+`, (`+settlementEvidenceSQL()+`)
			  FROM activity_request_settlement judged JOIN activity a ON a.id = judged.request_activity_id
			 WHERE judged.request_activity_id = $2 FOR UPDATE OF judged`, asOf, l.Request).Scan(&still, &evidence); err != nil {
			return fmt.Errorf("activities: rechecking a settled verdict that lost its evidence: %w", err)
		}
		if !still {
			return nil
		}
		if evidence != nil {
			request.Evidence = *evidence
		}
		withdrawn = true
		return recordSettlement(ctx, tx, RequestSettlementInput{Request: request, Verdict: RequestUnsure, DecidedBy: settlementBasisLost})
	})
	if errors.Is(err, errSettlementBusy) {
		return false, nil
	}
	return withdrawn && err == nil, err
}

// settlementBasisLost names the withdrawal as the decider of its verdict.
const settlementBasisLost = "evidence_withdrawn"
