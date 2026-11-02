// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func captureCounterpartyReply(incoming context.Context, t *testing.T, e *integration.Env, owner ids.UUID, seat, key, counterparty, subject string) {
	t.Helper()
	at := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	parent := key + "-outbound@ws.example"
	messages := []owedMail{
		{from: seat, to: counterparty, subject: subject, messageID: parent, at: at},
		{from: counterparty, to: seat, subject: "Re: " + subject, messageID: key + "@counterparty.example", inReplyTo: parent, at: at.Add(time.Hour)},
	}
	sink := newCaptureSink(e.Pool, CaptureConfig{})
	for index, message := range messages {
		raw := message.raw()
		parsed, err := mailmap.Parse(raw, seat)
		if err != nil {
			t.Fatal(err)
		}
		ctx := incoming
		if index == 0 {
			parsed = parsed.AttestSentByOwner(true)
			ctx = mailboxOwnerCtx(e, owner)
		}
		if _, err := sink.Upsert(ctx, parsed.ToRecord("gmail", raw)); err != nil {
			t.Fatalf("capturing reply evidence: %v", err)
		}
	}
}
