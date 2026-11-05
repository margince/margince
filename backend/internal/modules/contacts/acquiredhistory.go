// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Provider provenance and the importing seat's first connection establish held correspondence.
func writtenToBeforeConnectedTx(ctx context.Context, tx pgx.Tx, email string) (bool, error) {
	var held bool
	err := tx.QueryRow(ctx, "SELECT "+sentHistorySQL+" FROM (SELECT lower(trim(@email)) AS email) ce", pgx.NamedArgs{historyEmailParam: email, historyExpandedParam: true}).Scan(&held)
	if err != nil {
		return false, fmt.Errorf("contacts: read correspondence history: %w", err)
	}
	return held, nil
}

func receivedBeforeConnectedTx(ctx context.Context, tx pgx.Tx, email string) (bool, error) {
	var held bool
	err := tx.QueryRow(ctx, "SELECT "+receivedHistorySQL+" FROM (SELECT lower(trim(@email)) AS email) ce", pgx.NamedArgs{historyEmailParam: email, historyExpandedParam: true}).Scan(&held)
	if err != nil {
		return false, fmt.Errorf("contacts: read correspondence history: %w", err)
	}
	return held, nil
}

// The versioned correction retains its narrower scope; runtime admission also covers fellow recipients.
const sentHistorySQL = `EXISTS (
 SELECT 1 FROM activity a
 JOIN capture_import ci ON ci.activity_id = a.id
 JOIN activity_participant s ON s.activity_id = a.id AND s.role = 'from' AND s.user_id = ci.user_id
 WHERE (a.counterparty_email = ce.email OR (@expanded AND EXISTS (
 SELECT 1 FROM activity_participant p WHERE p.activity_id = a.id AND p.role IN ('to', 'cc') AND p.address = ce.email)))
 AND a.kind = 'email' AND a.direction = 'outbound'
 AND a.counterparty_outbound_attested AND a.archived_at IS NULL
 AND a.occurred_at < (SELECT min(cc.created_at) FROM capture_connection cc WHERE cc.user_id = ci.user_id) -- occurred_at raw: mail sent before the mailbox connected, an instant
)`

const receivedHistorySQL = `EXISTS (
 SELECT 1 FROM activity_participant p
 JOIN activity a ON a.id = p.activity_id
 JOIN capture_import ci ON ci.activity_id = a.id
 WHERE p.address = ce.email AND p.role IN ('to', 'cc')
 AND a.kind = 'email' AND (a.direction = 'inbound' OR (@expanded AND a.sent_by_colleague IS NOT NULL))
 AND NOT a.bulk_mail_attested AND a.archived_at IS NULL
 AND ci.provider_received_at < (SELECT min(cc.created_at) FROM capture_connection cc WHERE cc.user_id = ci.user_id))`

const (
	historyEmailParam    = "email"
	historyExpandedParam = "expanded"
)
