// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The window the settlement pass reads a conversation through.
//
// This is the read that hands thread text to a model, so what it admits is a
// disclosure boundary rather than a matter of accuracy. The store's tests prove
// which requests are OFFERED; these prove what is READ once one is, which is a
// separate statement and was separately wrong.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedThreadMessage writes one message onto a named thread, as capture would.
//
// counterparty and attested are the two columns that say who a message was
// really with — the pair a forged References header cannot produce.
func seedThreadMessage(t *testing.T, e *integration.Env, thread, direction, body,
	counterparty string, attested bool, at time.Time,
) ids.UUID {
	t.Helper()
	activity := ids.NewV7()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, direction, subject, body, occurred_at,
			                      thread_key, counterparty_email, counterparty_outbound_attested,
			                      source, captured_by, audience)
			VALUES ($1, 'email', $2, 'Re: terms', $3, $4, $5, $6, $7, 'seed', 'system', 'workspace')`,
			activity, direction, body, at, thread, counterparty, attested)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return activity
}

// bodiesRead joins the bodies the conversation window returned, so a test can
// ask whether some text reached it.
func bodiesRead(t *testing.T, e *integration.Env, request ids.UUID, counterparty string, asOf time.Time) string {
	t.Helper()
	var joined strings.Builder
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		messages, err := requestConversation(context.Background(), tx, request, counterparty, asOf)
		if err != nil {
			return err
		}
		for _, message := range messages {
			joined.WriteString(message.Body)
			joined.WriteString("\n")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reading the conversation: %v", err)
	}
	return joined.String()
}

// A forged thread root must not pull another customer's mail into the window.
//
// thread_key can be the RFC822 References root, which the SENDER types, so a
// stranger who has seen one of our Message-IDs can put their own message on a
// real customer's thread. Read on the key alone, this window then returned that
// customer's own inbound text AND our replies to them — up to
// settleThreadMessages of it — and the pass sends every body to the model as
// context for the stranger's request. The correspondent is what refuses it.
func TestTheSettlementWindowNeverReadsAnotherCustomersMail(t *testing.T) {
	e := integration.Setup(t)
	const thread = "forged-references-root"
	const stranger = "stranger@elsewhere.test"
	const customer = "buyer@customer.test"
	base := time.Now().UTC().Add(-24 * time.Hour)

	request := seedThreadMessage(t, e, thread, "inbound",
		"Please confirm the terms.", stranger, false, base)
	seedThreadMessage(t, e, thread, "inbound",
		"Our confidential renewal number is 480000.", customer, false, base.Add(time.Hour))
	seedThreadMessage(t, e, thread, "outbound",
		"Confirming the 480000 figure for you.", customer, true, base.Add(2*time.Hour))

	read := bodiesRead(t, e, request, stranger, base.Add(48*time.Hour))
	if strings.Contains(read, "480000") {
		t.Fatalf("the window handed another customer's mail to the stranger's request; it read:\n%s", read)
	}
}

// Our own reply is still read, so the fix does not empty the window.
//
// A guard that refused everything would pass the test above and silently stop
// the pass judging anything, which no other assertion here would catch.
func TestTheSettlementWindowStillReadsOurOwnReply(t *testing.T) {
	e := integration.Setup(t)
	const thread = "an-honest-thread"
	const customer = "buyer@customer.test"
	base := time.Now().UTC().Add(-24 * time.Hour)

	request := seedThreadMessage(t, e, thread, "inbound",
		"Please confirm the terms.", customer, false, base)
	seedThreadMessage(t, e, thread, "outbound",
		"Confirmed, the terms stand.", customer, true, base.Add(time.Hour))

	read := bodiesRead(t, e, request, customer, base.Add(48*time.Hour))
	if !strings.Contains(read, "Confirmed, the terms stand.") {
		t.Fatalf("the window lost our own reply to this customer; it read:\n%s", read)
	}
}

// An unattested outbound is not evidence we wrote to this address.
//
// counterparty_email alone is settable by any path that files a row; the
// attestation is the provider's own record of having sent there, which is what
// makes the pairing forgery-resistant rather than merely present.
func TestTheSettlementWindowRefusesAnUnattestedOutbound(t *testing.T) {
	e := integration.Setup(t)
	const thread = "unattested-outbound"
	const customer = "buyer@customer.test"
	base := time.Now().UTC().Add(-24 * time.Hour)

	request := seedThreadMessage(t, e, thread, "inbound",
		"Please confirm the terms.", customer, false, base)
	seedThreadMessage(t, e, thread, "outbound",
		"An outbound nobody attested.", customer, false, base.Add(time.Hour))

	read := bodiesRead(t, e, request, customer, base.Add(48*time.Hour))
	if strings.Contains(read, "An outbound nobody attested.") {
		t.Fatalf("the window read an outbound the provider never filed as sent; it read:\n%s", read)
	}
}
