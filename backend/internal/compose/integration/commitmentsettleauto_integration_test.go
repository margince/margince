// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
)

// A trigger over no database fails if it tries to settle anything. A clean
// return shows an automatic close reaches no task and no offer.
func TestAnAutomaticCloseRetiresNoTaskAndNoOffer(t *testing.T) {
	trigger := compose.NewCommitmentSettleTrigger(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	env := events.Envelope{
		Type:    "conversation_claim.changed",
		Actor:   events.Actor{Type: "system", ID: contacts.PromiseKeptActor},
		Payload: []byte(`{"claim_id":"0193c6e0-0000-7000-8000-000000000001","status":"done"}`),
	}
	if err := trigger.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
}
