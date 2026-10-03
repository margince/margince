// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// Mail the seat sent from another address of theirs, through the real sink.
// The connector knows only the grant address, so this mail reaches the sink
// as inbound from that other address; the sink holds the seat's addresses and
// is where it becomes the outbound mail it is.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
)

const formerAddress = "founder@previous-employer.example"

type capturedEnds struct {
	direction    string
	counterparty string
	attested     bool
}

func endsOf(t *testing.T, env captureEnv, messageID string) capturedEnds {
	t.Helper()
	var ends capturedEnds
	err := database.WithWorkspaceTx(env.e.Admin(), env.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT direction, coalesce(counterparty_email, ''), counterparty_outbound_attested
			  FROM activity WHERE source_id = $1`, messageID).
			Scan(&ends.direction, &ends.counterparty, &ends.attested)
	})
	if err != nil {
		t.Fatalf("reading the captured message %s: %v", messageID, err)
	}
	return ends
}

func TestMailSentFromTheSeatsFormerAddressIsTheirOutboundMail(t *testing.T) {
	env := newCaptureEnv(t)
	declareIdentity(t, env.e, env.e.Rep1, capturemod.IdentityKindAddress, formerAddress)

	env.syncSent(t, map[string]bool{"former-1@previous-employer.example": true},
		emailCC(formerAddress, "Founder", "buyer@customer.example", "other@customer.example",
			"former-1@previous-employer.example"))

	got := endsOf(t, env, "former-1@previous-employer.example")
	want := capturedEnds{direction: "outbound", counterparty: "buyer@customer.example", attested: true}
	if got != want {
		t.Errorf("mail the seat sent from %s was captured as %+v, want %+v", formerAddress, got, want)
	}
	if n := countRows(t, env.e, `
		SELECT count(*) FROM activity_participant p JOIN activity a ON a.id = p.activity_id
		 WHERE a.source_id = 'former-1@previous-employer.example' AND p.role = 'from' AND p.user_id = '`+
		env.e.Rep1.String()+`'`); n != 1 {
		t.Errorf("the seat is the sender on %d participant rows, want 1", n)
	}
}

// A From line is the sender's text. Without the provider filing the message as
// sent, a stranger writing the seat's former address stays a stranger's mail.
func TestMailFromTheFormerAddressThatWasNotFiledAsSentStaysInbound(t *testing.T) {
	env := newCaptureEnv(t)
	declareIdentity(t, env.e, env.e.Rep1, capturemod.IdentityKindAddress, formerAddress)

	env.sync(t, emailCC(formerAddress, "Founder", captureOwner, "other@customer.example",
		"former-2@previous-employer.example"))

	if got := endsOf(t, env, "former-2@previous-employer.example"); got.direction != "inbound" || got.attested {
		t.Errorf("unfiled mail from %s was captured as %+v, want inbound and unattested", formerAddress, got)
	}
}

// Filed as sent but from an address the seat never claimed: placement is not
// authorship, so the message keeps the direction its From gives it.
func TestMailFiledAsSentFromAnUnclaimedAddressStaysInbound(t *testing.T) {
	env := newCaptureEnv(t)

	env.syncSent(t, map[string]bool{"former-3@previous-employer.example": true},
		emailCC(formerAddress, "Founder", "buyer@customer.example", "other@customer.example",
			"former-3@previous-employer.example"))

	if got := endsOf(t, env, "former-3@previous-employer.example"); got.direction != "inbound" || got.attested {
		t.Errorf("sent-filed mail from an unclaimed address was captured as %+v, want inbound and unattested", got)
	}
}
