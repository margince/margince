// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
)

// A trigger with no stores panics if it tries to settle anything, so a clean
// return proves the automatic close reached no task and no offer.
func TestAnAutomaticCloseRetiresNoTaskAndNoOffer(t *testing.T) {
	trigger := &CommitmentSettleTrigger{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	env := events.Envelope{
		Type:    eventClaimChanged,
		Actor:   events.Actor{Type: "system", ID: contacts.PromiseKeptActor},
		Payload: []byte(`{"claim_id":"0193c6e0-0000-7000-8000-000000000001","status":"done"}`),
	}
	if err := trigger.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
}
