// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Leasing a due sender out to one worker.
//
// Its own file because the claim is where the row becomes a QUESTION rather
// than a record: everything the judgment needs travels out of here — the
// message's text, and the direction it went. A field missing from this read is
// a field the model is never told about, and the last one that was missing let
// a mailbox the owner had written TO be judged as a stranger writing in.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ClaimDue atomically leases up to limit due rows for this workspace. FOR UPDATE
// SKIP LOCKED lets several replicas drain the ledger without double-judging a
// row or serializing on each other; the lease is what a crashed worker releases
// by expiry.
//
// Claiming bumps attempts, so a row that keeps failing walks toward its bound
// rather than being retried forever, and stamps a fresh claim token every
// batch shares — the key Resolve and Defer demand back.
func (s *PendingStore) ClaimDue(ctx context.Context, limit int) ([]PendingCounterparty, error) {
	claim := ids.NewV7()
	var out []PendingCounterparty
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			UPDATE capture_pending_counterparty p
			   SET attempts = p.attempts + 1,
			       claimed_until = now() + make_interval(secs => $2),
			       claimed_by = $4,
			       updated_at = now()
			 WHERE p.id IN (
			   SELECT id FROM capture_pending_counterparty
			    WHERE status = 'pending'
			      AND next_attempt_at IS NOT NULL AND next_attempt_at <= now()
			      AND (claimed_until IS NULL OR claimed_until <= now())
			      -- The bound is a property of the ROW, not of a live worker.
			      -- A worker that crashes, is killed, or outruns its lease never
			      -- reaches Defer, so a row whose content reliably kills the
			      -- verdict step would otherwise be re-claimed every lease
			      -- expiry forever, at one model call a time.
			      AND attempts < $3
			    ORDER BY next_attempt_at
			    LIMIT $1
			    FOR UPDATE SKIP LOCKED)
			RETURNING p.id, p.email, coalesce(p.domain, ''), coalesce(left(p.display_name, $5), ''),
			          p.activity_id, p.owner_id,
			          coalesce(left((SELECT a.subject FROM activity a WHERE a.id = p.activity_id AND a.restricted_at IS NULL), $6), ''),
			          coalesce(left((SELECT a.body FROM activity a WHERE a.id = p.activity_id AND a.restricted_at IS NULL), $7), ''),
			          coalesce((SELECT a.direction FROM activity a WHERE a.id = p.activity_id), '')`,
			limit, pendingLease.Seconds(), PendingMaxAttempts, claim,
			MaxCapturedNameChars, MaxCapturedSubjectChars, MaxCapturedBodyChars)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			p := PendingCounterparty{Claim: claim}
			if err := rows.Scan(&p.ID, &p.Email, &p.Domain, &p.DisplayName,
				&p.ActivityID, &p.OwnerID, &p.Subject, &p.Body, &p.Direction); err != nil {
				return err
			}
			out = append(out, p)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		// Whether each address has ever answered us, read in the SAME
		// transaction that claimed the rows.
		//
		// After the loop rather than as a subselect: wroteBackTx is the one
		// spelling of "this address wrote back" in this package, and inlining
		// its query here would be the second copy — the two would then disagree
		// the first time either learned about a new kind of thread.
		for i := range out {
			replied, err := wroteBackTx(ctx, tx, out[i].Email)
			if err != nil {
				return fmt.Errorf("capture: reading whether %s ever answered: %w", out[i].Domain, err)
			}
			out[i].WroteBack = replied
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("capture: claiming due dispositions: %w", err)
	}
	return out, nil
}
