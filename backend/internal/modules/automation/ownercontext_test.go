// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestOwnerContextCarriesTheOwnersSeat(t *testing.T) {
	for _, seat := range []principal.SeatType{principal.SeatRead, principal.SeatFull} {
		t.Run(string(seat), func(t *testing.T) {
			owner := ids.NewV7()
			ctx, err := ownerContext(context.Background(), &fakeAuthzResolver{seat: seat}, ids.NewV7(), owner)
			if err != nil {
				t.Fatalf("ownerContext: %v", err)
			}
			actor, ok := principal.Actor(ctx)
			if !ok {
				t.Fatal("ownerContext bound no actor")
			}
			if actor.SeatType != seat {
				t.Errorf("seat = %q, want %q: a read seat acting in a background job must stay a read seat", actor.SeatType, seat)
			}
			if actor.UserID != owner || actor.ID != principal.HumanIDPrefix+owner.String() {
				t.Errorf("actor = %q for user %s, want the owner's own human id", actor.ID, actor.UserID)
			}
		})
	}
}
