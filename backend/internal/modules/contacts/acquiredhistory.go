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
// Lateral key fences prevent cached plans from hashing the whole mailbox.
const sentHistorySQL = `EXISTS (
 WITH candidates AS MATERIALIZED (
 SELECT id FROM activity WHERE counterparty_email=ce.email
 UNION ALL SELECT activity_id FROM (` + historyRecipientsSQL + `) recipients WHERE @expanded)
 SELECT 1 FROM candidates c
 CROSS JOIN LATERAL (
 SELECT id,occurred_at,kind,direction,counterparty_outbound_attested,archived_at
 FROM activity WHERE id=c.id OFFSET 0) a
 CROSS JOIN LATERAL (
 SELECT user_id FROM capture_import WHERE activity_id=a.id OFFSET 0) ci
 WHERE a.kind='email' AND a.direction='outbound'
 AND a.counterparty_outbound_attested AND a.archived_at IS NULL
 AND EXISTS (SELECT 1 FROM activity_participant s WHERE s.activity_id=a.id
 AND s.role='from' AND s.user_id=ci.user_id OFFSET 0)
 AND a.occurred_at < (SELECT min(cc.created_at) FROM capture_connection cc WHERE cc.user_id=ci.user_id) -- occurred_at raw: sent mail predates the mailbox connection, an instant
)`

const receivedHistorySQL = `EXISTS (
 WITH recipients AS MATERIALIZED (` + historyRecipientsSQL + `)
 SELECT 1 FROM recipients p
 CROSS JOIN LATERAL (
 SELECT id,kind,direction,sent_by_colleague,bulk_mail_attested,archived_at
 FROM activity WHERE id=p.activity_id OFFSET 0) a
 CROSS JOIN LATERAL (
 SELECT user_id,provider_received_at FROM capture_import WHERE activity_id=a.id OFFSET 0) ci
 WHERE a.kind='email' AND (a.direction='inbound' OR (@expanded AND a.sent_by_colleague IS NOT NULL))
 AND NOT a.bulk_mail_attested AND a.archived_at IS NULL
 AND ci.provider_received_at < (SELECT min(cc.created_at) FROM capture_connection cc WHERE cc.user_id=ci.user_id))`

const (
	historyEmailParam    = "email"
	historyExpandedParam = "expanded"
)
