// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The capture-classify label columns (ADR-0063) belong to the activity
// row, so their reads and writes live here — the classify engine consumes
// these two methods and never touches activity SQL itself. A label routes
// attention only: setting one changes NOTHING else on the row, mints no
// audit entry and no event (§3.2 — a noise label deleting or archiving
// anything is a hard-floor violation).

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// UnlabeledEmail is one classify-backlog row as the prompt consumes it.
type UnlabeledEmail struct {
	ID      ids.UUID
	Subject string
	Body    string // pre-truncated to bodyLimit
}

// UnlabeledCaptureEmails reads the oldest connector-captured emails not
// yet labeled — the partial-index backlog (idx_activity_unlabeled).
//
// Mail from a sender whose disposition is still open is left out (ADR-0072 §5).
// Labeling routes attention, and a message from someone the workspace has not
// yet decided is a counterparty at all should not be competing for attention —
// least of all when the pending question may resolve to `noise` and hide it. It
// re-enters the backlog by itself once the verdict lands, because this is a
// query over live state rather than a queue anything has to remember to refill.
func (s *Store) UnlabeledCaptureEmails(ctx context.Context, limit, bodyLimit int) ([]UnlabeledEmail, error) {
	var out []UnlabeledEmail
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, coalesce(subject, ''), coalesce(left(body, $1), '')
			FROM activity
			WHERE `+ClassifyBacklogPredicate+`
			ORDER BY occurred_at
			LIMIT $2`, bodyLimit, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var m UnlabeledEmail
			if err := rows.Scan(&m.ID, &m.Subject, &m.Body); err != nil {
				return err
			}
			out = append(out, m)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("activities: reading the classify backlog: %w", err)
	}
	return out, nil
}

// SetCaptureLabel writes one verdict, reporting whether it applied. The
// capture_label IS NULL predicate is the CAS: a concurrent pass that
// labeled the row first wins and this write reports applied=false — the
// earlier verdict stands, never an overwrite.
func (s *Store) SetCaptureLabel(ctx context.Context, id ids.UUID, label string) (applied bool, err error) {
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		// All THREE exclusions are re-tested at WRITE time, not only in the
		// backlog that selected this row. The classifier reads a batch, spends a
		// model call per message and writes the answers back, and the row can
		// change inside that window.
		//
		// The archive: a noise disposition or the retention sweep takes the
		// message off the timeline, and a label landing afterwards puts a
		// hidden message back on a shared worklist. Nothing would clear it —
		// the backlog excludes archived rows, so no later pass revisits one.
		//
		// The audience: a human or a verdict limits the message, and without
		// this clause the write lands after the narrowing and re-labels a
		// message whose text the worklist's readers may no longer open —
		// nothing would clear it a second time, because the narrowing already
		// ran.
		//
		// The statutory hold: activity_refuse_restricted_mutation rejects a
		// write once restricted_at is set, so a label arriving AFTER a hold
		// already fails at the database. The order this clause covers is the
		// other one — label first, hold second — which commits the hold with a
		// derived label already on the row, and the trigger then refuses every
		// attempt to remove it.
		tag, err := tx.Exec(ctx, `
			UPDATE activity SET capture_label = $2, capture_labeled_at = now()
			WHERE id = $1 AND capture_label IS NULL AND archived_at IS NULL
			  AND audience = 'workspace' AND restricted_at IS NULL`, id, label)
		if err != nil {
			return fmt.Errorf("activities: setting capture label: %w", err)
		}
		applied = tag.RowsAffected() > 0
		return nil
	})
	return applied, err
}
