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

// The seat's own alias is theirs. A colleague's copy received from it, which the
// seat's mailbox then delivers from its sent filing, is the seat's outbound mail
// to the customer.
func TestTheSeatsOwnAliasTurnsAColleaguesCopyRound(t *testing.T) {
	env := newCaptureEnv(t)
	declareIdentity(t, env.e, env.e.Rep1, capturemod.IdentityKindAddress, formerAddress)
	colleague := secondMailbox(t, env.e, env.e.Rep3)
	const msgID = "sent-colleague-first@previous-employer.example"
	raw := emailCC(formerAddress, "Founder", "buyer@customer.example", secondSeatAddress, msgID)

	colleague(t, raw)
	env.syncSent(t, map[string]bool{msgID: true}, raw)

	want := capturedEnds{direction: "outbound", counterparty: "buyer@customer.example", attested: true}
	if got := endsOf(t, env, msgID); got != want {
		t.Errorf("the seat's sent copy from its own alias left the colleague's copy as %+v, want %+v", got, want)
	}
}

// An address two seats hold is nobody's to claim: whichever mailbox synced
// first would otherwise decide whose mail it is.
func TestAnAddressAnotherSeatAlsoHoldsDoesNotTurnACopyRound(t *testing.T) {
	env := newCaptureEnv(t)
	declareIdentity(t, env.e, env.e.Rep1, capturemod.IdentityKindAddress, formerAddress)
	declareIdentity(t, env.e, env.e.Rep3, capturemod.IdentityKindAddress, formerAddress)
	colleague := secondMailbox(t, env.e, env.e.Rep3)
	const msgID = "shared-alias@previous-employer.example"
	raw := emailCC(formerAddress, "Founder", "buyer@customer.example", secondSeatAddress, msgID)

	colleague(t, raw)
	before := endsOf(t, env, msgID)
	env.syncSent(t, map[string]bool{msgID: true}, raw)

	if got := endsOf(t, env, msgID); got != before {
		t.Errorf("an address two seats hold rewrote the colleague's copy from %+v to %+v", before, got)
	}
}

// The correction needs the provider's sent filing. A colleague's received copy
// replayed by the seat's mailbox WITHOUT it stays as first read.
func TestAColleaguesCopyStaysInboundWithoutTheSentFiling(t *testing.T) {
	env := newCaptureEnv(t)
	declareIdentity(t, env.e, env.e.Rep1, capturemod.IdentityKindAddress, formerAddress)
	colleague := secondMailbox(t, env.e, env.e.Rep3)
	const msgID = "unfiled-colleague-first@previous-employer.example"
	raw := emailCC(formerAddress, "Founder", "buyer@customer.example", secondSeatAddress, msgID)

	colleague(t, raw)
	env.sync(t, raw)

	if got := endsOf(t, env, msgID); got.direction != "inbound" || got.attested {
		t.Errorf("an unfiled replay rewrote the colleague's copy to %+v", got)
	}
}

// Mail the seat sent, stored before the provider's sent filing reached it, is
// their outbound mail without the attestation. The filing arriving later
// attests it, which is what lets a reply count as an answer off its thread.
func TestALaterSentFilingAttestsTheSeatsOwnMail(t *testing.T) {
	env := newCaptureEnv(t)
	const msgID = "sent-unfiled-first@myco.example"
	raw := emailCC(captureOwner, "Owner", "buyer@customer.example", "other@customer.example", msgID)

	env.sync(t, raw)
	if got := endsOf(t, env, msgID); got.direction != "outbound" || got.attested {
		t.Fatalf("the unfiled copy was captured as %+v; this test needs it outbound and unattested first", got)
	}
	env.syncSent(t, map[string]bool{msgID: true}, raw)

	want := capturedEnds{direction: "outbound", counterparty: "buyer@customer.example", attested: true}
	if got := endsOf(t, env, msgID); got != want {
		t.Errorf("after the sent filing, the message reads %+v, want %+v", got, want)
	}
}

// Attesting needs authorship, not a matching recipient. A seat that declares a
// colleague's address and plants the colleague's message as sent reaches the
// attestation branch with a proven replay, and must still change nothing.
//
// The colleague's copy is set to what their own mailbox stores for mail they
// sent (TestALaterSentFilingAttestsTheSeatsOwnMail reaches the same shape
// through the writer): outbound to the buyer, sent by them, unattested.
// The fake connector reads every mailbox as the workspace owner's, so it
// cannot deliver that shape through a second seat.
func TestAPlantedSentCopyDoesNotAttestAColleaguesMail(t *testing.T) {
	env := newCaptureEnv(t)
	colleague := secondMailbox(t, env.e, env.e.Rep3)
	declareIdentity(t, env.e, env.e.Rep1, capturemod.IdentityKindAddress, secondSeatAddress)
	const msgID = "colleagues-own@myco.example"
	raw := emailCC(secondSeatAddress, "Colleague", "buyer@customer.example", "other@customer.example", msgID)
	colleague(t, raw)
	err := database.WithWorkspaceTx(env.e.Admin(), env.e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if _, err := tx.Exec(ctx, `UPDATE activity SET direction = 'outbound', counterparty_email = 'buyer@customer.example'
			WHERE source_id = $1`, msgID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO activity_participant (activity_id, role, user_id)
			SELECT id, 'from', $2 FROM activity WHERE source_id = $1`, msgID, env.e.Rep3)
		return err
	})
	if err != nil {
		t.Fatalf("setting the colleague's copy to their own sent mail: %v", err)
	}
	before := endsOf(t, env, msgID)

	env.syncSent(t, map[string]bool{msgID: true}, raw)

	if got := endsOf(t, env, msgID); got != before {
		t.Errorf("a planted sent copy changed the colleague's mail from %+v to %+v", before, got)
	}
}

// One address in two spellings is one address: a Unicode domain and its
// punycode are the same mailbox, and a second seat holding either spelling
// keeps the alias nobody's to claim.
func TestAnAliasAnotherSeatHoldsInAnotherSpellingDoesNotTurnACopyRound(t *testing.T) {
	env := newCaptureEnv(t)
	const punycode = "founder@xn--bcher-kva.example"
	declareIdentity(t, env.e, env.e.Rep1, capturemod.IdentityKindAddress, punycode)
	declareIdentity(t, env.e, env.e.Rep3, capturemod.IdentityKindAddress, "founder@bücher.example")
	colleague := secondMailbox(t, env.e, env.e.Rep3)
	const msgID = "spelled-twice@xn--bcher-kva.example"
	raw := emailCC(punycode, "Founder", "buyer@customer.example", secondSeatAddress, msgID)

	colleague(t, raw)
	before := endsOf(t, env, msgID)
	env.syncSent(t, map[string]bool{msgID: true}, raw)

	if got := endsOf(t, env, msgID); got != before {
		t.Errorf("an alias the colleague holds in its Unicode spelling rewrote their copy from %+v to %+v", before, got)
	}
}
