// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package comms

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestPostReceiptAdmissionCannotReopenAnAcceptedDelivery(t *testing.T) {
	for _, failure := range []string{"error", "panic"} {
		t.Run(failure, func(t *testing.T) {
			e := setupStore(t)
			called := 0
			WithPartyAdmission(func(ctx context.Context, activity ids.ActivityID) error {
				called++
				if activity != e.activity || ctx.Err() != nil {
					t.Fatal("admission lost the accepted activity or inherited cancellation")
				}
				if _, bounded := ctx.Deadline(); !bounded {
					t.Fatal("post-receipt derivation is unbounded")
				}
				if failure == "panic" {
					panic("derivation failed")
				}
				return errors.New("derivation failed")
			})(e.store)
			id := e.stage(t, e.baseInput(e.activity, "party-receipt@example.test"))
			ctx, cancel := context.WithCancel(e.ctx)
			cancel()
			if err := e.store.RecordSent(ctx, id, connector.SendReceipt{ProviderMessageID: "accepted"}); err != nil {
				t.Fatal(err)
			}
			if err := e.store.RecordSent(e.ctx, id, connector.SendReceipt{ProviderMessageID: "duplicate"}); !errors.Is(err, ErrTerminal) {
				t.Fatalf("duplicate receipt: %v", err)
			}
			var state, receipt string
			if err := e.owner.QueryRow(t.Context(), `SELECT status,provider_message_id FROM comms_outbound WHERE id=$1`, id).Scan(&state, &receipt); err != nil {
				t.Fatal(err)
			}
			if state != "sent" || receipt != "accepted" || called != 1 {
				t.Fatalf("state=%s receipt=%s derivations=%d", state, receipt, called)
			}
		})
	}
}
