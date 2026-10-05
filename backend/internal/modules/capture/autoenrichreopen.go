// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The auto-enrich half of outage recovery: companies whose deep read failed
// until the attempt bound, so the sweep stopped offering them. ReopenWindow and
// the reasoning are pendingreopen.go's.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// enrichStateObject names the cursor in the audit trail.
const enrichStateObject = "capture_auto_enrich_state"

// parkedEnrichment selects cursors that ran out of attempts on a failed read
// inside the window, for a company nobody has since made a dossier for or
// archived: a read by a human leaves a non-failed site_read, which the sweep
// already treats as done, so reopening it would only queue a duplicate. One
// text for the count, the list and the lock.
const parkedEnrichment = `
	s.attempts >= $1 AND s.last_outcome IN ('exhausted', 'failed')
	AND s.last_attempt_at >= $2 AND s.last_attempt_at < $3
	AND EXISTS (SELECT 1 FROM company o
	             WHERE o.id = s.company_id AND o.archived_at IS NULL AND NOT o.is_anchor)
	AND NOT EXISTS (SELECT 1 FROM site_read sr
	                 WHERE sr.company_id = s.company_id
	                   AND sr.status NOT IN ('failed', 'cancelled'))`

// ParkedEnrichments lists up to limit companies whose enrichment the window
// exhausted, oldest failure first.
func (s *AutoEnrichStore) ParkedEnrichments(ctx context.Context, w ReopenWindow, limit int) ([]ids.UUID, error) {
	var out []ids.UUID
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT s.company_id FROM capture_auto_enrich_state s
			 WHERE `+parkedEnrichment+`
			 ORDER BY s.last_attempt_at, s.company_id
			 LIMIT $4`, autoEnrichMaxAttempts, w.From, w.To, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id ids.UUID
			if err := rows.Scan(&id); err != nil {
				return err
			}
			out = append(out, id)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("capture: listing exhausted auto-enrich cursors in the window: %w", err)
	}
	return out, nil
}

// CountParkedEnrichments is ParkedEnrichments' size, for a dry run.
func (s *AutoEnrichStore) CountParkedEnrichments(ctx context.Context, w ReopenWindow) (int, error) {
	var n int
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM capture_auto_enrich_state s
			 WHERE `+parkedEnrichment, autoEnrichMaxAttempts, w.From, w.To).Scan(&n)
	})
	if err != nil {
		return 0, fmt.Errorf("capture: counting exhausted auto-enrich cursors in the window: %w", err)
	}
	return n, nil
}

// ReopenEnrichment gives one company a fresh allowance, due now, and audits it
// in the same transaction. It reports false when the cursor stopped matching
// between the scan and the lock, which is how a second run reopens nothing.
//
// Audit only, for the reason ReopenParkedTx gives.
func (s *AutoEnrichStore) ReopenEnrichment(ctx context.Context, w ReopenWindow, companyID ids.UUID) (bool, error) {
	reopened := false
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var attempts int
		var outcome string
		err := tx.QueryRow(ctx, `
			SELECT s.attempts, s.last_outcome FROM capture_auto_enrich_state s
			 WHERE s.company_id = $4 AND `+parkedEnrichment+`
			 FOR UPDATE OF s`, autoEnrichMaxAttempts, w.From, w.To, companyID).Scan(&attempts, &outcome)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE capture_auto_enrich_state
			   SET attempts = 0, next_attempt_at = now(), last_outcome = NULL, updated_at = now()
			 WHERE company_id = $1`, companyID); err != nil {
			return err
		}
		if _, err := storekit.AuditWithEvidence(ctx, tx, "update", enrichStateObject, companyID,
			map[string]any{"attempts": attempts, "last_outcome": outcome},
			map[string]any{"attempts": 0, "last_outcome": nil},
			map[string]any{"reopened_window_from": w.From, "reopened_window_to": w.To}); err != nil {
			return err
		}
		reopened = true
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("capture: reopening auto-enrich for company %s: %w", companyID, err)
	}
	return reopened, nil
}
