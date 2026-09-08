// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The test_mailbox connector's own bookkeeping: what it sent, and whether
// its own Sync has echoed it back yet. Capture-owned, no workspace_id — the
// audited record is the domain activity Sink.Upsert produces from the echo,
// plus the send-side comms_outbound row; this table is neither, and rows
// are retained (no sweep) until the table itself is dropped.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SentMessage is one row test_mailbox's SendEmail recorded, not yet echoed.
type SentMessage struct {
	ID        ids.UUID
	MessageID string
	To, Cc    []string
	Subject   string
	SentAt    time.Time
}

// TestMailboxLedger is test_mailbox's own sent-and-echo bookkeeping store.
type TestMailboxLedger struct{ db *database.DB }

// NewTestMailboxLedger builds the ledger over the installation handle.
func NewTestMailboxLedger(db *database.DB) *TestMailboxLedger {
	return &TestMailboxLedger{db: db}
}

// RecordSent writes one row per successful SendEmail call, idempotent on
// (user_id, message_id): a dispatcher retry between this write and the
// comms commit re-sends the identical row, which the conflict target
// discards rather than duplicates.
func (l *TestMailboxLedger) RecordSent(ctx context.Context, userID ids.UUID, messageID string, to, cc []string, subject string) error {
	// A nil slice binds as SQL NULL, not the column default — both columns
	// are NOT NULL. Cc-with-no-To is not a shape SendEmail's own quarantine
	// loop refuses (an empty To passes trivially, nothing to check), so this
	// is not an edge case to special-case away for either list.
	if to == nil {
		to = []string{}
	}
	if cc == nil {
		cc = []string{}
	}
	return l.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO capture_test_mailbox_sent (user_id, message_id, to_addresses, cc_addresses, subject)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (user_id, message_id) DO NOTHING`,
			userID, messageID, to, cc, subject)
		if err != nil {
			return fmt.Errorf("test_mailbox: recording a sent message: %w", err)
		}
		return nil
	})
}

// Unechoed returns the rows this seat has not yet filed back, oldest first,
// for Sync to echo through the sink.
func (l *TestMailboxLedger) Unechoed(ctx context.Context, userID ids.UUID) ([]SentMessage, error) {
	var out []SentMessage
	err := l.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, message_id, to_addresses, cc_addresses, subject, sent_at
			  FROM capture_test_mailbox_sent
			 WHERE user_id = $1 AND echoed_at IS NULL
			 ORDER BY sent_at`,
			userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var m SentMessage
			if err := rows.Scan(&m.ID, &m.MessageID, &m.To, &m.Cc, &m.Subject, &m.SentAt); err != nil {
				return err
			}
			out = append(out, m)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("test_mailbox: reading unechoed messages: %w", err)
	}
	return out, nil
}

// MarkEchoed records that Sync has already filed this row back, so the next
// sync does not re-emit it. Scoped by userID as well as id — the row's only
// tenancy boundary (this table carries no workspace_id) must hold in the
// store itself, not rely on every caller already having filtered by it.
func (l *TestMailboxLedger) MarkEchoed(ctx context.Context, userID ids.UUID, id ids.UUID) error {
	return l.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`UPDATE capture_test_mailbox_sent SET echoed_at = now() WHERE id = $1 AND user_id = $2`, id, userID)
		if err != nil {
			return fmt.Errorf("test_mailbox: marking a message echoed: %w", err)
		}
		return nil
	})
}
