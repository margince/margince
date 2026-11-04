// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// What the sender ledger says about an address whose contact a confidentiality
// hold kept to its owner.
//
// The release reads the rows, not a yes or no. Each withheld row names the
// message whose thread held it. Every one of those must stop holding first.

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SenderStanding is one ledger row for an address.
type SenderStanding struct {
	ID         ids.UUID
	Email      string
	Status     string
	Withheld   bool
	ActivityID ids.UUID
	// Outbound is a message the mailbox owner provably sent to the address, read
	// with the provider's attestation as the verdict's claim reads it.
	Outbound bool
}

// openQuestionStatuses are the ledger states that keep whatever an address's
// contact is kept for: a question still waiting for an answer, or an erasure.
var openQuestionStatuses = []string{PendingStatusPending, PendingStatusUnsure, PendingStatusSuppressed}

// OpenQuestion reports a row in one of openQuestionStatuses.
func (s SenderStanding) OpenQuestion() bool {
	return slices.Contains(openQuestionStatuses, s.Status)
}

// WithheldContact reports a `contact` answer whose record was kept to the
// owner (MarkWithheldFromWorkspaceTx).
func (s SenderStanding) WithheldContact() bool {
	return s.Withheld && s.Status == PendingStatusReal
}

// SenderStandingsTx reads every ledger row for the addresses, in id order.
func SenderStandingsTx(ctx context.Context, tx pgx.Tx, emails []string) ([]SenderStanding, error) {
	folded := make([]string, 0, len(emails))
	for _, email := range emails {
		if f := normalizeEmail(email); f != "" {
			folded = append(folded, f)
		}
	}
	if len(folded) == 0 {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT p.id, p.email, p.status, p.withheld_from_workspace, p.activity_id,
		       coalesce((SELECT a.direction = 'outbound' AND a.counterparty_outbound_attested
		                   FROM activity a WHERE a.id = p.activity_id AND a.restricted_at IS NULL), false)
		  FROM capture_pending_counterparty p
		 WHERE p.email = ANY($1)
		 ORDER BY p.id`, folded)
	if err != nil {
		return nil, fmt.Errorf("capture: reading the ledger for a held contact: %w", err)
	}
	defer rows.Close()
	var out []SenderStanding
	for rows.Next() {
		var s SenderStanding
		if err := rows.Scan(&s.ID, &s.Email, &s.Status, &s.Withheld, &s.ActivityID, &s.Outbound); err != nil {
			return nil, fmt.Errorf("capture: reading the ledger for a held contact: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("capture: reading the ledger for a held contact: %w", err)
	}
	return out, nil
}

// WithheldSendersOnThreadTx lists up to limit addresses whose contact was
// withheld because of a message on this thread. A thread that stops holding
// may end their holds.
func WithheldSendersOnThreadTx(ctx context.Context, tx pgx.Tx, threadKey string, limit int) ([]string, error) {
	if threadKey == "" || limit <= 0 {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT p.email
		  FROM activity a
		  JOIN capture_pending_counterparty p ON p.activity_id = a.id
		 WHERE a.thread_key = $1 AND a.restricted_at IS NULL AND p.withheld_from_workspace
		 ORDER BY p.email
		 LIMIT $2`, threadKey, limit)
	if err != nil {
		return nil, fmt.Errorf("capture: listing the senders a thread withheld: %w", err)
	}
	emails, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("capture: listing the senders a thread withheld: %w", err)
	}
	return emails, nil
}

// ReleasableWithheldSendersTx lists up to limit addresses after `after`, in
// order. Each has withheld `contact` answers on no held message, and no open
// question. A hold on their contacts outlived its reason, through a failed live
// release or verdicts that landed in the other order.
func ReleasableWithheldSendersTx(ctx context.Context, tx pgx.Tx, after string, limit int) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT p.email
		  FROM capture_pending_counterparty p
		 WHERE p.withheld_from_workspace AND p.status = $3 AND p.email > $1
		   AND NOT EXISTS (
		       SELECT 1 FROM capture_pending_counterparty w
		         JOIN activity a ON a.id = w.activity_id
		        WHERE w.email = p.email AND w.withheld_from_workspace AND `+CounterpartyHeldOn("a")+`)
		   AND NOT EXISTS (
		       SELECT 1 FROM capture_pending_counterparty q
		        WHERE q.email = p.email AND q.status = ANY($4))
		 ORDER BY p.email
		 LIMIT $2`, after, limit, PendingStatusReal, openQuestionStatuses)
	if err != nil {
		return nil, fmt.Errorf("capture: listing withheld senders nothing holds: %w", err)
	}
	emails, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("capture: listing withheld senders nothing holds: %w", err)
	}
	return emails, nil
}
