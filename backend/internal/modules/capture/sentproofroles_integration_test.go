// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A mail sent from Margince proves a send to every address it carried, the
// copied ones too. Those reach the proof only through their participant rows.
// So the read keeps the recipient roles, and drops the sender rows a busy
// inbound address has thousands of.

import (
	"context"
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
		// A To address is stored with the recipients, a copied one with cc.
		to, cc := `jsonb_build_array('lead@buyer.example')`, `jsonb_build_array($2::text)`
		if role == "to" {
			to, cc = `jsonb_build_array('lead@buyer.example', $2::text)`, `'[]'::jsonb`
		}
		if _, err := tx.Exec(ctx, `INSERT INTO comms_outbound (activity_id, provider, message_id, recipients, cc, subject, body, status, sender_kind, user_id, consent_purpose)
			VALUES ($1, 'gmail', $4, `+to+`, `+cc+`, 'Proposal', 'Here is the proposal.', 'sent', 'user', $3, 'sales')`,
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

// One sent mail captured in two mailboxes has two party-source rows. The read
// stops at two proofs, so the repeat must neither count as a second send nor
// hide one.
func TestOneSendSeenFromTwoMailboxesIsOneSend(t *testing.T) {
	for name, tc := range map[string]struct {
		later string
		want  bool
	}{
		"a declining mail alone":           {"", false},
		"a declining mail and another one": {"Here is the proposal.", true},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, pool := meetingTenureEnv(t)
			copied := "copied-" + ids.NewV7().String() + "@buyer.example"
			err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
				first, second := ids.NewV7(), ids.NewV7()
				for _, seat := range []ids.UUID{first, second} {
					if _, err := tx.Exec(ctx, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Sender')`,
						seat, "sender-"+seat.String()+"@seller.example"); err != nil {
						return err
					}
				}
				if err := captureSentMail(ctx, tx, copied, "Not interested, thanks.", first, second); err != nil {
					return err
				}
				if tc.later != "" {
					if err := captureSentMail(ctx, tx, copied, tc.later, first); err != nil {
						return err
					}
				}
				sent, err := partyWasWrittenToTx(ctx, tx, copied)
				if err != nil {
					return err
				}
				if sent != tc.want {
					t.Errorf("written to = %v, want %v", sent, tc.want)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// captureSentMail stores a sent mail that copies the address, captured from
// each mailbox through the party-source writer.
func captureSentMail(ctx context.Context, tx pgx.Tx, copied, body string, mailboxes ...ids.UUID) error {
	mail := ids.NewV7()
	if _, err := tx.Exec(ctx, `INSERT INTO activity (id, kind, direction, occurred_at, subject, body, source, captured_by,
		counterparty_email, counterparty_outbound_attested)
		VALUES ($1, 'email', 'outbound', now() - interval '1 day', 'Proposal', $3, 'gmail',
		'connector:gmail:'||$2::text, 'lead@buyer.example', true)`, mail, mailboxes[0], body); err != nil {
		return err
	}
	for _, mailbox := range mailboxes {
		if _, err := tx.Exec(ctx, `INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, mail, mailbox); err != nil {
			return err
		}
		evidence := partySourceEvidence{
			Activity: ids.From[ids.ActivityKind](mail), User: mailbox, Provider: "gmail",
			Sent: true, Recipients: []string{"lead@buyer.example", copied}, ContainersKnown: true,
		}
		if err := recordPartySourceTx(ctx, tx, evidence); err != nil {
			return err
		}
	}
	return nil
}

// A send from Margince to its counterparty is reached two ways, by the
// counterparty and by the recipients. A declining one is still one send.
func TestADecliningSendReachedTwoWaysIsOneSend(t *testing.T) {
	ctx, pool := meetingTenureEnv(t)
	buyer := "buyer-" + ids.NewV7().String() + "@buyer.example"
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		seat, mail := ids.NewV7(), ids.NewV7()
		if _, err := tx.Exec(ctx, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Sender')`,
			seat, "sender-"+seat.String()+"@seller.example"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO activity (id, kind, direction, occurred_at, subject, body, source, captured_by,
			counterparty_email, counterparty_outbound_attested)
			VALUES ($1, 'email', 'outbound', now() - interval '1 day', 'Re: Offer', 'Not interested, thanks.', 'manual',
			'human:'||$2::text, $3, true)`, mail, seat, buyer); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, $2, 'to')`, mail, buyer); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO comms_outbound (activity_id, provider, message_id, recipients, cc, subject, body, status, sender_kind, user_id, consent_purpose)
			VALUES ($1, 'gmail', $4, jsonb_build_array($2::text), '[]'::jsonb, 'Re: Offer', 'Not interested, thanks.', 'sent', 'user', $3, 'sales')`,
			mail, buyer, seat, "<"+mail.String()+"@seller.example>"); err != nil {
			return err
		}
		sent, err := partyWasWrittenToTx(ctx, tx, buyer)
		if err != nil {
			return err
		}
		if sent {
			t.Error("one declining send counted as a correspondence")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// A captured send stored before capture folded its counterparty still proves
// the send to that address.
func TestAnUnfoldedCounterpartyStillProvesASend(t *testing.T) {
	ctx, pool := meetingTenureEnv(t)
	local := "buyer-" + ids.NewV7().String()
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		seat, mail := ids.NewV7(), ids.NewV7()
		if _, err := tx.Exec(ctx, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Sender')`,
			seat, "sender-"+seat.String()+"@seller.example"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO activity (id, kind, direction, occurred_at, subject, body, source, captured_by,
			counterparty_email, counterparty_outbound_attested)
			VALUES ($1, 'email', 'outbound', now() - interval '1 day', 'Proposal', 'Here is the proposal.', 'gmail',
			'connector:gmail:'||$2::text, $3, true)`, mail, seat, "Mixed-"+local+"@Buyer.example"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, $2, 'to')`,
			mail, "mixed-"+local+"@buyer.example"); err != nil {
			return err
		}
		sent, err := partyWasWrittenToTx(ctx, tx, "mixed-"+local+"@buyer.example")
		if err != nil {
			return err
		}
		if !sent {
			t.Error("a send to an unfolded counterparty was not found")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
