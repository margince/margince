// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func sentUserMailSQL(alias string) string {
	return fmt.Sprintf("%[1]s.sender_kind='user' AND %[1]s.status='sent' AND %[1]s.user_id IS NOT NULL AND %[1]s.channel_user_id IS NULL", alias)
}

func partySendProofSQL(activity, address string) string {
	return `(` + activity + `.counterparty_outbound_attested AND (
 EXISTS (SELECT 1 FROM capture_party_source ps WHERE ps.activity_id=` + activity + `.id AND ps.sent_by_owner AND NOT ps.admission_blocked AND ` + address + `=ANY(ps.recipients))
 OR EXISTS (SELECT 1 FROM comms_outbound delivery WHERE delivery.activity_id=` + activity + `.id
 AND ` + sentUserMailSQL("delivery") + `
 AND NOT EXISTS(SELECT 1 FROM capture_party_source revoked WHERE revoked.activity_id=delivery.activity_id AND revoked.user_id=delivery.user_id AND revoked.admission_blocked)
 AND EXISTS (SELECT 1 FROM jsonb_array_elements_text(delivery.recipients || delivery.cc) recipient(address) WHERE lower(recipient.address)=` + address + `))
 OR (` + activity + `.captured_by LIKE 'connector:%' AND lower(` + activity + `.counterparty_email)=` + address + `
 AND NOT EXISTS (SELECT 1 FROM capture_party_source ps WHERE ps.activity_id=` + activity + `.id)
 AND NOT EXISTS (SELECT 1 FROM capture_party_source_replay modern WHERE modern.activity_id=` + activity + `.id)
 AND NOT EXISTS (SELECT 1 FROM comms_outbound staged WHERE staged.activity_id=` + activity + `.id))))`
}

func partyWasWrittenToTx(ctx context.Context, tx pgx.Tx, email string) (bool, error) {
	if normalizeEmail(email) == "" {
		return false, nil
	}
	args := []any{normalizeEmail(email)}
	address := fmt.Sprintf("$%d::text", len(args))
	visible, err := auth.ActivityContentClause(ctx, "a", func(value any) int {
		args = append(args, value)
		return len(args)
	})
	if err != nil {
		return false, err
	}
	query := `WITH candidates AS (
 SELECT ps.activity_id FROM capture_party_source ps WHERE ps.sent_by_owner AND NOT ps.admission_blocked AND ps.recipients @> ARRAY[` + address + `]
 UNION SELECT p.activity_id FROM activity_participant p WHERE lower(p.address)=` + address + `
 UNION SELECT a.id FROM activity a WHERE a.counterparty_email=` + address + `), visible_activities AS MATERIALIZED (
 SELECT a.id,a.occurred_at,a.subject,a.body,a.archived_at,a.counterparty_outbound_attested,a.captured_by,a.counterparty_email
 FROM activity a JOIN candidates c ON c.activity_id=a.id WHERE ` + visible + `)
 SELECT a.occurred_at, coalesce(a.subject,''), coalesce(a.body,'') FROM visible_activities a
 WHERE a.archived_at IS NULL AND ` + partySendProofSQL("a", address) + ` ORDER BY a.occurred_at,a.id LIMIT 2`
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	sent := []outboundMail{}
	for rows.Next() {
		var mail outboundMail
		var subject, body string
		if err := rows.Scan(&mail.at, &subject, &body); err != nil {
			return false, err
		}
		mail.words = subject + " " + textlang.NewTextOnly(body)
		sent = append(sent, mail)
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return writesTo(sent), nil
}
