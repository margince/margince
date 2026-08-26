// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seatNamer is the edge from the agent surface to identity's seat read.
//
// A query row names the colleague who owns the record, which is what makes a
// workspace-wide read safe to act on: a company is visible to every rep by
// design (auth/tableclass.go — a rep who cannot see that an account belongs to
// another team contacts it again), so the row has to say whose it is.
//
// It lives here rather than in agents because a module never imports a sibling.
// The conversion is the whole body: agents speaks in bare ids because it knows
// nothing of seats, and identity answers about ids.UserID.
func seatNamer(seats *identity.Service) agents.SeatNamer {
	return func(ctx context.Context, owners []ids.UUID) (map[ids.UUID]string, error) {
		seatIDs := make([]ids.UserID, 0, len(owners))
		for _, owner := range owners {
			seatIDs = append(seatIDs, ids.From[ids.UserKind](owner))
		}
		return seats.SeatNames(ctx, seatIDs)
	}
}
