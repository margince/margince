// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A mail sent from Margince proves a send to every address it carried, the
// copied ones too. Those reach the proof only through their participant rows,
// so the read keeps the recipient roles while it drops the sender rows a busy
// inbound address has thousands of.

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestEveryRecipientOfMailSentFromMarginceWasWrittenTo(t *testing.T) {
	for _, role := range []string{"to", "cc", "bcc"} {
		t.Run(role, func(t *testing.T) { recipientWasWrittenTo(t, role) })
	}
}

func recipientWasWrittenTo(t *testing.T, role string) {
	t.Helper()
	ctx, pool := meetingTenureEnv(t)
	copied := role + "@buyer.example"
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		seat, mail := ids.NewV7(), ids.NewV7()
		if _, err := tx.Exec(ctx, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Sender')`,
			seat, "sender-"+seat.String()+"@seller.example"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO activity (id, kind, direction, occurred_at, subject, body, source, captured_by,
			counterparty_email, counterparty_outbound_attested)
			VALUES ($1, 'email', 'outbound', now() - interval '1 day', 'Proposal', 'Here is the proposal.', 'manual',
			'human:'||$2::text, 'lead@buyer.example', true)`, mail, seat); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, 'lead@buyer.example', 'to'), ($1, $2, $3)`,
			mail, copied, role); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO comms_outbound (activity_id, provider, message_id, recipients, cc, subject, body, status, sender_kind, user_id, consent_purpose)
			VALUES ($1, 'gmail', $4, jsonb_build_array('lead@buyer.example'), jsonb_build_array($2::text), 'Proposal', 'Here is the proposal.', 'sent', 'user', $3, 'sales')`,
			mail, copied, seat, "<"+mail.String()+"@seller.example>"); err != nil {
			return err
		}
		sent, err := partyWasWrittenToTx(ctx, tx, copied)
		if err != nil {
			return err
		}
		if !sent {
			t.Errorf("a %s recipient of mail sent from Margince was not written to", role)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
