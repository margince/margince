// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type instructionMessage struct {
	Subject, Body, HTMLBody string
	Scope                   string
	Status, HeldReason      string
	ScheduledBy             ids.UUID
	Version                 int64
	RecordedAt              time.Time
	Known                   bool
	Wording                 [sha256.Size]byte
}

// All payload fields except wording belong to the frozen scope, including
// fields added by a later writer that this reader does not yet name.
func instructionMessageTx(ctx context.Context, tx pgx.Tx, intentID ids.UUID) (instructionMessage, error) {
	var out instructionMessage
	var scope []byte
	err := tx.QueryRow(ctx, `
		SELECT now(), s.id IS NOT NULL, coalesce(s.version, 0),
		       coalesce(s.payload->>'subject', ''), coalesce(s.payload->>'body', ''),
		       coalesce(s.payload->>'html_body', ''), coalesce(s.status, ''),
		       coalesce(s.held_reason, ''), coalesce(s.scheduled_by, '00000000-0000-0000-0000-000000000000'::uuid),
		       (to_jsonb(s) - ARRAY['payload','status','held_reason','version','updated_at',
		         'scheduled_at','scheduled_tz','activity_id','delivery_id','created_at']) ||
		       jsonb_build_object('payload', s.payload - ARRAY['subject','body','html_body'])
		  FROM (SELECT 1) clock LEFT JOIN scheduled_send s ON s.id = @intent`,
		pgx.StrictNamedArgs{"intent": intentID}).Scan(&out.RecordedAt, &out.Known, &out.Version,
		&out.Subject, &out.Body, &out.HTMLBody, &out.Status, &out.HeldReason, &out.ScheduledBy, &scope)
	if err != nil {
		return out, fmt.Errorf("consent: reading the message being acknowledged: %w", err)
	}
	if out.Known {
		out.Wording = SendingDigest(out.Subject, out.Body, out.HTMLBody)
		_, out.Scope, err = diffhash.Canonical(scope)
		if err != nil {
			return out, fmt.Errorf("consent: fingerprinting the message scope: %w", err)
		}
	}
	return out, nil
}

func (m instructionMessage) matchesApproval(inst correctionInstruction) bool {
	return m.Known && inst.Scope != "" && inst.Scope == m.Scope && bytes.Equal(inst.Approved, m.Wording[:])
}
