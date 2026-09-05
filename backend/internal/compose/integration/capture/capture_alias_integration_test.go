// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A forwarding alias discovered from the mail that arrives at it, through the
// real sink.
//
// The refusals matter more than the discovery. A missed alias costs a contact
// somebody deletes; a trusted forgery lets a sender declare themselves to be
// the mailbox owner, which would suppress their own capture and shift what the
// owner's self-set covers.

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// deliveredMail is an inbound message carrying delivery headers in the order
// this test writes them, which is the whole subject: `above` lands before the
// receiving hop's Received, `below` after it.
func deliveredMail(above, below, from, msgID string) []byte {
	lines := []string{}
	if above != "" {
		lines = append(lines, "Delivered-To: "+above)
	}
	lines = append(lines, "Received: from mx.example by mail.google.com")
	if below != "" {
		lines = append(lines, "Delivered-To: "+below)
	}
	lines = append(lines,
		"From: "+from,
		"To: "+captureOwner,
		"Subject: project",
		"Date: Wed, 04 Jun 2026 08:00:00 +0000",
		"Message-ID: <"+msgID+">",
		"Content-Type: text/plain", "", "hello", "")
	return []byte(strings.Join(lines, "\r\n"))
}

// ownAddresses names every address this seat's self-set holds, with where each
// one came from.
func ownAddresses(t *testing.T, env captureEnv, seat ids.UUID) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := database.WithWorkspaceTx(env.e.Admin(), env.e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(),
			`SELECT value, source FROM capture_owner_identity WHERE user_id = $1`, seat)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var value, source string
			if err := rows.Scan(&value, &source); err != nil {
				return err
			}
			out[value] = source
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatalf("reading the seat's own addresses: %v", err)
	}
	return out
}

const forwardingAlias = "founder@previous-employer.example"

// Two distinct messages delivered to the alias, and the product learns it is
// the seat's own address — which is the whole ticket: an alias is never the
// From of anything the mailbox sends, so nothing else could have found it.
func TestAnAliasTwoMessagesWereDeliveredToBecomesTheSeatsOwn(t *testing.T) {
	env := newCaptureEnv(t)

	env.sync(t, deliveredMail(forwardingAlias, "", "buyer@customer.example", "alias-1@customer.example"))
	if got := ownAddresses(t, env, env.e.Rep1); got[forwardingAlias] != "" {
		t.Fatalf("one message was enough to claim %s (%v) — a single sighting is not corroboration",
			forwardingAlias, got)
	}

	env.sync(t, deliveredMail(forwardingAlias, "", "other@customer.example", "alias-2@customer.example"))
	if got := ownAddresses(t, env, env.e.Rep1)[forwardingAlias]; got != capturemod.IdentitySourceDeliveredTo {
		t.Errorf("after two messages %s is recorded with source %q, want %q — the seat is still a stranger "+
			"in their own CRM", forwardingAlias, got, capturemod.IdentitySourceDeliveredTo)
	}
}

// The same message twice is one sighting. A re-sync, or a push notification
// replaying a message already captured, must not be able to reach the
// threshold on its own — that would be corroboration by one piece of evidence
// counted twice.
func TestOneMessageArrivingTwiceIsOneSighting(t *testing.T) {
	env := newCaptureEnv(t)
	message := deliveredMail(forwardingAlias, "", "buyer@customer.example", "alias-replay@customer.example")

	env.sync(t, message)
	env.sync(t, message)

	if got := ownAddresses(t, env, env.e.Rep1); got[forwardingAlias] != "" {
		t.Errorf("one message counted twice claimed %s (%v)", forwardingAlias, got)
	}
}

// THE ATTACK. A sender writes their own Delivered-To naming somebody they want
// silenced. It lands below the receiving hop's Received — the position no
// sender can get above — and must never become an identity, however many
// messages carry it.
func TestASenderCannotDeclareAnyoneTheMailboxOwner(t *testing.T) {
	env := newCaptureEnv(t)
	const victim = "victim@customer.example"

	env.sync(t, deliveredMail("", victim, "attacker@elsewhere.example", "forged-1@elsewhere.example"))
	env.sync(t, deliveredMail("", victim, "attacker@elsewhere.example", "forged-2@elsewhere.example"))
	env.sync(t, deliveredMail("", victim, "attacker@elsewhere.example", "forged-3@elsewhere.example"))

	if got := ownAddresses(t, env, env.e.Rep1); got[victim] != "" {
		t.Fatalf("a sender declared %s to be the mailbox owner's own address (%v): their mail would then be "+
			"read as the owner's and never captured as correspondence", victim, got)
	}
}

// A real delivery above the hop and a forged one below it, in one message. The
// real one is learned and the forged one is not — which a walk that read the
// last match, or collected every match, would get exactly backwards.
func TestARealDeliveryIsLearnedAndAForgedOneBesideItIsNot(t *testing.T) {
	env := newCaptureEnv(t)
	const victim = "victim@customer.example"

	env.sync(t, deliveredMail(forwardingAlias, victim, "attacker@elsewhere.example", "mixed-1@elsewhere.example"))
	env.sync(t, deliveredMail(forwardingAlias, victim, "attacker@elsewhere.example", "mixed-2@elsewhere.example"))

	own := ownAddresses(t, env, env.e.Rep1)
	if own[forwardingAlias] == "" {
		t.Errorf("the delivery the receiving server wrote was not learned (%v)", own)
	}
	if own[victim] != "" {
		t.Errorf("the forged delivery beside it was learned as well (%v)", own)
	}
}

// The seat's own primary address is on every Delivered-To their mailbox
// receives. It is already covered by the connection, so it is not a candidate
// and never reaches the ledger — otherwise the table would fill with the one
// address nobody needs discovering.
func TestAnAddressTheSeatAlreadyHoldsIsNotRediscovered(t *testing.T) {
	env := newCaptureEnv(t)
	declareIdentity(t, env.e, env.e.Rep1, capturemod.IdentityKindAddress, captureOwner)

	env.sync(t, deliveredMail(captureOwner, "", "buyer@customer.example", "own-1@customer.example"))
	env.sync(t, deliveredMail(captureOwner, "", "other@customer.example", "own-2@customer.example"))

	var sightings int
	err := database.WithWorkspaceTx(env.e.Admin(), env.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM capture_alias_sighting WHERE value = $1`, captureOwner).Scan(&sightings)
	})
	if err != nil {
		t.Fatalf("counting the sightings: %v", err)
	}
	if sightings != 0 {
		t.Errorf("%d sightings recorded for an address the seat already holds", sightings)
	}
	// And the claim it already had is untouched — still the seat's own word,
	// not overwritten by a discovery.
	if got := ownAddresses(t, env, env.e.Rep1)[captureOwner]; got != capturemod.IdentitySourceUser {
		t.Errorf("the seat's declared address now reads as source %q", got)
	}
}
