// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
 AND comms_outbound_addressed(delivery.recipients, delivery.cc) @> ARRAY[` + address + `])
 OR (` + activity + `.captured_by LIKE 'connector:%' AND lower(` + activity + `.counterparty_email)=` + address + `
 AND NOT EXISTS (SELECT 1 FROM capture_party_source ps WHERE ps.activity_id=` + activity + `.id)
 AND NOT EXISTS (SELECT 1 FROM capture_party_source_replay modern WHERE modern.activity_id=` + activity + `.id)
 AND NOT EXISTS (SELECT 1 FROM comms_outbound staged WHERE staged.activity_id=` + activity + `.id))))`
}

// proofCandidatesSQL lists the activities each arm of partySendProofSQL can
// prove, by an index on the address, cheapest arm first. The read joins it to
// its gates and stops at the sends it needs, so it never gathers them all. The
// last two arms count only mail that also lists the address in a recipient row.
// A later arm skips mail the first arm lists, so one send is not read twice.
var proofCandidatesSQL = `SELECT a.id, 1 FROM activity a WHERE a.counterparty_email = $1::text AND a.counterparty_outbound_attested
 UNION ALL SELECT ps.activity_id, 2 FROM capture_party_source ps
  WHERE ps.sent_by_owner AND NOT ps.admission_blocked AND ps.recipients @> ARRAY[$1::text]
 UNION ALL SELECT sent.activity_id, 3 FROM comms_outbound sent
  WHERE ` + sentUserMailSQL("sent") + ` AND comms_outbound_addressed(sent.recipients, sent.cc) @> ARRAY[$1::text]
   AND ` + recipientRowSQL("sent.activity_id") + `
 UNION ALL SELECT a.id, 4 FROM activity a WHERE activity_unfolded_counterparty(a.counterparty_email) = $1::text
   AND ` + recipientRowSQL("a.id")

func recipientRowSQL(activity string) string {
	return `EXISTS (SELECT 1 FROM activity_participant p WHERE p.activity_id = ` + activity + `
     AND lower(p.address) = $1::text AND p.role IN ('to','cc','bcc'))`
}

// partyWasWrittenToTx reports whether the workspace provably wrote to email.
// Any two proven sends decide, and so does a lone one, so the read stops at the
// first two distinct ones. Reading every candidate first cost a busy
// correspondent's whole history on each message.
func partyWasWrittenToTx(ctx context.Context, tx pgx.Tx, email string) (bool, error) {
	address := normalizeEmail(email)
	if address == "" {
		return false, nil
	}
	base := []any{address}
	visible, err := auth.ActivityContentClause(ctx, "a", func(value any) int {
		base = append(base, value)
		return len(base)
	})
	if err != nil {
		return false, err
	}
	query := `SELECT a.id, a.occurred_at, coalesce(a.subject,''), coalesce(a.body,'')
 FROM (` + proofCandidatesSQL + `) candidate(id, arm) JOIN activity a ON a.id = candidate.id
 WHERE a.counterparty_outbound_attested AND a.archived_at IS NULL AND ` + visible + `
   AND (candidate.arm = 1 OR a.counterparty_email IS DISTINCT FROM $1::text)
   AND ` + partySendProofSQL("a", "$1::text") + fmt.Sprintf(`
   AND a.id <> ALL(coalesce($%d::uuid[], '{}')) LIMIT $%d`, len(base)+1, len(base)+2)
	proven := provenSends{}
	err = database.ReuseGenericPlansTx(ctx, tx, func() error {
		for !proven.decided() {
			need := 2 - len(proven.mail)
			returned, err := proven.read(ctx, tx, query, append(slices.Clone(base), proven.ids, need))
			if err != nil || returned < need {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return writesTo(proven.mail), nil
}

// provenSends collects distinct proven sends until two decide the question.
// One mail can be listed twice, once per mailbox that captured it or once per
// arm that finds it. So a full answer with a repeat is asked again, with the
// found mail excluded, and a short answer means every arm is used up.
type provenSends struct {
	ids  []ids.UUID
	mail []outboundMail
}

func (p *provenSends) decided() bool { return len(p.mail) >= 2 }

func (p *provenSends) read(ctx context.Context, tx pgx.Tx, query string, args []any) (int, error) {
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	returned := 0
	for rows.Next() {
		var id ids.UUID
		var mail outboundMail
		var subject, body string
		if err := rows.Scan(&id, &mail.at, &subject, &body); err != nil {
			return 0, err
		}
		returned++
		if slices.Contains(p.ids, id) {
			continue
		}
		mail.words = subject + " " + textlang.NewTextOnly(body)
		p.ids = append(p.ids, id)
		p.mail = append(p.mail, mail)
	}
	return returned, rows.Err()
}
