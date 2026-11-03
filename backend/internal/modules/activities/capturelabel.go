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
	// Inbound says whether the counterparty wrote this message or we did. The
	// label itself does not care — attention routes on mail in both directions —
	// but the same call also judges whether a REPLY was positive, and that
	// question is only meaningful about a message somebody sent us.
	Inbound bool
	// Version is the row's version when it was read. SetCaptureLabel applies
	// the verdict only to that version. Text that changed during the model
	// call is classified again, not labelled from its previous text.
	Version int64
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
//
// ruleset is the digest of the classify prompt the caller will ask: a message
// every rung declined under another prompt is offered to this one.
func (s *Store) UnlabeledCaptureEmails(ctx context.Context, ruleset string, limit, bodyLimit int) ([]UnlabeledEmail, error) {
	if ruleset == "" {
		return nil, fmt.Errorf("activities: reading the classify backlog: %w", errRulesetUnnamed)
	}
	var out []UnlabeledEmail
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, coalesce(subject, ''), coalesce(left(body, $1), ''),
			       -- direction is NULLABLE, and a NULL is not an inbound message.
			       -- Compared bare, the equality yields NULL for a row that never
			       -- recorded a direction, which fails the scan and takes the whole
			       -- batch down: one directionless row would stop the pass for
			       -- every message queued behind it. Unknown reads as outbound,
			       -- which is the safe way round — the message is still labelled,
			       -- and it is asked no reply question rather than having its
			       -- direction guessed.
			       coalesce(direction = 'inbound', false), version
			FROM activity
			WHERE `+ClassifyBacklogPredicate("$3")+`
			ORDER BY occurred_at
			LIMIT $2`, bodyLimit, limit, ruleset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var m UnlabeledEmail
			if err := rows.Scan(&m.ID, &m.Subject, &m.Body, &m.Inbound, &m.Version); err != nil {
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
// capture_label IS NULL predicate is the CAS. A concurrent pass that labeled
// the row first wins, and this write reports applied=false. The earlier verdict
// stands, never an overwrite. A row whose version moved since readVersion also
// reports applied=false, because the verdict is about old text.
func (s *Store) SetCaptureLabel(ctx context.Context, id ids.UUID, label string, readVersion int64) (applied bool, err error) {
	err = s.db.Tx(ctx, func(tx pgx.Tx) (err error) {
		applied, err = setCaptureLabelTx(ctx, tx, id, label, readVersion)
		return err
	})
	return applied, err
}

// setCaptureLabelTx is the label write on the caller's transaction.
//
// The exclusions are re-tested at write time, not only in the backlog that
// selected the row, because the row can change during the model call. A label
// must not put an archived or narrowed message back on a shared worklist. A
// statutory hold fixes a derived label in place once it is set.
func setCaptureLabelTx(ctx context.Context, tx pgx.Tx, id ids.UUID, label string, readVersion int64) (bool, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE activity SET capture_label = $2, capture_labeled_at = now()
		WHERE id = $1 AND capture_label IS NULL AND archived_at IS NULL
		  AND audience = 'workspace' AND restricted_at IS NULL AND version = $3`, id, label, readVersion)
	if err != nil {
		return false, fmt.Errorf("activities: setting capture label: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}
