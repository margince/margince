// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose_test

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
)

// The provider's arrival time is what proves a mailbox already held a message
// before the seat connected it (contacts.receivedBeforeConnectedTx). It has to
// land on THIS seat's import row, and never be taken from the Date header.
func TestTheSinkRecordsWhenTheProviderSaysTheMessageArrived(t *testing.T) {
	e := integration.Setup(t)
	const messageID = "arrival@counterparty.test"
	const addr = "rep1@ws.example"
	raw := []byte(strings.Join([]string{
		"From: Pat Counterparty <pat@counterparty.test>",
		"To: " + addr,
		"Cc: kim@counterparty.test",
		"Subject: Angebot",
		"Date: Wed, 04 Jun 2014 08:00:00 +0000",
		"Message-ID: <" + messageID + ">",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Anbei.",
		"",
	}, "\r\n"))
	claimOwnAddress(t, e, e.Rep1, addr)
	parsed, err := mailmap.Parse(raw, addr)
	if err != nil {
		t.Fatal(err)
	}
	// First through a round that names no arrival time (a Graph delta), then
	// through one that does (the backfill): the empty value is filled.
	sink := capture.NewSink(e.DB())
	ref, err := sink.Upsert(connectorCtx(e, "gmail", e.Rep1), parsed.ToRecord("gmail", raw))
	if err != nil {
		t.Fatalf("capturing the message: %v", err)
	}
	rec := parsed.ToRecord("gmail", raw)
	arrived := time.Date(2026, 6, 4, 8, 0, 0, 0, time.UTC)
	rec.ProviderReceivedAt = arrived
	if _, err := sink.Upsert(connectorCtx(e, "gmail", e.Rep1), rec); err != nil {
		t.Fatalf("capturing the message again: %v", err)
	}

	got := scalar[time.Time](t, e, `
		SELECT provider_received_at FROM capture_import WHERE activity_id = $1 AND user_id = $2`, ref.ID, e.Rep1)
	if !got.Equal(arrived) {
		t.Errorf("the import row says the message arrived at %v, want the provider's %v (not the Date header)", got, arrived)
	}
}
